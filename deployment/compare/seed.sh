#!/usr/bin/env bash
# Seed a synthetic fixture database, then stand up BOTH backends against an
# identical copy of it so their API responses can be diffed on the full payload
# (dates included).
#
# The data is the project's SYNTHETIC DataFixtures (john@econumo.test, etc.),
# loaded into the *test* database (var/db/db_test.sqlite). Your personal
# var/db/db.sqlite is never touched.
#
#   deployment/compare/seed.sh
#
# What it does:
#   1. Loads fixtures into var/db/db_test.sqlite (via the dev container, env=test).
#   2. Copies that seed to a stable file the Go backend reads, so PHP and Go each
#      read byte-identical data:
#        - PHP reads var/db/db_test.sqlite           (served by the dev container)
#        - Go  reads deployment/compare/db.go.sqlite (a copy of the same seed)
#   3. Prints the exact commands to boot both backends + run the diff harness.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$ROOT/deployment/compare"
TEST_DB="$ROOT/var/db/db_test.sqlite"
mkdir -p "$OUT"

echo "==> Bringing up the dev PHP stack"
docker-compose up -d

echo "==> Recreating the test database and loading synthetic fixtures (env=test)"
docker-compose exec -uwww-data app bin/console doctrine:database:drop --force --env=test -vvv || true
docker-compose exec -uwww-data app bin/console doctrine:database:create --env=test -vvv
docker-compose exec -uwww-data app bin/console doctrine:migration:migrate -n --env=test -vvv
docker-compose exec -uwww-data app bin/console doctrine:fixtures:load --purge-with-truncate -n --env=test -vvv

if [ ! -f "$TEST_DB" ]; then
  echo "ERROR: expected seeded DB at $TEST_DB but it does not exist" >&2
  echo "       (check DATABASE_NAME in .env.test — should be db_test)" >&2
  exit 1
fi

echo "==> Copying the seed to the Go backend's file (identical bytes)"
cp "$TEST_DB" "$OUT/db.go.sqlite"

if ! cmp -s "$TEST_DB" "$OUT/db.go.sqlite"; then
  echo "ERROR: the copy differs from the seed — aborting" >&2
  exit 1
fi
SUM="$(shasum -a 256 "$TEST_DB" | cut -d' ' -f1)"
echo "==> Done. Two identical databases (sha256 ${SUM:0:16}...):"
echo "      PHP -> $TEST_DB        (served by the dev container)"
echo "      Go  -> $OUT/db.go.sqlite"
echo
echo "------------------------------------------------------------------------"
echo "1) Make the PHP dev container serve the TEST database on :8082"
echo "   (APP_ENV=test points it at db_test.sqlite):"
echo
echo "     APP_ENV=test docker-compose up -d"
echo
echo "2) Start the Go backend on :8282 against its copy:"
echo
echo "     cd go && PORT=8282 \\"
echo "       DATABASE_URL=\"sqlite://$OUT/db.go.sqlite\" \\"
echo "       JWT_PUBLIC_KEY=../config/jwt/public.pem JWT_SECRET_KEY=../config/jwt/private.pem \\"
echo "       JWT_PASSPHRASE=d78eedcb16c13bd949ede5d1b8b910cd APP_ENV=dev ECONUMO_DATA_SALT= \\"
echo "       ECONUMO_SPA_DIR=../web/dist/spa go run ./cmd/econumo"
echo
echo "3) Run the diff harness:"
echo
echo "     cd go && go run ./cmd/apicompare -php http://localhost:8082 -go http://localhost:8282"
echo "------------------------------------------------------------------------"
