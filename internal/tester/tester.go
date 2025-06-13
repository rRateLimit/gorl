package tester

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
	"github.com/rRateLimit/gorl/internal/limiter"
	"github.com/rRateLimit/gorl/internal/stats"
)

// RateLimitTester is the main struct for testing rate limits
type RateLimitTester struct {
	config      *config.Config
	stats       *stats.Stats
	rateLimiter limiter.RateLimiter
	client      *http.Client

	// Display options
	showLiveStats  bool
	showCompact    bool
	reportInterval time.Duration
}

// TesterOptions holds options for the tester
type TesterOptions struct {
	ShowLiveStats  bool
	ShowCompact    bool
	ReportInterval time.Duration
}

// NewRateLimitTester creates a new rate limit tester
func NewRateLimitTester(cfg *config.Config, opts TesterOptions) *RateLimitTester {
	// Set defaults
	cfg.SetDefaults()

	// Create HTTP client with custom transport
	transport := &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     cfg.DisableKeepAlives,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		DialContext: (&net.Dialer{
			Timeout: cfg.ConnectTimeout,
			KeepAlive: func() time.Duration {
				if cfg.TCPKeepAlive {
					return cfg.TCPKeepAlivePeriod
				}
				return -1 // Disable keep-alive
			}(),
		}).DialContext,
	}

	client := &http.Client{
		Timeout:   cfg.HTTPTimeout,
		Transport: transport,
	}

	// Set default report interval
	reportInterval := opts.ReportInterval
	if reportInterval == 0 {
		reportInterval = 2 * time.Second
	}

	return &RateLimitTester{
		config:         cfg,
		stats:          stats.NewStats(),
		rateLimiter:    limiter.NewRateLimiter(cfg.Algorithm, cfg.RequestsPerSecond),
		client:         client,
		showLiveStats:  opts.ShowLiveStats,
		showCompact:    opts.ShowCompact,
		reportInterval: reportInterval,
	}
}

// sendRequest sends a single HTTP request
func (r *RateLimitTester) sendRequest(ctx context.Context) stats.RequestResult {
	start := time.Now()

	// Create request body
	var body io.Reader
	if r.config.Body != "" {
		body = strings.NewReader(r.config.Body)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, r.config.Method, r.config.URL, body)
	if err != nil {
		return stats.RequestResult{
			Error:     err,
			Duration:  time.Since(start),
			Timestamp: start,
		}
	}

	// Add headers
	for key, value := range r.config.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := r.client.Do(req)
	if err != nil {
		return stats.RequestResult{
			Error:     err,
			Duration:  time.Since(start),
			Timestamp: start,
		}
	}
	defer resp.Body.Close()

	// Read response body to completion
	io.Copy(io.Discard, resp.Body)

	return stats.RequestResult{
		StatusCode: resp.StatusCode,
		Duration:   time.Since(start),
		Timestamp:  start,
	}
}

