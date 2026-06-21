#!/usr/bin/env bash
# Per-case mutation comparison driver.
#
# For EACH write-endpoint case (see `go run ./cmd/mutatecompare -list`):
#   1. stop the Go backend (PHP reads the file per-request, no restart needed)
#   2. copy a FRESH seed DB for each backend (full isolation per case)
#   3. clear the PHP cache so it reopens the fresh file
#   4. restart Go against its fresh copy
#   5. run `mutatecompare -case <name>` (POST to both, diff response + state)
#   6. record PASS/DIFF/SKIP, move to the next case
#
# Usage:
#   deployment/compare/mutate_compare.sh                 # all cases
#   deployment/compare/mutate_compare.sh category/       # only matching prefix
#
# Env overrides: EMAIL, PASSWORD, PHP_URL, GO_URL, SEED, PHP_DB, GO_DB.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

SEED="${SEED:-$ROOT/var/test/seed.sqlite}"
PHP_DB="${PHP_DB:-$ROOT/deployment/compare/db.php.sqlite}"
GO_DB="${GO_DB:-$ROOT/deployment/compare/db.go.sqlite}"
PHP_URL="${PHP_URL:-http://localhost:8082}"
GO_URL="${GO_URL:-http://localhost:8282}"
EMAIL="${EMAIL:-kuznetsov2d@gmail.com}"
PASSWORD="${PASSWORD:-econum0))}"
FILTER="${1:-}"

[ -f "$SEED" ] || { echo "seed not found: $SEED" >&2; exit 1; }

stop_go() {
  lsof -nP -iTCP:8282 -sTCP:LISTEN -t 2>/dev/null | xargs -r kill 2>/dev/null
  pkill -f "exe/econumo" 2>/dev/null
  # wait for the port to free
  for _ in $(seq 1 20); do
    lsof -nP -iTCP:8282 -sTCP:LISTEN -t >/dev/null 2>&1 || return 0
    sleep 0.3
  done
}

start_go() {
  ( cd "$ROOT/go" && nohup go run ./cmd/econumo > /tmp/go_mutate.log 2>&1 & )
  for _ in $(seq 1 60); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "$GO_URL/_/health-check")" = "200" ] && return 0
    sleep 0.5
  done
  echo "Go did not come up; log:" >&2; tail -5 /tmp/go_mutate.log >&2; return 1
}

reset_dbs() {
  # CRITICAL: Go opens SQLite in WAL mode, leaving <db>-wal and <db>-shm sidecars.
  # Copying only the main file leaves a stale WAL that the next Go boot replays —
  # silently re-applying the PREVIOUS case's writes. Remove the sidecars (Go must
  # be stopped first) so each case starts from the pristine seed.
  rm -f "$GO_DB-wal" "$GO_DB-shm" "$PHP_DB-wal" "$PHP_DB-shm"
  cp "$SEED" "$PHP_DB"
  cp "$SEED" "$GO_DB"
  # PHP opens the sqlite file per request, but cache:clear ensures no stale state.
  docker compose exec -T -u www-data app sh -c 'bin/console cache:clear --env=dev -q' >/dev/null 2>&1
}

CASES=$(cd "$ROOT/go" && go run ./cmd/mutatecompare -list)
[ -n "$FILTER" ] && CASES=$(echo "$CASES" | grep "^$FILTER")

pass=0; fail=0; skip=0; failed_cases=""
for c in $CASES; do
  stop_go
  reset_dbs
  start_go || exit 1
  out=$(cd "$ROOT/go" && go run ./cmd/mutatecompare \
    -case "$c" -php "$PHP_URL" -go "$GO_URL" -email "$EMAIL" -password "$PASSWORD" 2>&1)
  echo "$out"
  if   echo "$out" | grep -q '^\[PASS\]'; then pass=$((pass+1))
  elif echo "$out" | grep -q '^\[SKIP\]'; then skip=$((skip+1))
  else fail=$((fail+1)); failed_cases="$failed_cases $c"; fi
done

echo
echo "===== mutate-compare: $pass passed, $fail failed, $skip skipped ====="
[ -n "$failed_cases" ] && echo "failed:$failed_cases"
[ "$fail" -eq 0 ]
