package test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/limiter"
	"github.com/rRateLimit/gorl/internal/stats"
	"github.com/rRateLimit/gorl/internal/tester"
)

// BenchmarkRateLimiters benchmarks different rate limiting algorithms
func BenchmarkRateLimiters(b *testing.B) {
	algorithms := []config.RateLimiterType{
		config.TokenBucket,
		config.LeakyBucket,
		config.FixedWindow,
		config.SlidingWindowLog,
		config.SlidingWindowCounter,
	}

	rates := []float64{10, 100, 1000}

	for _, algo := range algorithms {
		for _, rate := range rates {
			b.Run(fmt.Sprintf("%s/rate_%0.0f", algo, rate), func(b *testing.B) {
				rateLimiter := limiter.NewRateLimiter(algo, rate)
				ctx := context.Background()

				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						rateLimiter.Allow(ctx)
					}
				})
			})
		}
	}
}

// BenchmarkStatsCollection benchmarks statistics collection
func BenchmarkStatsCollection(b *testing.B) {
	statsCollector := stats.NewStats()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := stats.RequestResult{
				StatusCode: 200,
				Duration:   10 * time.Millisecond,
				Timestamp:  time.Now(),
			}
			statsCollector.UpdateStats(result)
		}
	})
}

// BenchmarkConcurrentRequests benchmarks concurrent request handling
func BenchmarkConcurrentRequests(b *testing.B) {
	concurrencyLevels := []int{1, 10, 50, 100}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency_%d", concurrency), func(b *testing.B) {
			// Create a test server
			testServer := NewTestServer()
			defer testServer.Close()

			cfg := &config.Config{
				URL:               testServer.URL() + "/bench",
				RequestsPerSecond: 1000, // High rate to avoid limiting
				Duration:          time.Duration(b.N) * time.Second,
				Concurrency:       concurrency,
				Algorithm:         config.TokenBucket,
				Method:            "GET",
			}
			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 1 * time.Hour, // Disable reporting
			}

			b.ResetTimer()

			// Run the test
			rateTester := tester.NewRateLimitTester(cfg, opts)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < b.N/concurrency; j++ {
						select {
						case <-ctx.Done():
							return
						default:
							rateTester.GetStats().UpdateStats(stats.RequestResult{
								StatusCode: 200,
								Duration:   1 * time.Millisecond,
								Timestamp:  time.Now(),
							})
						}
					}
				}()
			}
			wg.Wait()
		})
	}
}

// TestHighLoadPerformance tests performance under high load
func TestHighLoadPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	testServer := NewTestServer()
	defer testServer.Close()

	testCases := []struct {
		name        string
		rate        float64
		duration    time.Duration
		concurrency int
		algorithm   config.RateLimiterType
	}{
		{
			name:        "moderate_load",
			rate:        100,
			duration:    5 * time.Second,
			concurrency: 10,
			algorithm:   config.TokenBucket,
		},
		{
			name:        "high_load",
			rate:        500,
			duration:    5 * time.Second,
			concurrency: 50,
			algorithm:   config.TokenBucket,
		},
		{
			name:        "extreme_load",
			rate:        1000,
			duration:    5 * time.Second,
			concurrency: 100,
			algorithm:   config.TokenBucket,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				URL:               testServer.URL() + "/load-test",
				RequestsPerSecond: tc.rate,
				Duration:          tc.duration,
				Concurrency:       tc.concurrency,
				Algorithm:         tc.algorithm,
				Method:            "GET",
				HTTPTimeout:       5 * time.Second,
				ConnectTimeout:    2 * time.Second,
			}
			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 1 * time.Second,
			}

			// Record start metrics
			startTime := time.Now()
			startMem := getMemStats()

			// Run the test
			rateTester := tester.NewRateLimitTester(cfg, opts)
			err := rateTester.Run()
			if err != nil {
				t.Errorf("Performance test failed: %v", err)
			}

			// Record end metrics
			elapsed := time.Since(startTime)
			endMem := getMemStats()

			// Get final statistics
			finalStats := rateTester.GetStats().GetFinalStats()

			// Performance assertions
			expectedRequests := tc.rate * tc.duration.Seconds()
			tolerance := 0.2 // 20% tolerance

			if float64(finalStats.TotalRequests) < expectedRequests*(1-tolerance) {
				t.Errorf("Request count too low: got %d, expected at least %.0f",
					finalStats.TotalRequests, expectedRequests*(1-tolerance))
			}

			// Memory usage check
			memUsageMB := float64(endMem.Alloc-startMem.Alloc) / 1024 / 1024
			maxMemUsageMB := 100.0 // Maximum 100MB memory usage

			if memUsageMB > maxMemUsageMB {
				t.Errorf("Memory usage too high: %.2f MB (max: %.2f MB)", memUsageMB, maxMemUsageMB)
			}

			// Log performance metrics
			t.Logf("Performance Test: %s", tc.name)
			t.Logf("  Total Requests: %d", finalStats.TotalRequests)
			t.Logf("  Success Rate: %.2f%%", finalStats.SuccessRate)
			t.Logf("  Average Rate: %.2f req/s", finalStats.AverageRate)
			t.Logf("  Elapsed Time: %v", elapsed)
			t.Logf("  Memory Usage: %.2f MB", memUsageMB)
			t.Logf("  Avg Response Time: %v", finalStats.AvgResponseTime)
		})
	}
}

