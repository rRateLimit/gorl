package test

import (
	"os"
	"testing"
	"time"

	"github.com/rRateLimit/gorl/internal/config"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: config.Config{
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
			config: config.Config{
				RequestsPerSecond: 5.0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
		{
			name: "invalid rate",
			config: config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
		{
			name: "invalid duration",
			config: config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 5.0,
				Duration:          0,
				Concurrency:       1,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
		{
			name: "invalid concurrency",
			config: config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 5.0,
				Duration:          10 * time.Second,
				Concurrency:       0,
				Algorithm:         config.TokenBucket,
			},
			wantErr: true,
		},
		{
			name: "invalid algorithm",
			config: config.Config{
				URL:               "https://example.com",
				RequestsPerSecond: 5.0,
				Duration:          10 * time.Second,
				Concurrency:       1,
				Algorithm:         "invalid-algorithm",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigSetDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetDefaults()

	if cfg.Algorithm != config.TokenBucket {
		t.Errorf("Expected default algorithm to be %s, got %s", config.TokenBucket, cfg.Algorithm)
	}

	if cfg.HTTPTimeout != 30*time.Second {
		t.Errorf("Expected default HTTP timeout to be 30s, got %v", cfg.HTTPTimeout)
	}

	if cfg.TCPKeepAlivePeriod != 30*time.Second {
		t.Errorf("Expected default TCP keep-alive period to be 30s, got %v", cfg.TCPKeepAlivePeriod)
	}

	if cfg.MaxIdleConns != 100 {
		t.Errorf("Expected default max idle connections to be 100, got %d", cfg.MaxIdleConns)
	}

	if cfg.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected default max idle connections per host to be 10, got %d", cfg.MaxIdleConnsPerHost)
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	// Set test environment variables
	os.Setenv("GORL_URL", "https://test.example.com")
	os.Setenv("GORL_RATE", "10.5")
	os.Setenv("GORL_ALGORITHM", "leaky-bucket")
	os.Setenv("GORL_DURATION", "60s")
	os.Setenv("GORL_CONCURRENCY", "5")
	os.Setenv("GORL_METHOD", "POST")
	os.Setenv("GORL_HEADERS", "Content-Type:application/json,Authorization:Bearer token")
	os.Setenv("GORL_BODY", "test body")
	os.Setenv("GORL_HTTP_TIMEOUT", "45s")
	os.Setenv("GORL_TCP_KEEP_ALIVE", "false")
	os.Setenv("GORL_TCP_KEEP_ALIVE_PERIOD", "60s")
	os.Setenv("GORL_DISABLE_KEEP_ALIVES", "true")
	os.Setenv("GORL_MAX_IDLE_CONNS", "200")
	os.Setenv("GORL_MAX_IDLE_CONNS_PER_HOST", "20")

	defer func() {
		// Clean up
		os.Unsetenv("GORL_URL")
		os.Unsetenv("GORL_RATE")
		os.Unsetenv("GORL_ALGORITHM")
		os.Unsetenv("GORL_DURATION")
		os.Unsetenv("GORL_CONCURRENCY")
		os.Unsetenv("GORL_METHOD")
		os.Unsetenv("GORL_HEADERS")
		os.Unsetenv("GORL_BODY")
		os.Unsetenv("GORL_HTTP_TIMEOUT")
		os.Unsetenv("GORL_TCP_KEEP_ALIVE")
		os.Unsetenv("GORL_TCP_KEEP_ALIVE_PERIOD")
		os.Unsetenv("GORL_DISABLE_KEEP_ALIVES")
		os.Unsetenv("GORL_MAX_IDLE_CONNS")
		os.Unsetenv("GORL_MAX_IDLE_CONNS_PER_HOST")
	}()

	cfg := config.LoadFromFlags("https://default.com", 1.0, "token-bucket", 10*time.Second,
		1, "GET", "", "", 30*time.Second, true, 30*time.Second, false, 100, 10)

	if cfg.URL != "https://test.example.com" {
		t.Errorf("Expected URL to be overridden by env var, got %s", cfg.URL)
	}

	if cfg.RequestsPerSecond != 10.5 {
		t.Errorf("Expected rate to be 10.5, got %f", cfg.RequestsPerSecond)
	}

	if cfg.Algorithm != config.LeakyBucket {
		t.Errorf("Expected algorithm to be leaky-bucket, got %s", cfg.Algorithm)
	}

	if cfg.Duration != 60*time.Second {
		t.Errorf("Expected duration to be 60s, got %v", cfg.Duration)
	}

	if cfg.Concurrency != 5 {
		t.Errorf("Expected concurrency to be 5, got %d", cfg.Concurrency)
	}

	if cfg.Method != "POST" {
		t.Errorf("Expected method to be POST, got %s", cfg.Method)
	}

	if cfg.Headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, got %s", cfg.Headers["Content-Type"])
	}

	if cfg.Headers["Authorization"] != "Bearer token" {
		t.Errorf("Expected Authorization header to be Bearer token, got %s", cfg.Headers["Authorization"])
	}

	if cfg.Body != "test body" {
		t.Errorf("Expected body to be 'test body', got %s", cfg.Body)
	}

	if cfg.HTTPTimeout != 45*time.Second {
		t.Errorf("Expected HTTP timeout to be 45s, got %v", cfg.HTTPTimeout)
	}

	if cfg.TCPKeepAlive != false {
		t.Errorf("Expected TCP keep-alive to be false, got %v", cfg.TCPKeepAlive)
	}

	if cfg.TCPKeepAlivePeriod != 60*time.Second {
		t.Errorf("Expected TCP keep-alive period to be 60s, got %v", cfg.TCPKeepAlivePeriod)
	}

	if cfg.DisableKeepAlives != true {
		t.Errorf("Expected disable keep-alives to be true, got %v", cfg.DisableKeepAlives)
	}

	if cfg.MaxIdleConns != 200 {
		t.Errorf("Expected max idle connections to be 200, got %d", cfg.MaxIdleConns)
	}

	if cfg.MaxIdleConnsPerHost != 20 {
		t.Errorf("Expected max idle connections per host to be 20, got %d", cfg.MaxIdleConnsPerHost)
	}
}

func TestConfigJSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"url": "https://example.com",
		"requestsPerSecond": 5.0,
		"algorithm": "leaky-bucket",
		"duration": "30s",
		"concurrency": 2,
		"method": "POST",
		"headers": {
			"Content-Type": "application/json"
		},
		"body": "test",
		"httpTimeout": "45s",
		"tcpKeepAlive": false,
		"tcpKeepAlivePeriod": "60s",
		"disableKeepAlives": true,
		"maxIdleConns": 200,
		"maxIdleConnsPerHost": 20
	}`

	cfg := &config.Config{}
	if err := cfg.UnmarshalJSON([]byte(jsonData)); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if cfg.URL != "https://example.com" {
		t.Errorf("Expected URL to be https://example.com, got %s", cfg.URL)
	}

	if cfg.RequestsPerSecond != 5.0 {
		t.Errorf("Expected rate to be 5.0, got %f", cfg.RequestsPerSecond)
	}

	if cfg.Algorithm != config.LeakyBucket {
		t.Errorf("Expected algorithm to be leaky-bucket, got %s", cfg.Algorithm)
	}

	if cfg.Duration != 30*time.Second {
		t.Errorf("Expected duration to be 30s, got %v", cfg.Duration)
	}

	if cfg.HTTPTimeout != 45*time.Second {
		t.Errorf("Expected HTTP timeout to be 45s, got %v", cfg.HTTPTimeout)
	}

	if cfg.TCPKeepAlive != false {
		t.Errorf("Expected TCP keep-alive to be false, got %v", cfg.TCPKeepAlive)
	}

	if cfg.TCPKeepAlivePeriod != 60*time.Second {
		t.Errorf("Expected TCP keep-alive period to be 60s, got %v", cfg.TCPKeepAlivePeriod)
	}
}
