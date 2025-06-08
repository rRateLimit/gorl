package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiterType represents the type of rate limiting algorithm
type RateLimiterType string

const (
	TokenBucket          RateLimiterType = "token-bucket"
	LeakyBucket          RateLimiterType = "leaky-bucket"
	FixedWindow          RateLimiterType = "fixed-window"
	SlidingWindowLog     RateLimiterType = "sliding-window-log"
	SlidingWindowCounter RateLimiterType = "sliding-window-counter"
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

// CreateRateLimiter creates a rate limiter based on the specified type
func CreateRateLimiter(limiterType RateLimiterType, requestsPerSecond float64) RateLimiter {
	switch limiterType {
	case TokenBucket:
		return NewTokenBucketLimiter(requestsPerSecond)
	case LeakyBucket:
		return NewLeakyBucketLimiter(requestsPerSecond)
	case FixedWindow:
		return NewFixedWindowLimiter(requestsPerSecond)
	case SlidingWindowLog:
		return NewSlidingWindowLogLimiter(requestsPerSecond)
	case SlidingWindowCounter:
		return NewSlidingWindowCounterLimiter(requestsPerSecond)
	default:
		return NewTokenBucketLimiter(requestsPerSecond)
	}
}

// Config holds the configuration for the rate limiter
type Config struct {
	URL               string            // target URL
	RequestsPerSecond float64           // requests per second
	Duration          time.Duration     // total duration to run
	Concurrency       int               // number of concurrent workers
	Method            string            // HTTP method
	Headers           map[string]string // HTTP headers
	Body              string            // request body
	Algorithm         RateLimiterType   // rate limiting algorithm
}

// UnmarshalJSON implements custom JSON unmarshaling for Config
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		Duration  string `json:"duration"`
		Algorithm string `json:"algorithm"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.Duration != "" {
		duration, err := time.ParseDuration(aux.Duration)
		if err != nil {
			return fmt.Errorf("invalid duration format: %v", err)
		}
		c.Duration = duration
	}

	if aux.Algorithm != "" {
		c.Algorithm = RateLimiterType(aux.Algorithm)
	}

	return nil
}

// Stats holds the statistics for the requests
type Stats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	StatusCodes        map[int]int64
	TotalDuration      time.Duration
	MinDuration        time.Duration
	MaxDuration        time.Duration
	mutex              sync.RWMutex
}

// RequestResult holds the result of a single request
type RequestResult struct {
	StatusCode int
	Duration   time.Duration
	Error      error
}

// RateLimitTester is the main struct for testing rate limits
type RateLimitTester struct {
	config      Config
	stats       *Stats
	rateLimiter RateLimiter
	client      *http.Client
}

// NewRateLimitTester creates a new rate limit tester
func NewRateLimitTester(config Config) *RateLimitTester {
	return &RateLimitTester{
		config:      config,
		stats:       NewStats(),
		rateLimiter: CreateRateLimiter(config.Algorithm, config.RequestsPerSecond),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewStats creates a new stats instance
func NewStats() *Stats {
	return &Stats{
		StatusCodes: make(map[int]int64),
		MinDuration: time.Duration(1<<63 - 1), // max duration
	}
}

// UpdateStats updates the statistics with a request result
func (s *Stats) UpdateStats(result RequestResult) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.TotalRequests++

	if result.Error != nil {
		s.FailedRequests++
	} else {
		s.SuccessfulRequests++
		s.StatusCodes[result.StatusCode]++
	}

	if result.Duration > 0 {
		if result.Duration < s.MinDuration {
			s.MinDuration = result.Duration
		}
		if result.Duration > s.MaxDuration {
			s.MaxDuration = result.Duration
		}
		s.TotalDuration += result.Duration
	}
}

// GetStats returns a copy of the current statistics
func (s *Stats) GetStats() Stats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Create a copy of status codes map
	statusCodes := make(map[int]int64)
	for k, v := range s.StatusCodes {
		statusCodes[k] = v
	}

	return Stats{
		TotalRequests:      s.TotalRequests,
		SuccessfulRequests: s.SuccessfulRequests,
		FailedRequests:     s.FailedRequests,
		StatusCodes:        statusCodes,
		TotalDuration:      s.TotalDuration,
		MinDuration:        s.MinDuration,
		MaxDuration:        s.MaxDuration,
	}
}

// sendRequest sends a single HTTP request
func (r *RateLimitTester) sendRequest(ctx context.Context) RequestResult {
	start := time.Now()

	// Create request body
	var body io.Reader
	if r.config.Body != "" {
		body = strings.NewReader(r.config.Body)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, r.config.Method, r.config.URL, body)
	if err != nil {
		return RequestResult{
			Error:    err,
			Duration: time.Since(start),
		}
	}

	// Add headers
	for key, value := range r.config.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := r.client.Do(req)
	if err != nil {
		return RequestResult{
			Error:    err,
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()

	// Read response body to completion
	io.Copy(io.Discard, resp.Body)

	return RequestResult{
		StatusCode: resp.StatusCode,
		Duration:   time.Since(start),
	}
}

// worker runs requests in a worker goroutine
func (r *RateLimitTester) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Wait for rate limiter
			if err := r.rateLimiter.Allow(ctx); err != nil {
				return
			}

			// Send request
			result := r.sendRequest(ctx)
			r.stats.UpdateStats(result)
		}
	}
}

// Run starts the rate limit test
func (r *RateLimitTester) Run() {
	fmt.Printf("Starting rate limit test...\n")
	fmt.Printf("Target URL: %s\n", r.config.URL)
	fmt.Printf("Rate: %.2f requests/second\n", r.config.RequestsPerSecond)
	fmt.Printf("Algorithm: %s\n", r.rateLimiter.String())
	fmt.Printf("Duration: %v\n", r.config.Duration)
	fmt.Printf("Concurrency: %d\n", r.config.Concurrency)
	fmt.Printf("Method: %s\n", r.config.Method)
	fmt.Println("----------------------------------------")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.config.Duration)
	defer cancel()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < r.config.Concurrency; i++ {
		wg.Add(1)
		go r.worker(ctx, &wg)
	}

	// Start stats reporter
	go r.reportStats(ctx)

	// Wait for completion
	wg.Wait()

	// Print final results
	fmt.Println("\n========================================")
	fmt.Println("Final Results:")
	r.printFinalStats()
}

// reportStats periodically reports statistics
func (r *RateLimitTester) reportStats(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := r.stats.GetStats()
			if stats.TotalRequests > 0 {
				fmt.Printf("Requests: %d | Success: %d | Failed: %d | Rate: %.2f req/s\n",
					stats.TotalRequests,
					stats.SuccessfulRequests,
					stats.FailedRequests,
					float64(stats.TotalRequests)/time.Since(time.Now().Add(-5*time.Second)).Seconds())
			}
		}
	}
}

// printFinalStats prints the final statistics
func (r *RateLimitTester) printFinalStats() {
	stats := r.stats.GetStats()

	fmt.Printf("Total Requests: %d\n", stats.TotalRequests)
	fmt.Printf("Successful Requests: %d\n", stats.SuccessfulRequests)
	fmt.Printf("Failed Requests: %d\n", stats.FailedRequests)

	if stats.TotalRequests > 0 {
		fmt.Printf("Success Rate: %.2f%%\n", float64(stats.SuccessfulRequests)/float64(stats.TotalRequests)*100)
	}

	fmt.Println("\nStatus Code Distribution:")
	for code, count := range stats.StatusCodes {
		fmt.Printf("  %d: %d requests\n", code, count)
	}

	if stats.SuccessfulRequests > 0 {
		fmt.Printf("\nResponse Times:\n")
		fmt.Printf("  Min: %v\n", stats.MinDuration)
		fmt.Printf("  Max: %v\n", stats.MaxDuration)
		fmt.Printf("  Avg: %v\n", stats.TotalDuration/time.Duration(stats.SuccessfulRequests))
	}

	actualRate := float64(stats.TotalRequests) / r.config.Duration.Seconds()
	fmt.Printf("\nActual Rate: %.2f requests/second\n", actualRate)
	fmt.Printf("Target Rate: %.2f requests/second\n", r.config.RequestsPerSecond)
	fmt.Printf("Algorithm Used: %s\n", r.rateLimiter.String())
}

// parseHeaders parses header string in format "key1:value1,key2:value2"
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return headers
}

func main() {
	// Define command line flags
	var (
		url         = flag.String("url", "", "Target URL to test (required)")
		rate        = flag.Float64("rate", 1.0, "Requests per second")
		algorithm   = flag.String("algorithm", "token-bucket", "Rate limiting algorithm (token-bucket, leaky-bucket, fixed-window, sliding-window-log, sliding-window-counter)")
		duration    = flag.Duration("duration", 10*time.Second, "Duration to run the test")
		concurrency = flag.Int("concurrency", 1, "Number of concurrent workers")
		method      = flag.String("method", "GET", "HTTP method")
		headers     = flag.String("headers", "", "HTTP headers in format 'key1:value1,key2:value2'")
		body        = flag.String("body", "", "Request body")
		configFile  = flag.String("config", "", "JSON config file path")
		help        = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *help {
		fmt.Println("Rate Limit Tester")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		fmt.Println("\nAvailable Algorithms:")
		fmt.Println("  token-bucket         - Token bucket algorithm (default)")
		fmt.Println("  leaky-bucket         - Leaky bucket algorithm")
		fmt.Println("  fixed-window         - Fixed window algorithm")
		fmt.Println("  sliding-window-log   - Sliding window log algorithm")
		fmt.Println("  sliding-window-counter - Sliding window counter algorithm")
		fmt.Println("\nExamples:")
		fmt.Println("  gorl -url=http://example.com -rate=10 -duration=30s")
		fmt.Println("  gorl -url=http://api.example.com -rate=5 -algorithm=leaky-bucket")
		fmt.Println("  gorl -url=http://api.example.com -rate=5 -concurrency=2 -method=POST -body='{\"test\":\"data\"}'")
		fmt.Println("  gorl -config=config.json")
		return
	}

	var config Config

	// Load config from file if specified
	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to read config file: %v", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			log.Fatalf("Failed to parse config file: %v", err)
		}
	} else {
		// Use command line arguments
		if *url == "" {
			log.Fatal("URL is required. Use -url flag or provide a config file.")
		}

		config = Config{
			URL:               *url,
			RequestsPerSecond: *rate,
			Algorithm:         RateLimiterType(*algorithm),
			Duration:          *duration,
			Concurrency:       *concurrency,
			Method:            strings.ToUpper(*method),
			Headers:           parseHeaders(*headers),
			Body:              *body,
		}
	}

	// Set default algorithm if not specified
	if config.Algorithm == "" {
		config.Algorithm = TokenBucket
	}

	// Validate algorithm
	validAlgorithms := []RateLimiterType{TokenBucket, LeakyBucket, FixedWindow, SlidingWindowLog, SlidingWindowCounter}
	isValidAlgorithm := false
	for _, validAlg := range validAlgorithms {
		if config.Algorithm == validAlg {
			isValidAlgorithm = true
			break
		}
	}
	if !isValidAlgorithm {
		log.Fatalf("Invalid algorithm: %s. Valid algorithms are: %v", config.Algorithm, validAlgorithms)
	}

	// Validate config
	if config.URL == "" {
		log.Fatal("URL is required")
	}
	if config.RequestsPerSecond <= 0 {
		log.Fatal("Rate must be greater than 0")
	}
	if config.Duration <= 0 {
		log.Fatal("Duration must be greater than 0")
	}
	if config.Concurrency <= 0 {
		log.Fatal("Concurrency must be greater than 0")
	}

	// Create and run the tester
	tester := NewRateLimitTester(config)
	tester.Run()
}
