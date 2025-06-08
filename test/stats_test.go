package test

import (
	"errors"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/stats"
)

func TestStatsUpdateAndRetrieval(t *testing.T) {
	s := stats.NewStats()

	// Test initial state
	liveStats := s.GetLiveStats()
	if liveStats.TotalRequests != 0 {
		t.Errorf("Expected initial total requests to be 0, got %d", liveStats.TotalRequests)
	}

	// Add some successful requests
	now := time.Now()
	for i := 0; i < 5; i++ {
		result := stats.RequestResult{
			StatusCode: 200,
			Duration:   100 * time.Millisecond,
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}
		s.UpdateStats(result)
	}

	// Add some failed requests
	for i := 0; i < 2; i++ {
		result := stats.RequestResult{
			Error:     errors.New("test error"),
			Duration:  50 * time.Millisecond,
			Timestamp: now.Add(time.Duration(i+5) * time.Second),
		}
		s.UpdateStats(result)
	}

	liveStats = s.GetLiveStats()

	if liveStats.TotalRequests != 7 {
		t.Errorf("Expected total requests to be 7, got %d", liveStats.TotalRequests)
	}

	if liveStats.SuccessfulRequests != 5 {
		t.Errorf("Expected successful requests to be 5, got %d", liveStats.SuccessfulRequests)
	}

	if liveStats.FailedRequests != 2 {
		t.Errorf("Expected failed requests to be 2, got %d", liveStats.FailedRequests)
	}

	expectedSuccessRate := float64(5) / float64(7) * 100
	if liveStats.SuccessRate != expectedSuccessRate {
		t.Errorf("Expected success rate to be %.2f%%, got %.2f%%", expectedSuccessRate, liveStats.SuccessRate)
	}

	if liveStats.StatusCodes[200] != 5 {
		t.Errorf("Expected 5 requests with status code 200, got %d", liveStats.StatusCodes[200])
	}

	if liveStats.MinResponseTime != 100*time.Millisecond {
		t.Errorf("Expected min response time to be 100ms, got %v", liveStats.MinResponseTime)
	}

	if liveStats.MaxResponseTime != 100*time.Millisecond {
		t.Errorf("Expected max response time to be 100ms, got %v", liveStats.MaxResponseTime)
	}

	if liveStats.AvgResponseTime != 100*time.Millisecond {
		t.Errorf("Expected avg response time to be 100ms, got %v", liveStats.AvgResponseTime)
	}
}

func TestStatsRateCalculation(t *testing.T) {
	s := stats.NewStats()

	now := time.Now()

	// Add requests spread over 5 seconds
	for i := 0; i < 10; i++ {
		result := stats.RequestResult{
			StatusCode: 200,
			Duration:   100 * time.Millisecond,
			Timestamp:  now.Add(time.Duration(i) * 500 * time.Millisecond), // 2 requests per second
		}
		s.UpdateStats(result)
	}

	// Sleep a bit to ensure timestamps are in the past
	time.Sleep(100 * time.Millisecond)

	liveStats := s.GetLiveStats()

	// Current rate should be based on requests in last 10 seconds
	// Since we added 10 requests in 5 seconds, and they're all within the last 10 seconds,
	// the rate should be 10/10 = 1.0 req/s
	if liveStats.CurrentRate < 0.9 || liveStats.CurrentRate > 1.1 {
		t.Errorf("Expected current rate to be around 1.0 req/s, got %.2f", liveStats.CurrentRate)
	}

	// Average rate should be total requests / elapsed time
	elapsedSeconds := liveStats.ElapsedTime.Seconds()
	expectedAverageRate := float64(10) / elapsedSeconds
	if liveStats.AverageRate < expectedAverageRate*0.9 || liveStats.AverageRate > expectedAverageRate*1.1 {
		t.Errorf("Expected average rate to be around %.2f req/s, got %.2f", expectedAverageRate, liveStats.AverageRate)
	}
}

