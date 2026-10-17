#!/usr/bin/env bash

set -e

echo "Running rate limiter concurrency test..."

REDIS_ADDR=127.0.0.1:6379 go test -race -count=1 -v \
  -run TestRedisTokenBucketStrategy_Concurrent \
  ./internal/middleware/ratelimit/...
