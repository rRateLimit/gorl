package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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

// Config holds the configuration for the rate limiter
type Config struct {
	URL               string            `json:"url"`               // target URL
	RequestsPerSecond float64           `json:"requestsPerSecond"` // requests per second
	Duration          time.Duration     `json:"duration"`          // total duration to run
	Concurrency       int               `json:"concurrency"`       // number of concurrent workers
	Method            string            `json:"method"`            // HTTP method
	Headers           map[string]string `json:"headers"`           // HTTP headers
	Body              string            `json:"body"`              // request body
	Algorithm         RateLimiterType   `json:"algorithm"`         // rate limiting algorithm

	// HTTP/TCP settings
	HTTPTimeout         time.Duration `json:"httpTimeout"`         // HTTP request timeout
	TCPKeepAlive        bool          `json:"tcpKeepAlive"`        // TCP keep-alive
	TCPKeepAlivePeriod  time.Duration `json:"tcpKeepAlivePeriod"`  // TCP keep-alive period
	DisableKeepAlives   bool          `json:"disableKeepAlives"`   // Disable HTTP keep-alives
	MaxIdleConns        int           `json:"maxIdleConns"`        // Maximum idle connections
	MaxIdleConnsPerHost int           `json:"maxIdleConnsPerHost"` // Maximum idle connections per host
}

// UnmarshalJSON implements custom JSON unmarshaling for Config
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		Duration           string `json:"duration"`
		Algorithm          string `json:"algorithm"`
		HTTPTimeout        string `json:"httpTimeout"`
		TCPKeepAlivePeriod string `json:"tcpKeepAlivePeriod"`
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

	if aux.HTTPTimeout != "" {
		timeout, err := time.ParseDuration(aux.HTTPTimeout)
		if err != nil {
			return fmt.Errorf("invalid httpTimeout format: %v", err)
		}
		c.HTTPTimeout = timeout
	}

	if aux.TCPKeepAlivePeriod != "" {
		period, err := time.ParseDuration(aux.TCPKeepAlivePeriod)
		if err != nil {
			return fmt.Errorf("invalid tcpKeepAlivePeriod format: %v", err)
		}
		c.TCPKeepAlivePeriod = period
	}

	return nil
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Apply environment variable overrides
	applyEnvironmentOverrides(&config)

	return &config, nil
}

// LoadFromFlags creates configuration from command line flags
func LoadFromFlags(url string, rate float64, algorithm string, duration time.Duration,
	concurrency int, method string, headers string, body string,
	httpTimeout time.Duration, tcpKeepAlive bool, tcpKeepAlivePeriod time.Duration,
	disableKeepAlives bool, maxIdleConns int, maxIdleConnsPerHost int) *Config {

	config := &Config{
		URL:                 url,
		RequestsPerSecond:   rate,
		Algorithm:           RateLimiterType(algorithm),
		Duration:            duration,
		Concurrency:         concurrency,
		Method:              strings.ToUpper(method),
		Headers:             parseHeaders(headers),
		Body:                body,
		HTTPTimeout:         httpTimeout,
		TCPKeepAlive:        tcpKeepAlive,
		TCPKeepAlivePeriod:  tcpKeepAlivePeriod,
		DisableKeepAlives:   disableKeepAlives,
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
	}

	// Apply environment variable overrides
	applyEnvironmentOverrides(config)

	return config
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.Algorithm == "" {
		c.Algorithm = TokenBucket
	}
	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = 30 * time.Second
	}
	if c.TCPKeepAlivePeriod == 0 {
		c.TCPKeepAlivePeriod = 30 * time.Second
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 100
	}
	if c.MaxIdleConnsPerHost == 0 {
		c.MaxIdleConnsPerHost = 10
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("URL is required")
	}
	if c.RequestsPerSecond <= 0 {
		return fmt.Errorf("rate must be greater than 0")
	}
	if c.Duration <= 0 {
		return fmt.Errorf("duration must be greater than 0")
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}

	// Validate algorithm
	validAlgorithms := []RateLimiterType{TokenBucket, LeakyBucket, FixedWindow, SlidingWindowLog, SlidingWindowCounter}
	isValidAlgorithm := false
	for _, validAlg := range validAlgorithms {
		if c.Algorithm == validAlg {
			isValidAlgorithm = true
			break
		}
	}
	if !isValidAlgorithm {
		return fmt.Errorf("invalid algorithm: %s. Valid algorithms are: %v", c.Algorithm, validAlgorithms)
	}

	return nil
}

// applyEnvironmentOverrides applies environment variable overrides to config
func applyEnvironmentOverrides(config *Config) {
	if url := os.Getenv("GORL_URL"); url != "" {
		config.URL = url
	}
	if rate := os.Getenv("GORL_RATE"); rate != "" {
		if r, err := strconv.ParseFloat(rate, 64); err == nil {
			config.RequestsPerSecond = r
		}
	}
	if algorithm := os.Getenv("GORL_ALGORITHM"); algorithm != "" {
		config.Algorithm = RateLimiterType(algorithm)
	}
	if duration := os.Getenv("GORL_DURATION"); duration != "" {
		if d, err := time.ParseDuration(duration); err == nil {
			config.Duration = d
		}
	}
	if concurrency := os.Getenv("GORL_CONCURRENCY"); concurrency != "" {
		if c, err := strconv.Atoi(concurrency); err == nil {
			config.Concurrency = c
		}
	}
	if method := os.Getenv("GORL_METHOD"); method != "" {
		config.Method = strings.ToUpper(method)
	}
	if headers := os.Getenv("GORL_HEADERS"); headers != "" {
		config.Headers = parseHeaders(headers)
	}
	if body := os.Getenv("GORL_BODY"); body != "" {
		config.Body = body
	}
	if timeout := os.Getenv("GORL_HTTP_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTPTimeout = t
		}
	}
	if keepAlive := os.Getenv("GORL_TCP_KEEP_ALIVE"); keepAlive != "" {
		if ka, err := strconv.ParseBool(keepAlive); err == nil {
			config.TCPKeepAlive = ka
		}
	}
	if keepAlivePeriod := os.Getenv("GORL_TCP_KEEP_ALIVE_PERIOD"); keepAlivePeriod != "" {
		if kap, err := time.ParseDuration(keepAlivePeriod); err == nil {
			config.TCPKeepAlivePeriod = kap
		}
	}
	if disableKeepAlives := os.Getenv("GORL_DISABLE_KEEP_ALIVES"); disableKeepAlives != "" {
		if dka, err := strconv.ParseBool(disableKeepAlives); err == nil {
			config.DisableKeepAlives = dka
		}
	}
	if maxIdleConns := os.Getenv("GORL_MAX_IDLE_CONNS"); maxIdleConns != "" {
		if mic, err := strconv.Atoi(maxIdleConns); err == nil {
			config.MaxIdleConns = mic
		}
	}
	if maxIdleConnsPerHost := os.Getenv("GORL_MAX_IDLE_CONNS_PER_HOST"); maxIdleConnsPerHost != "" {
		if micph, err := strconv.Atoi(maxIdleConnsPerHost); err == nil {
			config.MaxIdleConnsPerHost = micph
		}
	}
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
