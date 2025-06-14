#!/bin/bash

# GoRL Benchmark Script
# This script runs various performance benchmarks for GoRL

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
URL="${BENCHMARK_URL:-https://httpbin.org/get}"
DURATION="${BENCHMARK_DURATION:-30s}"
WARMUP="${BENCHMARK_WARMUP:-5s}"

echo -e "${GREEN}GoRL Performance Benchmark${NC}"
echo "=================================="
echo "URL: $URL"
echo "Duration: $DURATION"
echo "Warmup: $WARMUP"
echo ""

# Build the binary
echo -e "${YELLOW}Building GoRL...${NC}"
make build

# Function to run benchmark
run_benchmark() {
    local name=$1
    local rate=$2
    local concurrency=$3
    local algorithm=$4

    echo -e "\n${GREEN}Running: $name${NC}"
    echo "Rate: $rate req/s, Concurrency: $concurrency, Algorithm: $algorithm"
    echo "---"

    ./bin/gorl \
        -url="$URL" \
        -rate="$rate" \
        -duration="$DURATION" \
        -concurrency="$concurrency" \
        -algorithm="$algorithm" \
        -benchmark \
        -warmup="$WARMUP"
}

# Test different algorithms
echo -e "\n${YELLOW}=== Algorithm Comparison ===${NC}"
for algo in token-bucket leaky-bucket fixed-window sliding-window-log sliding-window-counter; do
    run_benchmark "Algorithm: $algo" 100 10 "$algo"
done

# Test different rates
echo -e "\n${YELLOW}=== Rate Scaling Test ===${NC}"
for rate in 10 50 100 500 1000; do
    run_benchmark "Rate: $rate req/s" "$rate" 10 "token-bucket"
done

# Test different concurrency levels
echo -e "\n${YELLOW}=== Concurrency Scaling Test ===${NC}"
for concurrency in 1 5 10 50 100; do
    run_benchmark "Concurrency: $concurrency" 100 "$concurrency" "token-bucket"
done

# Test timeout configurations
echo -e "\n${YELLOW}=== Timeout Configuration Test ===${NC}"
echo -e "${GREEN}Running: Tight timeouts${NC}"
./bin/gorl \
    -url="$URL" \
    -rate=50 \
    -duration="$DURATION" \
    -concurrency=10 \
    -algorithm="token-bucket" \
    -http-timeout=1s \
    -connect-timeout=500ms \
    -benchmark \
    -warmup="$WARMUP"

echo -e "\n${GREEN}Running: Relaxed timeouts${NC}"
./bin/gorl \
    -url="$URL" \
    -rate=50 \
    -duration="$DURATION" \
    -concurrency=10 \
    -algorithm="token-bucket" \
    -http-timeout=30s \
    -connect-timeout=10s \
    -benchmark \
    -warmup="$WARMUP"

echo -e "\n${GREEN}Benchmark completed!${NC}"
