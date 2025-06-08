package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/limiter"
	"github.com/rRateLimit/gorl/pkg/ratelimiter"
)

// TestCustomRateLimiter は基本的なカスタムレートリミッターの実装例
type TestCustomRateLimiter struct {
	rate      float64
	interval  time.Duration
	lastTime  time.Time
	callCount int
	mu        sync.Mutex
}

func NewTestCustomRateLimiter(rate float64) ratelimiter.RateLimiter {
	return &TestCustomRateLimiter{
		rate:     rate,
		interval: time.Duration(float64(time.Second) / rate),
		lastTime: time.Now().Add(-time.Second), // Allow first request immediately
	}
}

func (t *TestCustomRateLimiter) Allow(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.callCount++
	now := time.Now()
	nextAllowed := t.lastTime.Add(t.interval)

	if now.Before(nextAllowed) {
		waitDuration := nextAllowed.Sub(now)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		t.mu.Unlock() // Unlock while waiting
		select {
		case <-timer.C:
			t.mu.Lock() // Reacquire lock
			t.lastTime = time.Now()
			return nil
		case <-ctx.Done():
			t.mu.Lock() // Reacquire lock for defer
			return ctx.Err()
		}
	}

	t.lastTime = now
	return nil
}

func (t *TestCustomRateLimiter) String() string {
	return fmt.Sprintf("Test Custom Limiter (%.2f req/s, calls: %d)", t.rate, t.callCount)
}

func (t *TestCustomRateLimiter) GetCallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.callCount
}