// TestAlgorithmPerformanceComparison compares performance of different algorithms
func TestAlgorithmPerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance comparison in short mode")
	}

	testServer := NewTestServer()
	defer testServer.Close()

	algorithms := []config.RateLimiterType{
		config.TokenBucket,
		config.LeakyBucket,
		config.FixedWindow,
		config.SlidingWindowLog,
		config.SlidingWindowCounter,
	}

	rate := 200.0
	duration := 3 * time.Second
	concurrency := 20

	results := make(map[config.RateLimiterType]performanceResult)

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			cfg := &config.Config{
				URL:               testServer.URL() + "/algo-perf",
				RequestsPerSecond: rate,
				Duration:          duration,
				Concurrency:       concurrency,
				Algorithm:         algo,
				Method:            "GET",
			}
			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 1 * time.Hour,
			}

			// Measure performance
			startTime := time.Now()
			startMem := getMemStats()

			rateTester := tester.NewRateLimitTester(cfg, opts)
			err := rateTester.Run()
			if err != nil {
				t.Errorf("Algorithm %s test failed: %v", algo, err)
				return
			}

			elapsed := time.Since(startTime)
			endMem := getMemStats()

			finalStats := rateTester.GetStats().GetFinalStats()

			// Store results
			results[algo] = performanceResult{
				totalRequests:   finalStats.TotalRequests,
				successRate:     finalStats.SuccessRate,
				avgResponseTime: finalStats.AvgResponseTime,
				memoryUsageMB:   float64(endMem.Alloc-startMem.Alloc) / 1024 / 1024,
				cpuTime:         elapsed,
			}
		})
	}

	// Compare and report results
	t.Log("\nAlgorithm Performance Comparison:")
	t.Log("=================================")
	for algo, result := range results {
		t.Logf("%s:", algo)
		t.Logf("  Requests: %d", result.totalRequests)
		t.Logf("  Success Rate: %.2f%%", result.successRate)
		t.Logf("  Avg Response: %v", result.avgResponseTime)
		t.Logf("  Memory: %.2f MB", result.memoryUsageMB)
		t.Logf("  CPU Time: %v", result.cpuTime)
	}
}

