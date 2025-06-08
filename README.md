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

## Options

| Option         | Description                                   | Default      |
| -------------- | --------------------------------------------- | ------------ |
| `-url`         | Target URL to test (required)                 | -            |
| `-rate`        | Requests per second                           | 1.0          |
| `-algorithm`   | Rate limiting algorithm                       | token-bucket |
| `-duration`    | Test execution duration                       | 10s          |
| `-concurrency` | Number of concurrent workers                  | 1            |
| `-method`      | HTTP method                                   | GET          |
| `-headers`     | HTTP headers (key1:value1,key2:value2 format) | -            |
| `-body`        | Request body                                  | -            |
| `-config`      | Configuration file path                       | -            |
| `-help`        | Show help message                             | -            |

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
