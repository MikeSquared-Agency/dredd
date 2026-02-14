#!/usr/bin/env bash
set -euo pipefail

PORT="${DREDD_PORT:-8750}"
BASE="http://localhost:${PORT}"
FAIL=0

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1 — $2"; FAIL=1; }

echo "=== Dredd E2E Smoke Tests ==="

# 1. Health check
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' "${BASE}/health")
if [ "$HTTP" = "200" ]; then
  BODY=$(cat /tmp/e2e_body)
  if echo "$BODY" | grep -q '"ok"'; then
    pass "Health check"
  else
    fail "Health check" "unexpected body: $BODY"
  fi
else
  fail "Health check" "expected 200, got $HTTP"
fi

# 2. Status endpoint
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' "${BASE}/api/v1/dredd/status")
if [ "$HTTP" = "200" ]; then
  BODY=$(cat /tmp/e2e_body)
  if echo "$BODY" | grep -q '"dredd"'; then
    pass "Status endpoint — agent identified"
  else
    fail "Status endpoint" "missing agent name: $BODY"
  fi
  if echo "$BODY" | grep -q '"shadow"'; then
    pass "Status endpoint — shadow mode"
  else
    fail "Status endpoint" "not in shadow mode: $BODY"
  fi
else
  fail "Status endpoint" "expected 200, got $HTTP"
fi

# 3. 404 on unknown route
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/api/v1/nonexistent")
if [ "$HTTP" = "404" ]; then
  pass "Unknown route returns 404"
else
  fail "Unknown route" "expected 404, got $HTTP"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "All tests passed."
else
  echo "Some tests FAILED."
  exit 1
fi
