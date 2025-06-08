package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"golang.org/x/time/rate"
)

// RateLimiter interface for different rate limiting algorithms
type RateLimiter interface {
	Allow(ctx context.Context) error
	String() string
}

// TokenBucketLimiter implements token bucket algorithm
type TokenBucketLimiter struct {
	limiter *rate.Limiter
}

// NewTokenBucketLimiter creates a new token bucket rate limiter
func NewTokenBucketLimiter(requestsPerSecond float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), 1),
	}
}

func (t *TokenBucketLimiter) Allow(ctx context.Context) error {
	return t.limiter.Wait(ctx)
}

func (t *TokenBucketLimiter) String() string {
	return "Token Bucket"
}

// LeakyBucketLimiter implements leaky bucket algorithm
type LeakyBucketLimiter struct {
	interval    time.Duration
	lastRequest time.Time
	mutex       sync.Mutex
}

// NewLeakyBucketLimiter creates a new leaky bucket rate limiter
func NewLeakyBucketLimiter(requestsPerSecond float64) *LeakyBucketLimiter {
	interval := time.Duration(float64(time.Second) / requestsPerSecond)
	return &LeakyBucketLimiter{
		interval:    interval,
		lastRequest: time.Now().Add(-interval), // Allow first request immediately
	}
}

func (l *LeakyBucketLimiter) Allow(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now()
	timeSinceLastRequest := now.Sub(l.lastRequest)

	if timeSinceLastRequest < l.interval {
		sleepTime := l.interval - timeSinceLastRequest
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepTime):
		}
	}

	l.lastRequest = time.Now()
	return nil
}

func (l *LeakyBucketLimiter) String() string {
	return "Leaky Bucket"
}

// FixedWindowLimiter implements fixed window algorithm
type FixedWindowLimiter struct {
	windowSize    time.Duration
	maxRequests   int
	currentWindow time.Time
	requestCount  int
	mutex         sync.Mutex
}

// NewFixedWindowLimiter creates a new fixed window rate limiter
func NewFixedWindowLimiter(requestsPerSecond float64) *FixedWindowLimiter {
	windowSize := time.Second
	maxRequests := int(requestsPerSecond)
	if maxRequests == 0 {
		maxRequests = 1
	}

	return &FixedWindowLimiter{
		windowSize:    windowSize,
		maxRequests:   maxRequests,
		currentWindow: time.Now().Truncate(windowSize),
	}
}

func (f *FixedWindowLimiter) Allow(ctx context.Context) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	now := time.Now()
	currentWindowStart := now.Truncate(f.windowSize)

	// Reset counter if we're in a new window
	if currentWindowStart.After(f.currentWindow) {
		f.currentWindow = currentWindowStart
		f.requestCount = 0
	}

	// Check if we've exceeded the limit
	if f.requestCount >= f.maxRequests {
		// Wait until next window
		nextWindow := f.currentWindow.Add(f.windowSize)
		sleepTime := nextWindow.Sub(now)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepTime):
		}

		// Update to new window
		f.currentWindow = time.Now().Truncate(f.windowSize)
		f.requestCount = 0
	}

	f.requestCount++
	return nil
}

func (f *FixedWindowLimiter) String() string {
	return "Fixed Window"
}

// SlidingWindowLogLimiter implements sliding window log algorithm
type SlidingWindowLogLimiter struct {
	maxRequests  int
	windowSize   time.Duration
	requestTimes []time.Time
	mutex        sync.Mutex
}

// NewSlidingWindowLogLimiter creates a new sliding window log rate limiter
func NewSlidingWindowLogLimiter(requestsPerSecond float64) *SlidingWindowLogLimiter {
	return &SlidingWindowLogLimiter{
		maxRequests:  int(requestsPerSecond),
		windowSize:   time.Second,
		requestTimes: make([]time.Time, 0),
	}
}