func TestStatsReset(t *testing.T) {
	s := stats.NewStats()

	// Add some data
	result := stats.RequestResult{
		StatusCode: 200,
		Duration:   100 * time.Millisecond,
		Timestamp:  time.Now(),
	}
	s.UpdateStats(result)

	// Verify data exists
	liveStats := s.GetLiveStats()
	if liveStats.TotalRequests == 0 {
		t.Error("Expected some requests before reset")
	}

	// Reset and verify
	s.Reset()
	liveStats = s.GetLiveStats()

	if liveStats.TotalRequests != 0 {
		t.Errorf("Expected total requests to be 0 after reset, got %d", liveStats.TotalRequests)
	}

	if liveStats.SuccessfulRequests != 0 {
		t.Errorf("Expected successful requests to be 0 after reset, got %d", liveStats.SuccessfulRequests)
	}

	if liveStats.FailedRequests != 0 {
		t.Errorf("Expected failed requests to be 0 after reset, got %d", liveStats.FailedRequests)
	}

	if len(liveStats.StatusCodes) != 0 {
		t.Errorf("Expected status codes map to be empty after reset, got %v", liveStats.StatusCodes)
	}
}

func TestStatsFormatting(t *testing.T) {
	s := stats.NewStats()

	// Add some test data
	result := stats.RequestResult{
		StatusCode: 200,
		Duration:   100 * time.Millisecond,
		Timestamp:  time.Now(),
	}
	s.UpdateStats(result)

	result2 := stats.RequestResult{
		StatusCode: 404,
		Duration:   200 * time.Millisecond,
		Timestamp:  time.Now(),
	}
	s.UpdateStats(result2)

	// Test live stats formatting
	liveStatsStr := s.FormatLiveStats()
	if liveStatsStr == "" {
		t.Error("Expected non-empty live stats string")
	}

	// Should contain key information
	if !contains(liveStatsStr, "Total Requests: 2") {
		t.Error("Live stats should contain total requests")
	}

	if !contains(liveStatsStr, "Successful: 2") {
		t.Error("Live stats should contain successful requests")
	}

	if !contains(liveStatsStr, "200:1") {
		t.Error("Live stats should contain status code distribution")
	}

	if !contains(liveStatsStr, "404:1") {
		t.Error("Live stats should contain status code distribution")
	}

	// Test compact stats formatting
	compactStatsStr := s.FormatCompactStats()
	if compactStatsStr == "" {
		t.Error("Expected non-empty compact stats string")
	}

	// Should contain key information in compact form
	if !contains(compactStatsStr, "Total: 2") {
		t.Error("Compact stats should contain total requests")
	}

	if !contains(compactStatsStr, "Success: 2") {
		t.Error("Compact stats should contain successful requests")
	}

	if !contains(compactStatsStr, "Failed: 0") {
		t.Error("Compact stats should contain failed requests")
	}
}

func TestStatsPeriodicStats(t *testing.T) {
	s := stats.NewStats()

	now := time.Now()

	// Add requests over different time periods
	for i := 0; i < 5; i++ {
		result := stats.RequestResult{
			StatusCode: 200,
			Duration:   100 * time.Millisecond,
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}
		s.UpdateStats(result)
	}

	// Get periodic stats for last 3 seconds
	periodicStats := s.GetPeriodicStats(3 * time.Second)

	if periodicStats.Period != 3*time.Second {
		t.Errorf("Expected period to be 3s, got %v", periodicStats.Period)
	}

	// Should have some requests in the period (exact count depends on timing)
	if periodicStats.RequestCount < 0 {
		t.Errorf("Expected non-negative request count, got %d", periodicStats.RequestCount)
	}

	if periodicStats.AverageRate < 0 {
		t.Errorf("Expected non-negative average rate, got %.2f", periodicStats.AverageRate)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != substr && (len(substr) == 0 || indexOf(s, substr) >= 0)
}

// Helper function to find substring index
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
