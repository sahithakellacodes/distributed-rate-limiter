#!/usr/bin/env bash

# ─────────────────────────────────────────────────────────────
# Resilience Tests — Graceful Degradation
# ─────────────────────────────────────────────────────────────
#
# Runs the ResilientWrapper tests. These prove the failover
# logic between the primary (Redis) and fallback (local)
# strategies:
#   - Healthy primary receives all requests
#   - Primary error triggers fallback + health failure report
#   - Unhealthy state skips primary entirely
#   - Recovery switches back to primary
#
# No external dependencies (uses mock strategies). Runs in ms.
#
# Usage:  bash scripts/test_resilience.sh
# ─────────────────────────────────────────────────────────────

set -uo pipefail

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Resilience Tests — Graceful Degradation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Proves: when Redis (primary) fails, the resilient wrapper"
echo "  falls back to the local strategy without dropping requests,"
echo "  and switches back once Redis recovers."
echo ""

go test -race -count=1 -v \
  -run 'TestResilientWrapper' \
  ./internal/middleware/ratelimit/...
exit_code=$?

echo ""
if [ "$exit_code" -eq 0 ]; then
  echo "  ✅  Resilience tests passed."
else
  echo "  ❌  Resilience tests failed."
fi

exit $exit_code