func (s *SlidingWindowLogLimiter) Allow(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	windowStart := now.Add(-s.windowSize)

	// Remove old requests outside the window
	validRequests := make([]time.Time, 0)
	for _, reqTime := range s.requestTimes {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}
	s.requestTimes = validRequests

	// Check if we can make another request
	if len(s.requestTimes) >= s.maxRequests {
		// Find the oldest request and wait until it's outside the window
		oldestRequest := s.requestTimes[0]
		waitTime := oldestRequest.Add(s.windowSize).Sub(now)

		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}

		// Retry the check after waiting
		return s.Allow(ctx)
	}

	// Add current request to log
	s.requestTimes = append(s.requestTimes, now)
	return nil
}

func (s *SlidingWindowLogLimiter) String() string {
	return "Sliding Window Log"
}

// SlidingWindowCounterLimiter implements sliding window counter algorithm
type SlidingWindowCounterLimiter struct {
	maxRequests      int
	windowSize       time.Duration
	subWindowSize    time.Duration
	subWindowCount   int
	counters         []int
	currentSubWindow int
	lastUpdateTime   time.Time
	mutex            sync.Mutex
}

// NewSlidingWindowCounterLimiter creates a new sliding window counter rate limiter
func NewSlidingWindowCounterLimiter(requestsPerSecond float64) *SlidingWindowCounterLimiter {
	subWindowCount := 10 // Divide window into 10 sub-windows for better precision
	windowSize := time.Second
	subWindowSize := windowSize / time.Duration(subWindowCount)

	return &SlidingWindowCounterLimiter{
		maxRequests:    int(requestsPerSecond),
		windowSize:     windowSize,
		subWindowSize:  subWindowSize,
		subWindowCount: subWindowCount,
		counters:       make([]int, subWindowCount),
		lastUpdateTime: time.Now(),
	}
}

func (s *SlidingWindowCounterLimiter) Allow(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	s.updateCounters(now)

	// Calculate total requests in current window
	totalRequests := 0
	for _, count := range s.counters {
		totalRequests += count
	}

	// Check if we can make another request
	if totalRequests >= s.maxRequests {
		// Wait for the next sub-window
		nextSubWindow := s.lastUpdateTime.Add(s.subWindowSize)
		waitTime := nextSubWindow.Sub(now)

		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}

		// Retry after waiting
		return s.Allow(ctx)
	}

	// Increment counter for current sub-window
	s.counters[s.currentSubWindow]++
	return nil
}

func (s *SlidingWindowCounterLimiter) updateCounters(now time.Time) {
	timeDiff := now.Sub(s.lastUpdateTime)
	subWindowsPassed := int(timeDiff / s.subWindowSize)

	if subWindowsPassed > 0 {
		// Clear counters for passed sub-windows
		for i := 0; i < subWindowsPassed && i < s.subWindowCount; i++ {
			s.currentSubWindow = (s.currentSubWindow + 1) % s.subWindowCount
			s.counters[s.currentSubWindow] = 0
		}

		// If more than a full window has passed, clear all counters
		if subWindowsPassed >= s.subWindowCount {
			for i := range s.counters {
				s.counters[i] = 0
			}
		}

		s.lastUpdateTime = now
	}
}

func (s *SlidingWindowCounterLimiter) String() string {
	return "Sliding Window Counter"
}

// NewRateLimiter creates a rate limiter based on the specified type
func NewRateLimiter(limiterType config.RateLimiterType, requestsPerSecond float64) RateLimiter {
	switch limiterType {
	case config.TokenBucket:
		return NewTokenBucketLimiter(requestsPerSecond)
	case config.LeakyBucket:
		return NewLeakyBucketLimiter(requestsPerSecond)
	case config.FixedWindow:
		return NewFixedWindowLimiter(requestsPerSecond)
	case config.SlidingWindowLog:
		return NewSlidingWindowLogLimiter(requestsPerSecond)
	case config.SlidingWindowCounter:
		return NewSlidingWindowCounterLimiter(requestsPerSecond)
	default:
		return NewTokenBucketLimiter(requestsPerSecond)
	}
}
