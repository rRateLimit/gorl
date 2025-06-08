package test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/limiter"
	"github.com/rRateLimit/gorl/internal/stats"
	"github.com/rRateLimit/gorl/internal/tester"
)

// Integration tests that verify all components work together

func TestEndToEndBasicFlow(t *testing.T) {
	// Test the complete flow from config loading to test execution
	testServer := NewTestServer()
	defer testServer.Close()

	// Create a configuration
	cfg := &config.Config{
		URL:               testServer.URL() + "/integration",
		RequestsPerSecond: 10.0,            // Higher rate for faster test
		Duration:          1 * time.Second, // Reduced duration
		Concurrency:       2,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
		Headers: map[string]string{
			"User-Agent": "GoRL-Integration-Test",
		},
	}

	// Set defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	// Create tester
	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 200 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	// Run the test
	start := time.Now()
	err := rateTester.Run()
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Integration test failed: %v", err)
	}

	// Verify timing is approximately correct
	expectedDuration := cfg.Duration
	tolerance := 500 * time.Millisecond

	if elapsed < expectedDuration-tolerance || elapsed > expectedDuration+tolerance {
		t.Errorf("Test duration unexpected: got %v, expected ~%v", elapsed, expectedDuration)
	}

	// Verify stats
	finalStats := rateTester.GetStats().GetLiveStats()

	if finalStats.TotalRequests == 0 {
		t.Error("Expected requests to be made")
	}

	if finalStats.SuccessfulRequests == 0 {
		t.Error("Expected successful requests")
	}

	// Verify rate is approximately correct
	expectedRequests := cfg.RequestsPerSecond * cfg.Duration.Seconds()
	tolerance_requests := expectedRequests * 0.3 // 30% tolerance

	if float64(finalStats.TotalRequests) < expectedRequests-tolerance_requests ||
		float64(finalStats.TotalRequests) > expectedRequests+tolerance_requests {
		t.Errorf("Request count unexpected: got %d, expected ~%.0f",
			finalStats.TotalRequests, expectedRequests)
	}
}

func TestConfigFileIntegration(t *testing.T) {
	// Test loading configuration from JSON file
	testServer := NewTestServer()
	defer testServer.Close()

	// Create temporary config file
	configData := map[string]interface{}{
		"url":               testServer.URL() + "/config-test",
		"requestsPerSecond": 3.0,
		"duration":          "3s",
		"concurrency":       1,
		"algorithm":         "leaky-bucket",
		"method":            "POST",
		"headers": map[string]string{
			"Content-Type": "application/json",
		},
		"body":                `{"test": true}`,
		"httpTimeout":         "5s",
		"tcpKeepAlive":        true,
		"tcpKeepAlivePeriod":  "30s",
		"disableKeepAlives":   false,
		"maxIdleConns":        50,
		"maxIdleConnsPerHost": 5,
	}

	configJSON, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "gorl-test-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(configJSON); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Load config from file
	cfg, err := config.LoadFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify config was loaded correctly
	if cfg.URL != testServer.URL()+"/config-test" {
		t.Errorf("Expected URL %s, got %s", testServer.URL()+"/config-test", cfg.URL)
	}

	if cfg.RequestsPerSecond != 3.0 {
		t.Errorf("Expected rate 3.0, got %f", cfg.RequestsPerSecond)
	}

	if cfg.Duration != 3*time.Second {
		t.Errorf("Expected duration 3s, got %v", cfg.Duration)
	}

	if cfg.Algorithm != config.LeakyBucket {
		t.Errorf("Expected algorithm leaky-bucket, got %s", cfg.Algorithm)
	}

	if cfg.HTTPTimeout != 5*time.Second {
		t.Errorf("Expected HTTP timeout 5s, got %v", cfg.HTTPTimeout)
	}

	// Run test with loaded config
	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 200 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)
	err = rateTester.Run()

	if err != nil {
		t.Errorf("Test with config file failed: %v", err)
	}

	// Verify test ran successfully
	finalStats := rateTester.GetStats().GetLiveStats()
	if finalStats.TotalRequests == 0 {
		t.Error("Expected requests with config file")
	}
}