func TestCustomAlgorithmRegistration(t *testing.T) {
	// Create a new registry for isolated testing
	registry := ratelimiter.NewRegistry()

	// Test successful registration
	err := registry.Register("test-custom", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err != nil {
		t.Errorf("Failed to register custom algorithm: %v", err)
	}

	// Test duplicate registration fails
	err = registry.Register("test-custom", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err == nil {
		t.Error("Expected error when registering duplicate algorithm")
	}

	// Test algorithm is registered
	if !registry.IsRegistered("test-custom") {
		t.Error("Algorithm should be registered")
	}

	// Test algorithm creation
	limiter, err := registry.Create("test-custom", 5.0)
	if err != nil {
		t.Errorf("Failed to create custom algorithm: %v", err)
	}

	if limiter == nil {
		t.Error("Created limiter should not be nil")
	}

	// Test limiter functionality
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = limiter.Allow(ctx)
	if err != nil {
		t.Errorf("First request should be allowed: %v", err)
	}
}

func TestCustomAlgorithmUnregistration(t *testing.T) {
	registry := ratelimiter.NewRegistry()

	// Register an algorithm
	err := registry.Register("temp-algo", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err != nil {
		t.Errorf("Failed to register algorithm: %v", err)
	}

	// Verify it's registered
	if !registry.IsRegistered("temp-algo") {
		t.Error("Algorithm should be registered")
	}

	// Unregister it
	err = registry.Unregister("temp-algo")
	if err != nil {
		t.Errorf("Failed to unregister algorithm: %v", err)
	}

	// Verify it's no longer registered
	if registry.IsRegistered("temp-algo") {
		t.Error("Algorithm should not be registered after unregistration")
	}

	// Test unregistering non-existent algorithm
	err = registry.Unregister("non-existent")
	if err == nil {
		t.Error("Expected error when unregistering non-existent algorithm")
	}
}

func TestCustomAlgorithmListing(t *testing.T) {
	registry := ratelimiter.NewRegistry()

	// Initially should be empty
	list := registry.List()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d algorithms", len(list))
	}

	// Register some algorithms
	algorithms := []string{"algo1", "algo2", "algo3"}
	for _, name := range algorithms {
		err := registry.Register(name, func(rate float64) ratelimiter.RateLimiter {
			return NewTestCustomRateLimiter(rate)
		})
		if err != nil {
			t.Errorf("Failed to register %s: %v", name, err)
		}
	}

	// Check list
	list = registry.List()
	if len(list) != len(algorithms) {
		t.Errorf("Expected %d algorithms, got %d", len(algorithms), len(list))
	}

	// Verify all algorithms are in the list
	for _, expected := range algorithms {
		found := false
		for _, actual := range list {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Algorithm %s not found in list", expected)
		}
	}
}

func TestCustomAlgorithmIntegrationWithLimiter(t *testing.T) {
	// Test that custom algorithms work with the main limiter package

	// Register a custom algorithm in the default registry
	algorithmName := "integration-test-custom"
	err := ratelimiter.Register(algorithmName, func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err != nil {
		t.Errorf("Failed to register custom algorithm: %v", err)
	}
	defer ratelimiter.Unregister(algorithmName) // Clean up

	// Test that the limiter package recognizes it as a custom algorithm
	if !limiter.IsCustomAlgorithm(algorithmName) {
		t.Error("Limiter package should recognize custom algorithm")
	}

	// Test algorithm validation
	err = limiter.ValidateAlgorithm(algorithmName)
	if err != nil {
		t.Errorf("Custom algorithm should be valid: %v", err)
	}

	// Test that it appears in custom algorithm list
	customAlgos := limiter.ListCustomAlgorithms()
	found := false
	for _, name := range customAlgos {
		if name == algorithmName {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom algorithm should appear in list")
	}

	// Test creating rate limiter with custom algorithm
	rateLimiter := limiter.NewRateLimiter(config.RateLimiterType(algorithmName), 3.0)
	if rateLimiter == nil {
		t.Error("Should be able to create rate limiter with custom algorithm")
	}

	// Test that it works
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Custom rate limiter should allow request: %v", err)
	}
}

func TestCustomAlgorithmErrorHandling(t *testing.T) {
	registry := ratelimiter.NewRegistry()

	// Test registering with empty name
	err := registry.Register("", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err == nil {
		t.Error("Expected error when registering with empty name")
	}

	// Test registering with nil factory
	err = registry.Register("test-nil", nil)
	if err == nil {
		t.Error("Expected error when registering with nil factory")
	}

	// Test creating non-existent algorithm
	_, err = registry.Create("non-existent", 5.0)
	if err == nil {
		t.Error("Expected error when creating non-existent algorithm")
	}
}

func TestCustomAlgorithmConcurrency(t *testing.T) {
	registry := ratelimiter.NewRegistry()

	// Register an algorithm
	err := registry.Register("concurrent-test", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})
	if err != nil {
		t.Errorf("Failed to register algorithm: %v", err)
	}

	// Test concurrent access
	const numGoroutines = 10
	const numOperations = 100

	errChan := make(chan error, numGoroutines)
	doneChan := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { doneChan <- true }()

			for j := 0; j < numOperations; j++ {
				// Mix of different operations
				switch j % 4 {
				case 0:
					if !registry.IsRegistered("concurrent-test") {
						errChan <- fmt.Errorf("goroutine %d: algorithm not registered", id)
						return
					}
				case 1:
					_, err := registry.Create("concurrent-test", float64(j+1))
					if err != nil {
						errChan <- fmt.Errorf("goroutine %d: failed to create: %v", id, err)
						return
					}
				case 2:
					list := registry.List()
					if len(list) == 0 {
						errChan <- fmt.Errorf("goroutine %d: empty list", id)
						return
					}
				case 3:
					// Try to register with different name
					tempName := fmt.Sprintf("temp-%d-%d", id, j)
					err := registry.Register(tempName, func(rate float64) ratelimiter.RateLimiter {
						return NewTestCustomRateLimiter(rate)
					})
					if err != nil {
						errChan <- fmt.Errorf("goroutine %d: failed to register %s: %v", id, tempName, err)
						return
					}
					// Clean up
					registry.Unregister(tempName)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-doneChan
	}

	// Check for errors
	close(errChan)
	for err := range errChan {
		t.Error(err)
	}
}

func TestCustomAlgorithmPerformance(t *testing.T) {
	// Simple performance test for custom algorithm
	customLimiter := NewTestCustomRateLimiter(100.0) // High rate for performance testing

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numRequests = 1000
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		err := customLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
			break
		}
	}

	elapsed := time.Since(start)
	rate := float64(numRequests) / elapsed.Seconds()

	if rate < 50 { // Should be able to handle at least 50 req/s
		t.Errorf("Performance too low: %.2f req/s", rate)
	}

	t.Logf("Custom algorithm performance: %.2f req/s", rate)
}

func TestExampleAlgorithmsRegistration(t *testing.T) {
	// Test that example algorithms can be registered without conflict
	err := ratelimiter.RegisterExampleAlgorithms()
	if err != nil {
		t.Errorf("Failed to register example algorithms: %v", err)
	}

	// Test that expected algorithms are registered
	expectedAlgorithms := []string{"simple", "burst", "burst-10", "adaptive"}
	for _, name := range expectedAlgorithms {
		if !ratelimiter.IsRegistered(name) {
			t.Errorf("Example algorithm '%s' should be registered", name)
		}

		// Test that we can create instances
		_, err := ratelimiter.Create(name, 5.0)
		if err != nil {
			t.Errorf("Failed to create example algorithm '%s': %v", name, err)
		}
	}
}

// Benchmark tests for custom algorithms
func BenchmarkCustomAlgorithmRegistration(b *testing.B) {
	registry := ratelimiter.NewRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("bench-algo-%d", i)
		registry.Register(name, func(rate float64) ratelimiter.RateLimiter {
			return NewTestCustomRateLimiter(rate)
		})
	}
}

func BenchmarkCustomAlgorithmCreation(b *testing.B) {
	registry := ratelimiter.NewRegistry()
	registry.Register("bench-create", func(rate float64) ratelimiter.RateLimiter {
		return NewTestCustomRateLimiter(rate)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Create("bench-create", 10.0)
	}
}

func BenchmarkCustomAlgorithmAllow(b *testing.B) {
	limiter := NewTestCustomRateLimiter(1000.0) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx)
	}
}
