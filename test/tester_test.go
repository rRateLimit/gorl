package test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/tester"
)

// TestServer provides utilities for testing HTTP requests
type TestServer struct {
	server       *httptest.Server
	requestCount int64
	responses    []TestResponse
	delays       map[string]time.Duration
}

type TestResponse struct {
	Status  int
	Body    string
	Headers map[string]string
}

func NewTestServer() *TestServer {
	ts := &TestServer{
		responses: []TestResponse{{Status: 200, Body: "OK"}},
		delays:    make(map[string]time.Duration),
	}

	ts.server = httptest.NewServer(http.HandlerFunc(ts.handler))
	return ts
}

func (ts *TestServer) handler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&ts.requestCount, 1)

	// Add delay if configured
	if delay, exists := ts.delays[r.URL.Path]; exists {
		time.Sleep(delay)
	}

	// Default response
	response := TestResponse{Status: 200, Body: "OK"}
	if len(ts.responses) > 0 {
		response = ts.responses[0]
	}

	// Set headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	w.WriteHeader(response.Status)
	w.Write([]byte(response.Body))
}

func (ts *TestServer) SetResponse(response TestResponse) {
	ts.responses = []TestResponse{response}
}

func (ts *TestServer) SetDelay(path string, delay time.Duration) {
	ts.delays[path] = delay
}

func (ts *TestServer) GetRequestCount() int64 {
	return atomic.LoadInt64(&ts.requestCount)
}

func (ts *TestServer) Close() {
	ts.server.Close()
}

func (ts *TestServer) URL() string {
	return ts.server.URL
}

func TestNewRateLimitTester(t *testing.T) {
	cfg := &config.Config{
		URL:               "https://example.com",
		RequestsPerSecond: 5.0,
		Duration:          10 * time.Second,
		Concurrency:       2,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 1 * time.Second,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)
	if rateTester == nil {
		t.Fatal("Expected non-nil rate limit tester")
	}

	stats := rateTester.GetStats()
	if stats == nil {
		t.Error("Expected non-nil stats")
	}
}