func TestEnvironmentVariableIntegration(t *testing.T) {
	// Test environment variable override functionality
	testServer := NewTestServer()
	defer testServer.Close()

	// Set environment variables
	envVars := map[string]string{
		"GORL_URL":                     testServer.URL() + "/env-test",
		"GORL_RATE":                    "4.0",
		"GORL_ALGORITHM":               "fixed-window",
		"GORL_DURATION":                "2s",
		"GORL_CONCURRENCY":             "1",
		"GORL_METHOD":                  "GET",
		"GORL_HTTP_TIMEOUT":            "10s",
		"GORL_TCP_KEEP_ALIVE":          "false",
		"GORL_DISABLE_KEEP_ALIVES":     "true",
		"GORL_MAX_IDLE_CONNS":          "200",
		"GORL_MAX_IDLE_CONNS_PER_HOST": "20",
	}

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Clean up environment variables
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	// Create config with defaults (should be overridden by env vars)
	cfg := config.LoadFromFlags("https://default.com", 1.0, "token-bucket", 10*time.Second,
		1, "POST", "", "", 30*time.Second, true, 30*time.Second, false, 100, 10)

	// Verify environment variables were applied
	if cfg.URL != testServer.URL()+"/env-test" {
		t.Errorf("Expected URL from env var, got %s", cfg.URL)
	}

	if cfg.RequestsPerSecond != 4.0 {
		t.Errorf("Expected rate 4.0 from env var, got %f", cfg.RequestsPerSecond)
	}

	if cfg.Algorithm != config.FixedWindow {
		t.Errorf("Expected algorithm fixed-window from env var, got %s", cfg.Algorithm)
	}

	if cfg.HTTPTimeout != 10*time.Second {
		t.Errorf("Expected HTTP timeout 10s from env var, got %v", cfg.HTTPTimeout)
	}

	if cfg.TCPKeepAlive != false {
		t.Errorf("Expected TCP keep-alive false from env var, got %v", cfg.TCPKeepAlive)
	}

	// Run test with env-configured setup
	cfg.SetDefaults()
	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 200 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)
	err := rateTester.Run()

	if err != nil {
		t.Errorf("Test with environment variables failed: %v", err)
	}

	// Verify test ran with env config
	finalStats := rateTester.GetStats().GetLiveStats()
	if finalStats.TotalRequests == 0 {
		t.Error("Expected requests with environment variables")
	}
}

func TestAllAlgorithmsIntegration(t *testing.T) {
	// Test that all rate limiting algorithms work in integration
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
				URL:               testServer.URL() + "/algo/" + string(algorithm),
				RequestsPerSecond: 15.0,            // Higher rate
				Duration:          1 * time.Second, // Reduced duration
				Concurrency:       1,
				Algorithm:         algorithm,
				Method:            "GET",
			}

			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 200 * time.Millisecond,
			}

			rateTester := tester.NewRateLimitTester(cfg, opts)

			start := time.Now()
			err := rateTester.Run()
			elapsed := time.Since(start)

			if err != nil {
				t.Errorf("Algorithm %s failed: %v", algorithm, err)
			}

			// Verify timing
			expectedDuration := cfg.Duration
			tolerance := 500 * time.Millisecond

			if elapsed < expectedDuration-tolerance || elapsed > expectedDuration+tolerance {
				t.Errorf("Algorithm %s timing unexpected: got %v, expected ~%v",
					algorithm, elapsed, expectedDuration)
			}

			// Verify requests were made
			finalStats := rateTester.GetStats().GetLiveStats()
			if finalStats.TotalRequests == 0 {
				t.Errorf("Algorithm %s made no requests", algorithm)
			}

			// Verify rate is reasonable (within 50% of target)
			expectedRequests := cfg.RequestsPerSecond * cfg.Duration.Seconds()
			minExpected := expectedRequests * 0.5
			maxExpected := expectedRequests * 1.5

			// Fixed window can process more requests if test spans window boundaries
			if algorithm == config.FixedWindow {
				maxExpected = expectedRequests * 2.5 // Allow up to 2.5x for fixed window
			}

			if float64(finalStats.TotalRequests) < minExpected ||
				float64(finalStats.TotalRequests) > maxExpected {
				t.Errorf("Algorithm %s request count unreasonable: got %d, expected ~%.0f",
					algorithm, finalStats.TotalRequests, expectedRequests)
			}
		})
	}
}

func TestHighConcurrencyIntegration(t *testing.T) {
	// Test high concurrency scenarios
	testServer := NewTestServer()
	defer testServer.Close()

	cfg := &config.Config{
		URL:               testServer.URL() + "/concurrent",
		RequestsPerSecond: 40.0,            // Higher rate
		Duration:          1 * time.Second, // Reduced duration
		Concurrency:       5,               // High concurrency
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 200 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	start := time.Now()
	err := rateTester.Run()
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("High concurrency test failed: %v", err)
	}

	// Verify timing
	expectedDuration := cfg.Duration
	tolerance := 500 * time.Millisecond

	if elapsed < expectedDuration-tolerance || elapsed > expectedDuration+tolerance {
		t.Errorf("High concurrency timing unexpected: got %v, expected ~%v", elapsed, expectedDuration)
	}

	// Verify many requests were made
	finalStats := rateTester.GetStats().GetLiveStats()
	if finalStats.TotalRequests < 30 { // Should make many requests with high concurrency and rate
		t.Errorf("Expected many requests with high concurrency, got %d", finalStats.TotalRequests)
	}

	// Verify server received all requests
	serverRequestCount := testServer.GetRequestCount()
	if int64(finalStats.TotalRequests) != serverRequestCount {
		t.Errorf("Request count mismatch: stats=%d, server=%d",
			finalStats.TotalRequests, serverRequestCount)
	}
}