// TestMemoryLeaks tests for memory leaks during long-running operations
func TestMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	testServer := NewTestServer()
	defer testServer.Close()

	cfg := &config.Config{
		URL:               testServer.URL() + "/memory-test",
		RequestsPerSecond: 50,
		Duration:          10 * time.Second,
		Concurrency:       5,
		Algorithm:         config.TokenBucket,
		Method:            "GET",
	}
	cfg.SetDefaults()

	opts := tester.TesterOptions{
		ShowLiveStats:  false,
		ShowCompact:    false,
		ReportInterval: 1 * time.Second,
	}

	// Force garbage collection and record initial memory
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialMem := getMemStats()

	// Run the test
	rateTester := tester.NewRateLimitTester(cfg, opts)
	err := rateTester.Run()
	if err != nil {
		t.Fatalf("Memory leak test failed: %v", err)
	}

	// Force garbage collection and record final memory
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalMem := getMemStats()

	// Check for memory leaks
	memGrowthMB := float64(finalMem.Alloc-initialMem.Alloc) / 1024 / 1024
	maxGrowthMB := 10.0 // Maximum 10MB growth after GC

	if memGrowthMB > maxGrowthMB {
		t.Errorf("Potential memory leak detected: %.2f MB growth (max: %.2f MB)",
			memGrowthMB, maxGrowthMB)
	}

	t.Logf("Memory test completed:")
	t.Logf("  Initial memory: %.2f MB", float64(initialMem.Alloc)/1024/1024)
	t.Logf("  Final memory: %.2f MB", float64(finalMem.Alloc)/1024/1024)
	t.Logf("  Growth: %.2f MB", memGrowthMB)
}

// TestConcurrencyStress tests the system under various concurrency levels
func TestConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency stress test in short mode")
	}

	testServer := NewTestServer()
	defer testServer.Close()

	concurrencyLevels := []int{1, 10, 50, 100, 200}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("concurrency_%d", concurrency), func(t *testing.T) {
			cfg := &config.Config{
				URL:               testServer.URL() + "/concurrency-stress",
				RequestsPerSecond: float64(concurrency * 10),
				Duration:          3 * time.Second,
				Concurrency:       concurrency,
				Algorithm:         config.TokenBucket,
				Method:            "GET",
			}
			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 1 * time.Hour,
			}

			// Track goroutine count
			startGoroutines := runtime.NumGoroutine()

			rateTester := tester.NewRateLimitTester(cfg, opts)
			err := rateTester.Run()
			if err != nil {
				t.Errorf("Concurrency test failed at level %d: %v", concurrency, err)
				return
			}

			// Allow goroutines to clean up
			time.Sleep(100 * time.Millisecond)
			endGoroutines := runtime.NumGoroutine()

			// Check for goroutine leaks
			goroutineGrowth := endGoroutines - startGoroutines
			maxGrowth := 10 // Allow some growth for background tasks

			if goroutineGrowth > maxGrowth {
				t.Errorf("Potential goroutine leak: %d new goroutines (max: %d)",
					goroutineGrowth, maxGrowth)
			}

			finalStats := rateTester.GetStats().GetFinalStats()
			t.Logf("Concurrency %d: %d requests, %.2f%% success, %d goroutines",
				concurrency, finalStats.TotalRequests, finalStats.SuccessRate, endGoroutines)
		})
	}
}

// TestTimeoutPerformance tests performance with various timeout settings
func TestTimeoutPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout performance test in short mode")
	}

	testServer := NewTestServer()
	defer testServer.Close()

	// Add slow endpoint
	testServer.SetResponseDelay(50 * time.Millisecond)

	timeoutConfigs := []struct {
		name            string
		httpTimeout     time.Duration
		connectTimeout  time.Duration
		expectedSuccess bool
	}{
		{
			name:            "generous_timeouts",
			httpTimeout:     5 * time.Second,
			connectTimeout:  2 * time.Second,
			expectedSuccess: true,
		},
		{
			name:            "tight_timeouts",
			httpTimeout:     100 * time.Millisecond,
			connectTimeout:  50 * time.Millisecond,
			expectedSuccess: true,
		},
		{
			name:            "very_tight_timeouts",
			httpTimeout:     30 * time.Millisecond,
			connectTimeout:  10 * time.Millisecond,
			expectedSuccess: false,
		},
	}

	for _, tc := range timeoutConfigs {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				URL:                   testServer.URL() + "/timeout-perf",
				RequestsPerSecond:     20,
				Duration:              2 * time.Second,
				Concurrency:           5,
				Algorithm:             config.TokenBucket,
				Method:                "GET",
				HTTPTimeout:           tc.httpTimeout,
				ConnectTimeout:        tc.connectTimeout,
				TLSHandshakeTimeout:   tc.connectTimeout,
				ResponseHeaderTimeout: tc.httpTimeout / 2,
			}
			cfg.SetDefaults()

			opts := tester.TesterOptions{
				ShowLiveStats:  false,
				ShowCompact:    false,
				ReportInterval: 1 * time.Hour,
			}

			rateTester := tester.NewRateLimitTester(cfg, opts)
			err := rateTester.Run()
			if err != nil {
				t.Errorf("Timeout test failed: %v", err)
				return
			}

			finalStats := rateTester.GetStats().GetFinalStats()

			if tc.expectedSuccess && finalStats.SuccessRate < 90 {
				t.Errorf("Expected high success rate with %s, got %.2f%%",
					tc.name, finalStats.SuccessRate)
			} else if !tc.expectedSuccess && finalStats.SuccessRate > 50 {
				t.Errorf("Expected low success rate with %s, got %.2f%%",
					tc.name, finalStats.SuccessRate)
			}

			t.Logf("%s: %.2f%% success, avg response: %v",
				tc.name, finalStats.SuccessRate, finalStats.AvgResponseTime)
		})
	}
}

