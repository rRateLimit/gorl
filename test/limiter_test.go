package test

import (
	"context"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/limiter"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name         string
		limiterType  config.RateLimiterType
		rate         float64
		expectedType string
	}{
		{
			name:         "token bucket",
			limiterType:  config.TokenBucket,
			rate:         5.0,
			expectedType: "Token Bucket",
		},
		{
			name:         "leaky bucket",
			limiterType:  config.LeakyBucket,
			rate:         3.0,
			expectedType: "Leaky Bucket",
		},
		{
			name:         "fixed window",
			limiterType:  config.FixedWindow,
			rate:         10.0,
			expectedType: "Fixed Window",
		},
		{
			name:         "sliding window log",
			limiterType:  config.SlidingWindowLog,
			rate:         7.0,
			expectedType: "Sliding Window Log",
		},
		{
			name:         "sliding window counter",
			limiterType:  config.SlidingWindowCounter,
			rate:         8.0,
			expectedType: "Sliding Window Counter",
		},
		{
			name:         "invalid type defaults to token bucket",
			limiterType:  "invalid",
			rate:         5.0,
			expectedType: "Token Bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rateLimiter := limiter.NewRateLimiter(tt.limiterType, tt.rate)
			if rateLimiter == nil {
				t.Fatal("Expected non-nil rate limiter")
			}

			if rateLimiter.String() != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, rateLimiter.String())
			}
		})
	}
}

func TestTokenBucketLimiter(t *testing.T) {
	rate := 5.0 // 5 requests per second
	rateLimiter := limiter.NewTokenBucketLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test basic functionality
	start := time.Now()
	for i := 0; i < 3; i++ {
		err := rateLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	}

	// Should take approximately 2/5 seconds (0.4s) for 2 additional requests
	// since we allow 5 requests per second and already used 3
	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Errorf("Token bucket took too long: %v", elapsed)
	}
}

func TestLeakyBucketLimiter(t *testing.T) {
	rate := 10.0 // 10 requests per second = 100ms interval
	rateLimiter := limiter.NewLeakyBucketLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test timing precision
	start := time.Now()

	// First request should be immediate
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for first request, got %v", err)
	}

	// Second request should wait ~100ms
	err = rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for second request, got %v", err)
	}

	elapsed := time.Since(start)
	expectedMin := 80 * time.Millisecond // Allow some tolerance
	expectedMax := 150 * time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("Leaky bucket timing unexpected: got %v, expected between %v and %v",
			elapsed, expectedMin, expectedMax)
	}
}

func TestFixedWindowLimiter(t *testing.T) {
	rate := 3.0 // 3 requests per second
	rateLimiter := limiter.NewFixedWindowLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()

	// Should allow 3 requests quickly
	for i := 0; i < 3; i++ {
		err := rateLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Expected no error for request %d, got %v", i+1, err)
		}
	}

	// This should be fast (within the same window)
	quickElapsed := time.Since(start)
	if quickElapsed > 100*time.Millisecond {
		t.Errorf("Fixed window should allow quick requests within window, took %v", quickElapsed)
	}

	// 4th request should wait for next window
	start = time.Now()
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for 4th request, got %v", err)
	}

	elapsed := time.Since(start)
	// Should wait for next second window
	if elapsed < 800*time.Millisecond || elapsed > 1200*time.Millisecond {
		t.Errorf("Fixed window should wait for next window, waited %v", elapsed)
	}
}

func TestSlidingWindowLogLimiter(t *testing.T) {
	rate := 2.0 // 2 requests per second
	rateLimiter := limiter.NewSlidingWindowLogLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should allow 2 requests quickly
	start := time.Now()
	for i := 0; i < 2; i++ {
		err := rateLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Expected no error for request %d, got %v", i+1, err)
		}
	}

	quickElapsed := time.Since(start)
	if quickElapsed > 50*time.Millisecond {
		t.Errorf("Sliding window log should allow quick initial requests, took %v", quickElapsed)
	}

	// 3rd request should wait
	start = time.Now()
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for 3rd request, got %v", err)
	}

	elapsed := time.Since(start)
	// Should wait approximately 1 second (for the first request to age out)
	if elapsed < 800*time.Millisecond || elapsed > 1200*time.Millisecond {
		t.Errorf("Sliding window log should wait for window slide, waited %v", elapsed)
	}
}

func TestSlidingWindowCounterLimiter(t *testing.T) {
	rate := 4.0 // 4 requests per second
	rateLimiter := limiter.NewSlidingWindowCounterLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should allow multiple requests quickly
	start := time.Now()
	for i := 0; i < 4; i++ {
		err := rateLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Expected no error for request %d, got %v", i+1, err)
		}
	}

	quickElapsed := time.Since(start)
	if quickElapsed > 100*time.Millisecond {
		t.Errorf("Sliding window counter should allow quick initial requests, took %v", quickElapsed)
	}

	// 5th request should wait for sub-window
	start = time.Now()
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for 5th request, got %v", err)
	}

	elapsed := time.Since(start)
	// Should wait for next sub-window (100ms with 10 sub-windows per second)
	if elapsed < 80*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Sliding window counter should wait for sub-window, waited %v", elapsed)
	}
}

func TestRateLimiterCancellation(t *testing.T) {
	rate := 1.0 // 1 request per second (slow)
	rateLimiter := limiter.NewLeakyBucketLimiter(rate)

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Allow first request
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for first request, got %v", err)
	}

	// Cancel context before second request
	cancel()

	// Second request should return context error
	err = rateLimiter.Allow(ctx)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRateLimiterWithZeroRate(t *testing.T) {
	// Test edge case with very low rate
	rate := 0.1 // 0.1 requests per second = 10 seconds per request
	rateLimiter := limiter.NewLeakyBucketLimiter(rate)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First request should be immediate
	start := time.Now()
	err := rateLimiter.Allow(ctx)
	if err != nil {
		t.Errorf("Expected no error for first request, got %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("First request should be immediate, took %v", elapsed)
	}

	// Second request should timeout
	start = time.Now()
	err = rateLimiter.Allow(ctx)

	elapsed = time.Since(start)
	// Should timeout after ~2 seconds
	if err == nil {
		t.Error("Expected timeout error for second request")
	}

	if elapsed < 1800*time.Millisecond || elapsed > 2200*time.Millisecond {
		t.Errorf("Expected ~2s timeout, got %v", elapsed)
	}
}

// Benchmark tests
func BenchmarkTokenBucketLimiter(b *testing.B) {
	rateLimiter := limiter.NewTokenBucketLimiter(1000) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rateLimiter.Allow(ctx)
		}
	})
}

func BenchmarkLeakyBucketLimiter(b *testing.B) {
	rateLimiter := limiter.NewLeakyBucketLimiter(1000) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateLimiter.Allow(ctx)
	}
}

func BenchmarkFixedWindowLimiter(b *testing.B) {
	rateLimiter := limiter.NewFixedWindowLimiter(1000) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateLimiter.Allow(ctx)
	}
}

func BenchmarkSlidingWindowLogLimiter(b *testing.B) {
	rateLimiter := limiter.NewSlidingWindowLogLimiter(1000) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateLimiter.Allow(ctx)
	}
}

func BenchmarkSlidingWindowCounterLimiter(b *testing.B) {
	rateLimiter := limiter.NewSlidingWindowCounterLimiter(1000) // High rate to minimize waiting
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateLimiter.Allow(ctx)
	}
}