func TestErrorHandlingIntegration(t *testing.T) {
	// Test error scenarios in integration
	testServer := NewTestServer()
	defer testServer.Close()

	// Configure server to return errors
	testServer.SetResponse(TestResponse{
		Status: 500,
		Body:   "Internal Server Error",
	})

	cfg := &config.Config{
		URL:               testServer.URL() + "/errors",
		RequestsPerSecond: 5.0,
		Duration:          2 * time.Second,
		Concurrency:       1,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}

	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 200 * time.Millisecond,
	}

	rateTester := tester.NewRateLimitTester(cfg, opts)

	err := rateTester.Run()
	if err != nil {
		t.Errorf("Error handling test failed: %v", err)
	}

	// Should still complete successfully even with server errors
	finalStats := rateTester.GetStats().GetLiveStats()

	if finalStats.TotalRequests == 0 {
		t.Error("Expected requests even with server errors")
	}

	// All requests should be "successful" from HTTP perspective but with 500 status
	if finalStats.SuccessfulRequests == 0 {
		t.Error("Expected successful HTTP requests (even with 500 status)")
	}

	// Should record 500 status codes
	if finalStats.StatusCodes[500] == 0 {
		t.Error("Expected 500 status codes in stats")
	}
}

func TestStatsAccuracyIntegration(t *testing.T) {
	// Test that statistics are accurate across all components
	testServer := NewTestServer()
	defer testServer.Close()

	cfg := &config.Config{
		URL:               testServer.URL() + "/stats-test",
		RequestsPerSecond: 8.0,
		Duration:          2 * time.Second,
		Concurrency:       2,
		Algorithm:         config.LeakyBucket,
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
		t.Errorf("Stats accuracy test failed: %v", err)
	}

	finalStats := rateTester.GetStats().GetLiveStats()
	serverRequestCount := testServer.GetRequestCount()

	// Verify stats accuracy
	if int64(finalStats.TotalRequests) != serverRequestCount {
		t.Errorf("Total request count mismatch: stats=%d, server=%d",
			finalStats.TotalRequests, serverRequestCount)
	}

	if finalStats.SuccessfulRequests != finalStats.TotalRequests {
		t.Errorf("Expected all requests to be successful: total=%d, successful=%d",
			finalStats.TotalRequests, finalStats.SuccessfulRequests)
	}

	if finalStats.FailedRequests != 0 {
		t.Errorf("Expected no failed requests, got %d", finalStats.FailedRequests)
	}

	if finalStats.SuccessRate != 100.0 {
		t.Errorf("Expected 100%% success rate, got %.2f%%", finalStats.SuccessRate)
	}

	// Verify status code distribution
	if finalStats.StatusCodes[200] != finalStats.SuccessfulRequests {
		t.Errorf("Status code 200 count mismatch: expected=%d, got=%d",
			finalStats.SuccessfulRequests, finalStats.StatusCodes[200])
	}

	// Verify response times are reasonable
	if finalStats.MinResponseTime == 0 {
		t.Error("Expected non-zero minimum response time")
	}

	if finalStats.MaxResponseTime == 0 {
		t.Error("Expected non-zero maximum response time")
	}

	if finalStats.AvgResponseTime == 0 {
		t.Error("Expected non-zero average response time")
	}

	// Min should be <= Avg <= Max
	if finalStats.MinResponseTime > finalStats.AvgResponseTime ||
		finalStats.AvgResponseTime > finalStats.MaxResponseTime {
		t.Errorf("Response time ordering incorrect: min=%v, avg=%v, max=%v",
			finalStats.MinResponseTime, finalStats.AvgResponseTime, finalStats.MaxResponseTime)
	}
}

// TestComponentInteraction tests how different components interact
func TestComponentInteraction(t *testing.T) {
	// Test direct interaction between limiter and stats
	rateLimiter := limiter.NewRateLimiter(config.TokenBucket, 10.0)
	statsCollector := stats.NewStats()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Simulate some requests
	for i := 0; i < 5; i++ {
		start := time.Now()

		err := rateLimiter.Allow(ctx)
		if err != nil {
			t.Errorf("Rate limiter error: %v", err)
		}

		// Simulate a successful request
		result := stats.RequestResult{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Timestamp:  start,
		}

		statsCollector.UpdateStats(result)
	}

	// Verify stats
	liveStats := statsCollector.GetLiveStats()

	if liveStats.TotalRequests != 5 {
		t.Errorf("Expected 5 requests, got %d", liveStats.TotalRequests)
	}

	if liveStats.SuccessfulRequests != 5 {
		t.Errorf("Expected 5 successful requests, got %d", liveStats.SuccessfulRequests)
	}

	// Test stats formatting
	liveStatsStr := statsCollector.FormatLiveStats()
	if !strings.Contains(liveStatsStr, "Total Requests: 5") {
		t.Error("Live stats formatting should contain total requests")
	}

	compactStatsStr := statsCollector.FormatCompactStats()
	if !strings.Contains(compactStatsStr, "Total: 5") {
		t.Error("Compact stats formatting should contain total requests")
	}
}