// Helper types and functions

type performanceResult struct {
	totalRequests   int64
	successRate     float64
	avgResponseTime time.Duration
	memoryUsageMB   float64
	cpuTime         time.Duration
}

func getMemStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// BenchmarkRateLimiterAllow benchmarks the Allow method for each algorithm
func BenchmarkRateLimiterAllow(b *testing.B) {
	algorithms := []config.RateLimiterType{
		config.TokenBucket,
		config.LeakyBucket,
		config.FixedWindow,
		config.SlidingWindowLog,
		config.SlidingWindowCounter,
	}

	for _, algo := range algorithms {
		b.Run(string(algo), func(b *testing.B) {
			rateLimiter := limiter.NewRateLimiter(algo, 1000)
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rateLimiter.Allow(ctx)
			}
		})
	}
}

// BenchmarkStatsUpdate benchmarks the stats update operation
func BenchmarkStatsUpdate(b *testing.B) {
	s := stats.NewStats()
	result := stats.RequestResult{
		StatusCode: 200,
		Duration:   10 * time.Millisecond,
		Timestamp:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.UpdateStats(result)
	}
}

// BenchmarkConcurrentStatsUpdate benchmarks concurrent stats updates
func BenchmarkConcurrentStatsUpdate(b *testing.B) {
	s := stats.NewStats()

	b.RunParallel(func(pb *testing.PB) {
		result := stats.RequestResult{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Timestamp:  time.Now(),
		}

		for pb.Next() {
			s.UpdateStats(result)
		}
	})
}

// TestRateLimiterAccuracy tests the accuracy of rate limiting
func TestRateLimiterAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping accuracy test in short mode")
	}

	algorithms := []config.RateLimiterType{
		config.TokenBucket,
		config.LeakyBucket,
		config.FixedWindow,
		config.SlidingWindowLog,
		config.SlidingWindowCounter,
	}

	targetRate := 50.0
	duration := 5 * time.Second

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			rateLimiter := limiter.NewRateLimiter(algo, targetRate)
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()

			var allowed int64
			var denied int64
			var wg sync.WaitGroup

			// Run multiple goroutines to stress test
			workers := 10
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
							err := rateLimiter.Allow(ctx)
							if err == nil {
								atomic.AddInt64(&allowed, 1)
							} else {
								atomic.AddInt64(&denied, 1)
							}
							time.Sleep(1 * time.Millisecond) // Small delay
						}
					}
				}()
			}

			wg.Wait()

			actualRate := float64(allowed) / duration.Seconds()
			tolerance := 0.15 // 15% tolerance

			if actualRate < targetRate*(1-tolerance) || actualRate > targetRate*(1+tolerance) {
				t.Errorf("%s: Rate outside tolerance. Target: %.2f, Actual: %.2f, Allowed: %d, Denied: %d",
					algo, targetRate, actualRate, allowed, denied)
			} else {
				t.Logf("%s: Target: %.2f req/s, Actual: %.2f req/s (Allowed: %d, Denied: %d)",
					algo, targetRate, actualRate, allowed, denied)
			}
		})
	}
}