func TestRateLimitTesterBasicFunctionality(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	// Test successful requests
	testServer.SetResponse(TestResponse{
		Status:  200,
		Body:    `{"status": "ok"}`,
		Headers: map[string]string{"Content-Type": "application/json"},
	})

	cfg := &config.Config{
		URL:               testServer.URL() + "/test",
		RequestsPerSecond: 10.0, // High rate for quick test
		Duration:          2 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
		Headers:           map[string]string{"User-Agent": "Test-Client"},
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	// Run the test
	err := rateTester.Run()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check stats
	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	if stats.SuccessfulRequests == 0 {
		t.Error("Expected some successful requests")
	}

	if stats.FailedRequests != 0 {
		t.Errorf("Expected no failed requests, got %d", stats.FailedRequests)
	}

	// Verify server received requests
	serverRequestCount := testServer.GetRequestCount()
	if serverRequestCount == 0 {
		t.Error("Test server should have received requests")
	}

	// Check that actual requests match stats
	if int64(stats.TotalRequests) != serverRequestCount {
		t.Errorf("Stats mismatch: stats=%d, server=%d", stats.TotalRequests, serverRequestCount)
	}
}

func TestRateLimitTesterWithErrors(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	// Test error responses
	testServer.SetResponse(TestResponse{
		Status: 429, // Too Many Requests
		Body:   "Rate limit exceeded",
	})

	cfg := &config.Config{
		URL:               testServer.URL() + "/error",
		RequestsPerSecond: 5.0,
		Duration:          1 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	err := rateTester.Run()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	// All requests should be "successful" from HTTP perspective but with 429 status
	if stats.SuccessfulRequests == 0 {
		t.Error("Expected some successful requests (HTTP success)")
	}

	// Check status code distribution
	if stats.StatusCodes[429] == 0 {
		t.Error("Expected 429 status codes")
	}
}

func TestRateLimitTesterWithSlowServer(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	// Add delay to simulate slow server
	testServer.SetDelay("/slow", 200*time.Millisecond)

	cfg := &config.Config{
		URL:               testServer.URL() + "/slow",
		RequestsPerSecond: 10.0, // High rate
		Duration:          1 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
		HTTPTimeout:       500 * time.Millisecond, // Longer than delay
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	err := rateTester.Run()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	// Check response times
	if stats.MinResponseTime == 0 {
		t.Error("Expected non-zero response times")
	}

	// Response time should reflect the server delay
	if stats.MinResponseTime < 150*time.Millisecond {
		t.Errorf("Expected response time >= 150ms due to server delay, got %v", stats.MinResponseTime)
	}
}

func TestRateLimitTesterWithTimeout(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	// Add very long delay to cause timeout
	testServer.SetDelay("/timeout", 2*time.Second)

	cfg := &config.Config{
		URL:               testServer.URL() + "/timeout",
		RequestsPerSecond: 5.0,
		Duration:          1 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
		HTTPTimeout:       100 * time.Millisecond, // Shorter than delay
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	err := rateTester.Run()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	// Should have failed requests due to timeout
	if stats.FailedRequests == 0 {
		t.Error("Expected some failed requests due to timeout")
	}
}

func TestRateLimitTesterConcurrency(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	cfg := &config.Config{
		URL:               testServer.URL() + "/concurrent",
		RequestsPerSecond: 20.0, // High rate
		Duration:          2 * time.Second,
		Concurrency:       3, // Multiple workers
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	start := time.Now()
	err := rateTester.Run()
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Should complete in approximately the configured duration
	expectedDuration := cfg.Duration
	tolerance := 500 * time.Millisecond

	if elapsed < expectedDuration-tolerance || elapsed > expectedDuration+tolerance {
		t.Errorf("Test duration unexpected: got %v, expected ~%v", elapsed, expectedDuration)
	}

	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	// With 3 concurrent workers and high rate, should make many requests
	if stats.TotalRequests < 10 {
		t.Errorf("Expected more requests with concurrency, got %d", stats.TotalRequests)
	}
}

func TestRateLimitTesterDifferentAlgorithms(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	algorithms := []config.RateLimiterType{
		config.TokenBucket,
		config.LeakyBucket,
		config.FixedWindow,
		config.SlidingWindowLog,
		config.SlidingWindowCounter,
	}

	for _, algorithm := range algorithms {
		t.Run(string(algorithm), func(t *testing.T) {
			cfg := &config.Config{
				URL:               testServer.URL() + "/algo",
				RequestsPerSecond: 5.0,
				Duration:          1 * time.Second,
				Concurrency:       1,
				Algorithm:         algorithm,
				Method:            "GET",
			}

			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 100 * time.Millisecond,
			}

			rateTester := tester.NewRateLimitTester(cfg, opts)

			err := rateTester.Run()
			if err != nil {
				t.Errorf("Expected no error for %s, got %v", algorithm, err)
			}

			stats := rateTester.GetStats().GetLiveStats()

			if stats.TotalRequests == 0 {
				t.Errorf("Expected some requests for %s", algorithm)
			}
		})
	}
}

func TestRateLimitTesterConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 5.0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: false,
		},
		{
			name: "missing URL",
			config: &config.Config{
				RequestsPerSecond: 5.0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
		{
			name: "invalid rate",
			config: &config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tester.TesterOptions{}
			rateTester := tester.NewRateLimitTester(tt.config, opts)

			err := rateTester.Run()

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRateLimitTesterWithPOSTRequest(t *testing.T) {
	testServer := NewTestServer()
	defer testServer.Close()

	testBody := `{"test": "data", "timestamp": "2024-01-01"}`

	cfg := &config.Config{
		URL:               testServer.URL() + "/post",
		RequestsPerSecond: 5.0,
		Duration:          1 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"X-Test":       "header-value",
		},
		Body: testBody,
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 100 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	err := rateTester.Run()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats := rateTester.GetStats().GetLiveStats()

	if stats.TotalRequests == 0 {
		t.Error("Expected some requests to be made")
	}

	if stats.SuccessfulRequests == 0 {
		t.Error("Expected some successful requests")
	}
}

// Benchmark tests
func BenchmarkRateLimitTester(b *testing.B) {
	testServer := NewTestServer()
	defer testServer.Close()

	cfg := &config.Config{
		URL:               testServer.URL() + "/bench",
		RequestsPerSecond: 1000.0, // High rate for benchmarking
		Duration:          1 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 1 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateTester := tester.NewRateLimitTester(cfg, opts)
		rateTester.Run()
	}
}
