#!/usr/bin/env bash

# ─────────────────────────────────────────────────────────────
# Benchmarks — Performance Overhead
# ─────────────────────────────────────────────────────────────
#
# Runs Go benchmarks for all rate-limiting strategies. Output
# shows ns/op, B/op, and allocs/op — answering whether the
# per-request overhead is acceptable for a hot-path middleware.
#
# The Redis benchmark skips gracefully if Redis is unavailable.
#
# Usage:  bash scripts/test_benchmarks.sh
# ─────────────────────────────────────────────────────────────

set -uo pipefail

REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export REDIS_ADDR

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Benchmarks — Performance Overhead"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Proves: per-request overhead (ns/op) and memory allocations"
echo "  are acceptable for a hot-path middleware."
echo ""

go test -bench=. -benchmem -count=1 -run='^$' \
  ./internal/middleware/ratelimit/...
exit_code=$?

echo ""
if [ "$exit_code" -eq 0 ]; then
  echo "  ✅  Benchmarks completed."
else
  echo "  ❌  Benchmarks failed."
fi

exit $exit_code