// worker runs requests in a worker goroutine
func (r *RateLimitTester) worker(ctx context.Context, wg *sync.WaitGroup, workerID int) {
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

// printHeader prints the test configuration header
func (r *RateLimitTester) printHeader() {
	fmt.Printf("Starting rate limit test...\n")
	fmt.Printf("Target URL: %s\n", r.config.URL)
	fmt.Printf("Rate: %.2f requests/second\n", r.config.RequestsPerSecond)
	fmt.Printf("Algorithm: %s\n", r.rateLimiter.String())
	fmt.Printf("Duration: %v\n", r.config.Duration)
	fmt.Printf("Concurrency: %d\n", r.config.Concurrency)
	fmt.Printf("Method: %s\n", r.config.Method)

	// Print HTTP/TCP settings
	fmt.Printf("HTTP Timeout: %v\n", r.config.HTTPTimeout)
	fmt.Printf("Connect Timeout: %v\n", r.config.ConnectTimeout)
	fmt.Printf("TLS Handshake Timeout: %v\n", r.config.TLSHandshakeTimeout)
	fmt.Printf("Response Header Timeout: %v\n", r.config.ResponseHeaderTimeout)
	fmt.Printf("TCP Keep-Alive: %v", r.config.TCPKeepAlive)
	if r.config.TCPKeepAlive {
		fmt.Printf(" (period: %v)", r.config.TCPKeepAlivePeriod)
	}
	fmt.Println()
	fmt.Printf("Max Idle Connections: %d (per host: %d)\n", r.config.MaxIdleConns, r.config.MaxIdleConnsPerHost)
	fmt.Printf("Disable Keep-Alives: %v\n", r.config.DisableKeepAlives)

	if r.showLiveStats {
		fmt.Printf("Live Stats: Enabled (update interval: %v)\n", r.reportInterval)
	} else if r.showCompact {
		fmt.Printf("Compact Stats: Enabled (update interval: %v)\n", r.reportInterval)
	}

	fmt.Println("----------------------------------------")
}

// startLiveReporter starts the live statistics reporter
func (r *RateLimitTester) startLiveReporter(ctx context.Context) {
	if !r.showLiveStats && !r.showCompact {
		return
	}

	ticker := time.NewTicker(r.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.showLiveStats {
				// Clear screen and show detailed stats
				fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
				r.printHeader()
				fmt.Print(r.stats.FormatLiveStats())
			} else if r.showCompact {
				// Show compact one-line stats
				fmt.Printf("\r%s", r.stats.FormatCompactStats())
			}
		}
	}
}

// printFinalStats prints the final statistics
func (r *RateLimitTester) printFinalStats() {
	if r.showCompact {
		fmt.Println() // New line after compact stats
	}

	fmt.Println("\n========================================")
	fmt.Println("Final Results:")

	finalStats := r.stats.GetFinalStats()

	fmt.Printf("Total Requests: %d\n", finalStats.TotalRequests)
	fmt.Printf("Successful Requests: %d\n", finalStats.SuccessfulRequests)
	fmt.Printf("Failed Requests: %d\n", finalStats.FailedRequests)
	fmt.Printf("Success Rate: %.2f%%\n", finalStats.SuccessRate)

	fmt.Println("\nStatus Code Distribution:")
	for code, count := range finalStats.StatusCodes {
		fmt.Printf("  %d: %d requests\n", code, count)
	}

	if finalStats.SuccessfulRequests > 0 {
		fmt.Printf("\nResponse Times:\n")
		fmt.Printf("  Min: %v\n", finalStats.MinResponseTime.Truncate(time.Millisecond))
		fmt.Printf("  Max: %v\n", finalStats.MaxResponseTime.Truncate(time.Millisecond))
		fmt.Printf("  Avg: %v\n", finalStats.AvgResponseTime.Truncate(time.Millisecond))
	}

	fmt.Printf("\nRate Analysis:\n")
	fmt.Printf("  Target Rate: %.2f requests/second\n", r.config.RequestsPerSecond)
	fmt.Printf("  Average Rate: %.2f requests/second\n", finalStats.AverageRate)
	fmt.Printf("  Test Duration: %v\n", finalStats.ElapsedTime.Truncate(time.Second))
	fmt.Printf("  Algorithm Used: %s\n", r.rateLimiter.String())
}

// Run starts the rate limit test
func (r *RateLimitTester) Run() error {
	// Validate configuration
	if err := r.config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %v", err)
	}

	// Print header
	r.printHeader()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.config.Duration)
	defer cancel()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < r.config.Concurrency; i++ {
		wg.Add(1)
		go r.worker(ctx, &wg, i+1)
	}

	// Start live stats reporter
	go r.startLiveReporter(ctx)

	// Wait for completion
	wg.Wait()

	// Print final results
	r.printFinalStats()

	return nil
}

// GetStats returns the current statistics
func (r *RateLimitTester) GetStats() *stats.Stats {
	return r.stats
}

// Stop gracefully stops the test
func (r *RateLimitTester) Stop() {
	// This would be implemented if we need graceful shutdown
	// For now, canceling the context in Run() handles this
}
