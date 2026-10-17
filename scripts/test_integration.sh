#!/usr/bin/env bash

# ─────────────────────────────────────────────────────────────
# Integration Tests — Real Redis
# ─────────────────────────────────────────────────────────────
#
# Runs the Redis token bucket tests against a live Redis
# instance. These prove end-to-end correctness:
#   - Lua script atomically consumes tokens and enforces limits
#   - Token refill works via Redis' own TIME command
#   - Global limit holds across multiple strategy instances
#     (simulating multiple gateway pods)
#   - 500 concurrent requests are handled atomically
#
# Requires: Redis at REDIS_ADDR (default: 127.0.0.1:6379).
# Tests skip gracefully if Redis is not available.
#
# Usage:  bash scripts/test_integration.sh
#         REDIS_ADDR=my-redis:6379 bash scripts/test_integration.sh
# ─────────────────────────────────────────────────────────────

set -uo pipefail

REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export REDIS_ADDR

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Integration Tests — Real Redis"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Proves: rate limiting works end-to-end against a real Redis"
echo "  instance, including distributed correctness across multiple"
echo "  strategy instances and token refill via the Lua script."
echo ""
echo "  REDIS_ADDR=${REDIS_ADDR}"
echo "  (Tests skip gracefully if Redis is not available)"
echo ""

go test -race -count=1 -v \
  -run 'TestRedisTokenBucketStrategy' \
  ./internal/middleware/ratelimit/...
exit_code=$?

echo ""
if [ "$exit_code" -eq 0 ]; then
  echo "  ✅  Integration tests passed (or skipped — see output above)."
else
  echo "  ❌  Integration tests failed."
fi

exit $exit_code
