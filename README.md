# GoRL - Go Rate Limiter

A Go tool for testing HTTP request rate limits. This tool sends HTTP requests at a specified rate to test API rate limiting functionality and performance.

## Features

- Send HTTP requests at a specified rate (requests/second)
- Execute requests with multiple concurrent workers
- Multiple rate limiting algorithms (Token Bucket, Leaky Bucket, Fixed Window, Sliding Window Log, Sliding Window Counter)
- Real-time statistics reporting
- Detailed result reports (response times, status code distribution, etc.)
- Configuration via command-line arguments or JSON file
- Support for various HTTP methods and headers

## Installation

```bash
go build -o gorl main.go
```

## Usage

### Command Line Arguments

Basic usage:

```bash
./gorl -url=https://httpbin.org/get -rate=5 -duration=30s
```

Advanced configuration:

```bash
./gorl -url=https://api.example.com -rate=10 -algorithm=leaky-bucket -duration=60s -concurrency=3 -method=POST -headers="Content-Type:application/json,Authorization:Bearer token123" -body='{"test":"data"}'
```

### Configuration File

Using a configuration file (JSON):

```bash
./gorl -config=config.json
```

Example configuration file (see `config.example.json`):

```json
{
  "url": "https://httpbin.org/get",
  "requestsPerSecond": 5.0,
  "algorithm": "token-bucket",
  "duration": "30s",
  "concurrency": 2,
  "method": "GET",
  "headers": {
    "User-Agent": "GoRL Rate Limiter",
    "Accept": "application/json"
  },
  "body": ""
}
```

## Rate Limiting Algorithms

GoRL supports multiple rate limiting algorithms, each with different characteristics:

### Token Bucket (default)

- **Best for**: Allowing bursts while maintaining average rate
- **How it works**: Tokens are added to a bucket at a constant rate. Each request consumes one token
- **Pros**: Allows short bursts, smooth for variable traffic
- **Cons**: May allow more requests than expected in short periods

### Leaky Bucket

- **Best for**: Enforcing strict rate limits with no bursts
- **How it works**: Requests are processed at a fixed rate, excess requests wait
- **Pros**: Guarantees exact rate, prevents bursts
- **Cons**: May introduce latency for bursty traffic

### Fixed Window

- **Best for**: Simple rate limiting with predictable windows
- **How it works**: Counts requests in fixed time windows (e.g., per second)
- **Pros**: Simple, predictable behavior
- **Cons**: Can allow double the rate at window boundaries

### Sliding Window Log

- **Best for**: Precise rate limiting with perfect accuracy
- **How it works**: Maintains a log of all request timestamps
- **Pros**: Most accurate, no boundary effects
- **Cons**: High memory usage for high traffic

### Sliding Window Counter

- **Best for**: Good approximation with moderate memory usage
- **How it works**: Uses multiple sub-windows to approximate sliding behavior
- **Pros**: Good accuracy, reasonable memory usage
- **Cons**: More complex than fixed window

## Timeout Configuration

GoRL provides detailed timeout configuration options to handle various network conditions:

### Timeout Types

1. **HTTP Timeout** (`-http-timeout`): Overall timeout for the entire HTTP request

   - Default: 30s
   - Controls the maximum time for a complete request/response cycle

2. **Connect Timeout** (`-connect-timeout`): Timeout for establishing TCP connection

   - Default: 10s
   - Controls how long to wait for the initial TCP connection

3. **TLS Handshake Timeout** (`-tls-handshake-timeout`): Timeout for TLS/SSL handshake

   - Default: 10s
   - Controls the maximum time for TLS negotiation

4. **Response Header Timeout** (`-response-header-timeout`): Timeout for receiving response headers
   - Default: 10s
   - Controls how long to wait for the server to send response headers

### Usage Examples

```bash
# Quick timeout for fast APIs
./gorl -url=https://api.example.com -rate=10 -http-timeout=5s -connect-timeout=2s

# Longer timeouts for slow endpoints
./gorl -url=https://slow-api.example.com -rate=2 -http-timeout=60s -response-header-timeout=30s

# Strict timeouts for testing
./gorl -url=https://api.example.com -rate=5 -connect-timeout=1s -tls-handshake-timeout=2s
```

### Environment Variables

All timeout settings can also be configured via environment variables:

```bash
export GORL_HTTP_TIMEOUT=20s
export GORL_CONNECT_TIMEOUT=5s
export GORL_TLS_HANDSHAKE_TIMEOUT=5s
export GORL_RESPONSE_HEADER_TIMEOUT=10s
./gorl -url=https://api.example.com -rate=10
```

## Options

| Option                     | Description                                   | Default      |
| -------------------------- | --------------------------------------------- | ------------ |
| `-url`                     | Target URL to test (required)                 | -            |
| `-rate`                    | Requests per second                           | 1.0          |
| `-algorithm`               | Rate limiting algorithm                       | token-bucket |
| `-duration`                | Test execution duration                       | 10s          |
| `-concurrency`             | Number of concurrent workers                  | 1            |
| `-method`                  | HTTP method                                   | GET          |
| `-headers`                 | HTTP headers (key1:value1,key2:value2 format) | -            |
| `-body`                    | Request body                                  | -            |
| `-config`                  | Configuration file path                       | -            |
| `-http-timeout`            | HTTP request timeout                          | 30s          |
| `-connect-timeout`         | TCP connection timeout                        | 10s          |
| `-tls-handshake-timeout`   | TLS handshake timeout                         | 10s          |
| `-response-header-timeout` | Response header timeout                       | 10s          |
| `-tcp-keep-alive`          | Enable TCP keep-alive                         | true         |
| `-tcp-keep-alive-period`   | TCP keep-alive period                         | 30s          |
| `-disable-keep-alives`     | Disable HTTP keep-alives                      | false        |
| `-max-idle-conns`          | Maximum idle connections                      | 100          |
| `-max-idle-conns-per-host` | Maximum idle connections per host             | 10           |
| `-live`                    | Show live statistics                          | false        |
| `-compact`                 | Show compact one-line statistics              | false        |
| `-report-interval`         | Statistics report interval                    | 2s           |
| `-help`                    | Show help message                             | -            |

### Available Algorithms

- `token-bucket` - Token bucket algorithm (default)
- `leaky-bucket` - Leaky bucket algorithm
- `fixed-window` - Fixed window algorithm
- `sliding-window-log` - Sliding window log algorithm
- `sliding-window-counter` - Sliding window counter algorithm

## Output Example

```
Starting rate limit test...
Target URL: https://httpbin.org/get
Rate: 5.00 requests/second
Algorithm: Token Bucket
Duration: 30s
Concurrency: 2
Method: GET
----------------------------------------
Requests: 25 | Success: 25 | Failed: 0 | Rate: 5.00 req/s
Requests: 50 | Success: 50 | Failed: 0 | Rate: 5.00 req/s

========================================
Final Results:
Total Requests: 150
Successful Requests: 150
Failed Requests: 0
Success Rate: 100.00%

Status Code Distribution:
  200: 150 requests

Response Times:
  Min: 89.123ms
  Max: 245.567ms
  Avg: 142.345ms

Actual Rate: 5.00 requests/second
Target Rate: 5.00 requests/second
Algorithm Used: Token Bucket
```

## License

MIT License - See the LICENSE file for details.
