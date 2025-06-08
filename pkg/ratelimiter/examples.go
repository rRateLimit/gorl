package ratelimiter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SimpleRateLimiter implements a basic rate limiter using a simple timer approach.
// This serves as an example of how to implement the RateLimiter interface.
type SimpleRateLimiter struct {
	rate     float64
	interval time.Duration
	lastTime time.Time
	mu       sync.Mutex
}

// NewSimpleRateLimiter creates a new simple rate limiter.
func NewSimpleRateLimiter(rate float64) RateLimiter {
	interval := time.Duration(float64(time.Second) / rate)
	return &SimpleRateLimiter{
		rate:     rate,
		interval: interval,
		lastTime: time.Now().Add(-interval), // Allow first request immediately
	}
}

// Allow implements the RateLimiter interface.
func (s *SimpleRateLimiter) Allow(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	nextAllowed := s.lastTime.Add(s.interval)

	if now.Before(nextAllowed) {
		waitDuration := nextAllowed.Sub(now)

		// Create a timer for the wait duration
		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		select {
		case <-timer.C:
			s.lastTime = time.Now()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	s.lastTime = now
	return nil
}

// String returns a description of the rate limiter.
func (s *SimpleRateLimiter) String() string {
	return fmt.Sprintf("Simple Rate Limiter (%.2f req/s)", s.rate)
}

// BurstRateLimiter implements a rate limiter that allows controlled bursts.
// It maintains a burst capacity that refills over time.
type BurstRateLimiter struct {
	rate         float64
	burstSize    int
	tokens       float64
	lastRefill   time.Time
	refillPeriod time.Duration
	mu           sync.Mutex
}

// NewBurstRateLimiter creates a new burst rate limiter.
// burstSize specifies how many requests can be made in a burst.
func NewBurstRateLimiter(rate float64, burstSize int) RateLimiter {
	refillPeriod := time.Duration(float64(time.Second) / rate)
	return &BurstRateLimiter{
		rate:         rate,
		burstSize:    burstSize,
		tokens:       float64(burstSize), // Start with full burst capacity
		lastRefill:   time.Now(),
		refillPeriod: refillPeriod,
	}
}

// Allow implements the RateLimiter interface.
func (b *BurstRateLimiter) Allow(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.refillTokens(now)

	// If we have tokens available, consume one and proceed
	if b.tokens >= 1.0 {
		b.tokens--
		return nil
	}

	// No tokens available, wait for next refill
	waitDuration := b.refillPeriod
	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	// Unlock while waiting to avoid blocking other operations
	b.mu.Unlock()
	select {
	case <-timer.C:
		b.mu.Lock()
		b.refillTokens(time.Now())
		if b.tokens >= 1.0 {
			b.tokens--
			return nil
		}
		// Fallback: token still not available, should not happen
		return fmt.Errorf("token not available after wait")
	case <-ctx.Done():
		b.mu.Lock() // Reacquire lock for defer
		return ctx.Err()
	}
}

// refillTokens adds tokens based on elapsed time (must be called with lock held).
func (b *BurstRateLimiter) refillTokens(now time.Time) {
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := elapsed.Seconds() * b.rate

	b.tokens = min(b.tokens+tokensToAdd, float64(b.burstSize))
	b.lastRefill = now
}

// String returns a description of the rate limiter.
func (b *BurstRateLimiter) String() string {
	return fmt.Sprintf("Burst Rate Limiter (%.2f req/s, burst: %d)", b.rate, b.burstSize)
}

// AdaptiveRateLimiter implements a rate limiter that adapts its rate based on success/failure patterns.
// This is an advanced example showing how to create a dynamic rate limiter.
type AdaptiveRateLimiter struct {
	baseRate     float64
	currentRate  float64
	successCount int
	failureCount int
	lastAdjust   time.Time
	adjustPeriod time.Duration
	maxRate      float64
	minRate      float64
	lastTime     time.Time
	mu           sync.Mutex
}

// NewAdaptiveRateLimiter creates a new adaptive rate limiter.
func NewAdaptiveRateLimiter(baseRate float64) RateLimiter {
	return &AdaptiveRateLimiter{
		baseRate:     baseRate,
		currentRate:  baseRate,
		lastAdjust:   time.Now(),
		adjustPeriod: 10 * time.Second,             // Adjust every 10 seconds
		maxRate:      baseRate * 2,                 // Maximum 2x base rate
		minRate:      baseRate * 0.5,               // Minimum 0.5x base rate
		lastTime:     time.Now().Add(-time.Second), // Allow first request
	}
}

// Allow implements the RateLimiter interface.
func (a *AdaptiveRateLimiter) Allow(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	a.maybeAdjustRate(now)

	interval := time.Duration(float64(time.Second) / a.currentRate)
	nextAllowed := a.lastTime.Add(interval)

	if now.Before(nextAllowed) {
		waitDuration := nextAllowed.Sub(now)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		a.mu.Unlock() // Unlock while waiting
		select {
		case <-timer.C:
			a.mu.Lock() // Reacquire lock
			a.lastTime = time.Now()
			return nil
		case <-ctx.Done():
			a.mu.Lock() // Reacquire lock for defer
			return ctx.Err()
		}
	}

	a.lastTime = now
	return nil
}

// ReportSuccess reports a successful request (used for adaptation).
func (a *AdaptiveRateLimiter) ReportSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.successCount++
}

// ReportFailure reports a failed request (used for adaptation).
func (a *AdaptiveRateLimiter) ReportFailure() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failureCount++
}

// maybeAdjustRate adjusts the rate based on success/failure patterns (must be called with lock held).
func (a *AdaptiveRateLimiter) maybeAdjustRate(now time.Time) {
	if now.Sub(a.lastAdjust) < a.adjustPeriod {
		return
	}

	totalRequests := a.successCount + a.failureCount
	if totalRequests == 0 {
		return
	}

	successRate := float64(a.successCount) / float64(totalRequests)

	// Increase rate if success rate is high, decrease if low
	if successRate > 0.9 {
		// High success rate, try to increase rate
		a.currentRate = min(a.currentRate*1.1, a.maxRate)
	} else if successRate < 0.7 {
		// Low success rate, decrease rate
		a.currentRate = max(a.currentRate*0.9, a.minRate)
	}

	// Reset counters
	a.successCount = 0
	a.failureCount = 0
	a.lastAdjust = now
}

// String returns a description of the rate limiter.
func (a *AdaptiveRateLimiter) String() string {
	return fmt.Sprintf("Adaptive Rate Limiter (current: %.2f req/s, base: %.2f req/s)",
		a.currentRate, a.baseRate)
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// RegisterExampleAlgorithms registers the example algorithms with the default registry.
// This function can be called during application initialization to make the examples available.
func RegisterExampleAlgorithms() error {
	algorithms := map[string]Factory{
		"simple": func(rate float64) RateLimiter {
			return NewSimpleRateLimiter(rate)
		},
		"burst": func(rate float64) RateLimiter {
			// Default burst size of 5
			return NewBurstRateLimiter(rate, 5)
		},
		"burst-10": func(rate float64) RateLimiter {
			// Burst size of 10
			return NewBurstRateLimiter(rate, 10)
		},
		"adaptive": func(rate float64) RateLimiter {
			return NewAdaptiveRateLimiter(rate)
		},
	}

	for name, factory := range algorithms {
		if err := Register(name, factory); err != nil {
			return fmt.Errorf("failed to register algorithm '%s': %w", name, err)
		}
	}

	return nil
}
