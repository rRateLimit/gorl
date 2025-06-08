package stats

import (
	"fmt"
	"sync"
	"time"
)

// RequestResult holds the result of a single request
type RequestResult struct {
	StatusCode int
	Duration   time.Duration
	Error      error
	Timestamp  time.Time
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
	StartTime          time.Time
	LastRequestTime    time.Time

	// For rate calculation
	RecentRequests []time.Time // Recent request timestamps for rate calculation

	mutex sync.RWMutex
}

// PeriodicStats represents statistics for a specific time period
type PeriodicStats struct {
	Period             time.Duration
	RequestCount       int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageRate        float64
	MinDuration        time.Duration
	MaxDuration        time.Duration
	AverageDuration    time.Duration
}

// LiveStats represents current live statistics
type LiveStats struct {
	ElapsedTime        time.Duration
	CurrentRate        float64
	AverageRate        float64
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	SuccessRate        float64
	MinResponseTime    time.Duration
	MaxResponseTime    time.Duration
	AvgResponseTime    time.Duration
	StatusCodes        map[int]int64
}

// NewStats creates a new stats instance
func NewStats() *Stats {
	return &Stats{
		StatusCodes:    make(map[int]int64),
		MinDuration:    time.Duration(1<<63 - 1), // max duration
		StartTime:      time.Now(),
		RecentRequests: make([]time.Time, 0),
	}
}

// UpdateStats updates the statistics with a request result
func (s *Stats) UpdateStats(result RequestResult) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.TotalRequests++
	s.LastRequestTime = result.Timestamp

	// Add to recent requests for rate calculation
	s.RecentRequests = append(s.RecentRequests, result.Timestamp)

	// Keep only recent requests (last 10 seconds)
	cutoff := result.Timestamp.Add(-10 * time.Second)
	validRequests := make([]time.Time, 0)
	for _, reqTime := range s.RecentRequests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	s.RecentRequests = validRequests

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

// GetLiveStats returns current live statistics
func (s *Stats) GetLiveStats() LiveStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	now := time.Now()
	elapsed := now.Sub(s.StartTime)

	// Calculate current rate (requests in last 10 seconds)
	currentRate := float64(len(s.RecentRequests)) / 10.0

	// Calculate average rate
	averageRate := float64(s.TotalRequests) / elapsed.Seconds()

	// Calculate success rate
	var successRate float64
	if s.TotalRequests > 0 {
		successRate = float64(s.SuccessfulRequests) / float64(s.TotalRequests) * 100
	}

	// Calculate average response time
	var avgResponseTime time.Duration
	if s.SuccessfulRequests > 0 {
		avgResponseTime = s.TotalDuration / time.Duration(s.SuccessfulRequests)
	}

	// Copy status codes map
	statusCodes := make(map[int]int64)
	for k, v := range s.StatusCodes {
		statusCodes[k] = v
	}

	return LiveStats{
		ElapsedTime:        elapsed,
		CurrentRate:        currentRate,
		AverageRate:        averageRate,
		TotalRequests:      s.TotalRequests,
		SuccessfulRequests: s.SuccessfulRequests,
		FailedRequests:     s.FailedRequests,
		SuccessRate:        successRate,
		MinResponseTime:    s.MinDuration,
		MaxResponseTime:    s.MaxDuration,
		AvgResponseTime:    avgResponseTime,
		StatusCodes:        statusCodes,
	}
}

// GetFinalStats returns final statistics for reporting
func (s *Stats) GetFinalStats() LiveStats {
	return s.GetLiveStats()
}

// Reset resets all statistics
func (s *Stats) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.TotalRequests = 0
	s.SuccessfulRequests = 0
	s.FailedRequests = 0
	s.StatusCodes = make(map[int]int64)
	s.TotalDuration = 0
	s.MinDuration = time.Duration(1<<63 - 1)
	s.MaxDuration = 0
	s.StartTime = time.Now()
	s.LastRequestTime = time.Time{}
	s.RecentRequests = make([]time.Time, 0)
}

// GetPeriodicStats returns statistics for a specific period
func (s *Stats) GetPeriodicStats(period time.Duration) PeriodicStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	now := time.Now()
	periodStart := now.Add(-period)

	// Count requests in the period
	var requestCount int64
	for _, reqTime := range s.RecentRequests {
		if reqTime.After(periodStart) {
			requestCount++
		}
	}

	rate := float64(requestCount) / period.Seconds()

	return PeriodicStats{
		Period:             period,
		RequestCount:       requestCount,
		SuccessfulRequests: s.SuccessfulRequests, // This would need more detailed tracking
		FailedRequests:     s.FailedRequests,     // This would need more detailed tracking
		AverageRate:        rate,
		MinDuration:        s.MinDuration,
		MaxDuration:        s.MaxDuration,
		AverageDuration:    s.TotalDuration / time.Duration(max(1, s.SuccessfulRequests)),
	}
}

// FormatLiveStats formats live statistics for display
func (s *Stats) FormatLiveStats() string {
	stats := s.GetLiveStats()

	result := fmt.Sprintf("=== Live Statistics ===\n")
	result += fmt.Sprintf("Elapsed Time: %v\n", stats.ElapsedTime.Truncate(time.Second))
	result += fmt.Sprintf("Total Requests: %d\n", stats.TotalRequests)
	result += fmt.Sprintf("Successful: %d | Failed: %d\n", stats.SuccessfulRequests, stats.FailedRequests)
	result += fmt.Sprintf("Success Rate: %.2f%%\n", stats.SuccessRate)
	result += fmt.Sprintf("Current Rate: %.2f req/s\n", stats.CurrentRate)
	result += fmt.Sprintf("Average Rate: %.2f req/s\n", stats.AverageRate)

	if stats.SuccessfulRequests > 0 {
		result += fmt.Sprintf("Response Times - Min: %v | Max: %v | Avg: %v\n",
			stats.MinResponseTime.Truncate(time.Millisecond),
			stats.MaxResponseTime.Truncate(time.Millisecond),
			stats.AvgResponseTime.Truncate(time.Millisecond))
	}

	if len(stats.StatusCodes) > 0 {
		result += "Status Codes: "
		for code, count := range stats.StatusCodes {
			result += fmt.Sprintf("%d:%d ", code, count)
		}
		result += "\n"
	}

	return result
}

// FormatCompactStats formats statistics in a compact single line
func (s *Stats) FormatCompactStats() string {
	stats := s.GetLiveStats()

	return fmt.Sprintf("Total: %d | Success: %d | Failed: %d | Rate: %.2f req/s | Success: %.1f%% | Elapsed: %v",
		stats.TotalRequests,
		stats.SuccessfulRequests,
		stats.FailedRequests,
		stats.CurrentRate,
		stats.SuccessRate,
		stats.ElapsedTime.Truncate(time.Second))
}

// max returns the maximum of two int64 values
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
