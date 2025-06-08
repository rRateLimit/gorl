// Package main demonstrates how to implement and use custom rate limiting algorithms with GoRL.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rRateLimit/gorl/pkg/ratelimiter"
)

// ExponentialBackoffLimiter implements a rate limiter with exponential backoff on failures.
// This is an example of how to create a custom rate limiting algorithm.
type ExponentialBackoffLimiter struct {
	baseRate      float64
	currentRate   float64
	lastTime      time.Time
	failureCount  int
	maxBackoff    time.Duration
	backoffFactor float64
	resetInterval time.Duration
	lastReset     time.Time
	mu            sync.Mutex
}

// NewExponentialBackoffLimiter creates a new exponential backoff rate limiter.
func NewExponentialBackoffLimiter(rate float64) ratelimiter.RateLimiter {
	return &ExponentialBackoffLimiter{
		baseRate:      rate,
		currentRate:   rate,
		lastTime:      time.Now().Add(-time.Second), // Allow first request
		maxBackoff:    10 * time.Second,
		backoffFactor: 2.0,
		resetInterval: 30 * time.Second,
		lastReset:     time.Now(),
	}
}

// Allow implements the RateLimiter interface.
func (e *ExponentialBackoffLimiter) Allow(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	// Reset failure count if enough time has passed
	if now.Sub(e.lastReset) > e.resetInterval {
		e.failureCount = 0
		e.currentRate = e.baseRate
		e.lastReset = now
	}

	// Calculate interval based on current rate
	interval := time.Duration(float64(time.Second) / e.currentRate)
	nextAllowed := e.lastTime.Add(interval)

	if now.Before(nextAllowed) {
		waitDuration := nextAllowed.Sub(now)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		select {
		case <-timer.C:
			e.lastTime = time.Now()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	e.lastTime = now
	return nil
}

// ReportFailure should be called when a request fails to adjust the rate.
func (e *ExponentialBackoffLimiter) ReportFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.failureCount++
	backoffDuration := time.Duration(float64(time.Second) *
		(e.backoffFactor * float64(e.failureCount)))

	if backoffDuration > e.maxBackoff {
		backoffDuration = e.maxBackoff
	}

	// Reduce current rate based on backoff
	e.currentRate = e.baseRate / (1 + backoffDuration.Seconds())
	if e.currentRate < 0.1 {
		e.currentRate = 0.1 // Minimum rate
	}
}

// String returns a description of the rate limiter.
func (e *ExponentialBackoffLimiter) String() string {
	return fmt.Sprintf("Exponential Backoff Limiter (base: %.2f req/s, current: %.2f req/s, failures: %d)",
		e.baseRate, e.currentRate, e.failureCount)
}

// CircuitBreakerLimiter implements a circuit breaker pattern with rate limiting.
type CircuitBreakerLimiter struct {
	rate             float64
	interval         time.Duration
	lastTime         time.Time
	failures         int
	successes        int
	state            CircuitState
	stateChanged     time.Time
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	mu               sync.Mutex
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreakerLimiter creates a new circuit breaker rate limiter.
func NewCircuitBreakerLimiter(rate float64) ratelimiter.RateLimiter {
	return &CircuitBreakerLimiter{
		rate:             rate,
		interval:         time.Duration(float64(time.Second) / rate),
		lastTime:         time.Now().Add(-time.Second),
		state:            CircuitClosed,
		stateChanged:     time.Now(),
		failureThreshold: 5,                // Open circuit after 5 failures
		successThreshold: 3,                // Close circuit after 3 successes
		timeout:          30 * time.Second, // Try half-open after 30s
	}
}

// Allow implements the RateLimiter interface.
func (c *CircuitBreakerLimiter) Allow(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.updateState(now)

	switch c.state {
	case CircuitOpen:
		return fmt.Errorf("circuit breaker is open")
	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if c.successes >= 1 {
			return fmt.Errorf("circuit breaker is half-open, limiting requests")
		}
	}

	// Normal rate limiting
	nextAllowed := c.lastTime.Add(c.interval)
	if now.Before(nextAllowed) {
		waitDuration := nextAllowed.Sub(now)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		c.mu.Unlock()
		select {
		case <-timer.C:
			c.mu.Lock()
			c.lastTime = time.Now()
			return nil
		case <-ctx.Done():
			c.mu.Lock()
			return ctx.Err()
		}
	}

	c.lastTime = now
	return nil
}

// ReportSuccess should be called when a request succeeds.
func (c *CircuitBreakerLimiter) ReportSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failures = 0
	if c.state == CircuitHalfOpen {
		c.successes++
		if c.successes >= c.successThreshold {
			c.state = CircuitClosed
			c.stateChanged = time.Now()
			c.successes = 0
		}
	}
}

// ReportFailure should be called when a request fails.
func (c *CircuitBreakerLimiter) ReportFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failures++
	c.successes = 0

	if c.state == CircuitClosed && c.failures >= c.failureThreshold {
		c.state = CircuitOpen
		c.stateChanged = time.Now()
	} else if c.state == CircuitHalfOpen {
		c.state = CircuitOpen
		c.stateChanged = time.Now()
	}
}

