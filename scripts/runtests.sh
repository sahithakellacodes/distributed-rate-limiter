#!/usr/bin/env bash

# ─────────────────────────────────────────────────────────────
# Distributed Rate Limiter — Full Test Suite
# ─────────────────────────────────────────────────────────────
#
# Orchestrator that runs all test categories in order:
#   1. Unit tests       (algorithm correctness)
#   2. Resilience tests (graceful degradation)
#   3. Integration tests (real Redis)
#   4. Benchmarks       (performance overhead)
#
# Each category is an independent script in scripts/ that can
# also be run on its own.
#
# Usage:  bash scripts/runtests.sh
# ─────────────────────────────────────────────────────────────

set -uo pipefail

# Resolve the scripts directory relative to this script's location.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

section_names=()
section_results=()

record_result() {
  local name="$1"
  local exit_code="$2"
  section_names+=("$name")
  if [ "$exit_code" -eq 0 ]; then
    section_results+=("PASS")
  else
    section_results+=("FAIL")
  fi
}

# ── Header ───────────────────────────────────────────────────

echo "╔═══════════════════════════════════════════════════════════╗"
echo "║   Distributed Rate Limiter — Test Suite                  ║"
echo "╠═══════════════════════════════════════════════════════════╣"
echo "║  Demonstrates that the rate-limiting algorithms are      ║"
echo "║  correct, resilient, performant, and safe under          ║"
echo "║  concurrency — both locally and with real Redis.         ║"
echo "╚═══════════════════════════════════════════════════════════╝"

# ── 1. Unit Tests ────────────────────────────────────────────

bash "${SCRIPT_DIR}/test_unit.sh"
record_result "Unit Tests" $?

# ── 2. Resilience Tests ─────────────────────────────────────

bash "${SCRIPT_DIR}/test_resilience.sh"
record_result "Resilience Tests" $?

# ── 3. Integration Tests ────────────────────────────────────

bash "${SCRIPT_DIR}/test_integration.sh"
record_result "Integration Tests (Redis)" $?

# ── 4. Benchmarks ───────────────────────────────────────────

bash "${SCRIPT_DIR}/test_benchmarks.sh"
record_result "Benchmarks" $?

# ── Summary ──────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Summary"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

overall=0
printf "  %-30s %s\n" "SECTION" "RESULT"
printf "  %-30s %s\n" "───────────────────────────────" "──────"
for i in "${!section_names[@]}"; do
  printf "  %-30s %s\n" "${section_names[$i]}" "${section_results[$i]}"
  if [ "${section_results[$i]}" = "FAIL" ]; then
    overall=1
  fi
done

echo ""
if [ "$overall" -eq 0 ]; then
  echo "  ✅  All sections passed."
else
  echo "  ❌  Some sections failed. See output above for details."
fi
echo ""

exit $overall
