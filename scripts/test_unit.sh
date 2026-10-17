#!/usr/bin/env bash

# ─────────────────────────────────────────────────────────────
# Unit Tests — Algorithm Correctness
# ─────────────────────────────────────────────────────────────
#
# Runs the local token bucket and sliding window tests. These
# prove the rate-limiting math is correct:
#   - Tokens/slots are consumed up to capacity, then denied
#   - Token refill and window expiry restore capacity correctly
#   - Per-client buckets/windows are isolated
#   - Concurrent access respects the configured limit
#
# No external dependencies (no Redis). Runs in milliseconds.
#
# Usage:  bash scripts/test_unit.sh
# ─────────────────────────────────────────────────────────────

set -uo pipefail

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Unit Tests — Algorithm Correctness"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Proves: token bucket and sliding window math is correct,"
echo "  tokens refill on schedule, per-client isolation holds,"
echo "  and concurrent access respects the configured limit."
echo ""

go test -race -count=1 -v \
  -run 'TestLocalTokenBucket|TestLocalSlidingWindow' \
  ./internal/middleware/ratelimit/...
exit_code=$?

echo ""
if [ "$exit_code" -eq 0 ]; then
  echo "  ✅  Unit tests passed."
else
  echo "  ❌  Unit tests failed."
fi

exit $exit_code