// updateState updates the circuit breaker state based on time.
func (c *CircuitBreakerLimiter) updateState(now time.Time) {
	if c.state == CircuitOpen && now.Sub(c.stateChanged) >= c.timeout {
		c.state = CircuitHalfOpen
		c.stateChanged = now
		c.successes = 0
	}
}

// String returns a description of the rate limiter.
func (c *CircuitBreakerLimiter) String() string {
	states := map[CircuitState]string{
		CircuitClosed:   "Closed",
		CircuitOpen:     "Open",
		CircuitHalfOpen: "Half-Open",
	}
	return fmt.Sprintf("Circuit Breaker Limiter (%.2f req/s, state: %s, failures: %d)",
		c.rate, states[c.state], c.failures)
}

func main() {
	fmt.Println("Custom Rate Limiter Algorithm Example")
	fmt.Println("=====================================")
	fmt.Println()

	// Register custom algorithms
	err := ratelimiter.Register("exponential-backoff", func(rate float64) ratelimiter.RateLimiter {
		return NewExponentialBackoffLimiter(rate)
	})
	if err != nil {
		log.Fatalf("Failed to register exponential-backoff algorithm: %v", err)
	}

	err = ratelimiter.Register("circuit-breaker", func(rate float64) ratelimiter.RateLimiter {
		return NewCircuitBreakerLimiter(rate)
	})
	if err != nil {
		log.Fatalf("Failed to register circuit-breaker algorithm: %v", err)
	}

	fmt.Println("Successfully registered custom algorithms:")
	fmt.Printf("  - exponential-backoff\n")
	fmt.Printf("  - circuit-breaker\n")
	fmt.Println()

	// List all registered algorithms
	fmt.Println("All available algorithms:")
	for _, name := range ratelimiter.List() {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	// Test the exponential backoff algorithm
	fmt.Println("Testing Exponential Backoff Algorithm:")
	testAlgorithm("exponential-backoff", 2.0)

	fmt.Println()

	// Test the circuit breaker algorithm
	fmt.Println("Testing Circuit Breaker Algorithm:")
	testAlgorithm("circuit-breaker", 3.0)

	fmt.Println()
	fmt.Println("You can now use these custom algorithms with GoRL:")
	fmt.Println("  gorl -url=https://httpbin.org/get -rate=5 -algorithm=exponential-backoff")
	fmt.Println("  gorl -url=https://httpbin.org/get -rate=5 -algorithm=circuit-breaker")
}

func testAlgorithm(name string, rate float64) {
	limiter, err := ratelimiter.Create(name, rate)
	if err != nil {
		log.Printf("Failed to create %s: %v", name, err)
		return
	}

	fmt.Printf("Created: %s\n", limiter.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test a few requests
	for i := 0; i < 3; i++ {
		start := time.Now()
		err := limiter.Allow(ctx)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("Request %d failed: %v (waited %v)\n", i+1, err, elapsed)
		} else {
			fmt.Printf("Request %d allowed (waited %v)\n", i+1, elapsed)
		}
	}
}
