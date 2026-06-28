#!/usr/bin/env bash
# Build and run the Go backend locally against a throwaway copy of the dev DB,
# with the repo's JWT keys. Then seed a test user you can log in as.
#
#   ./run-local.sh            # build, seed, run on :8181
#   PORT=9000 ./run-local.sh  # different port
#
# Stop with Ctrl-C. The DB copy lives at /tmp/econumo-local.sqlite and is reused
# across runs (delete it for a clean slate).
set -euo pipefail
cd "$(dirname "$0")"

DB=/tmp/econumo-local.sqlite
SALT=0123456789abcdef                  # ECONUMO_DATA_SALT (16 bytes, AES-128)
PORT=${PORT:-8181}

# Fresh copy of the dev DB so we never mutate the original.
if [ ! -f "$DB" ]; then
  cp var/db/db1.sqlite "$DB"
  echo "copied dev DB -> $DB"
  CGO_ENABLED=0 go run ./cmd/seed -dsn "$DB" -email test@econumo.test -password password -salt "$SALT"
fi

echo "building..."
CGO_ENABLED=0 go build -o /tmp/econumo-local ./cmd/econumo

echo "starting on http://localhost:$PORT  (login: test@econumo.test / password)"
# The engine is derived from the DATABASE_URL scheme (sqlite://) — no DATABASE_DRIVER.
# JWT keys auto-generate on first run into var/jwt (persisted across runs).
DATABASE_URL="sqlite://$DB" \
ECONUMO_DATA_SALT="$SALT" \
ECONUMO_ALLOW_REGISTRATION=true \
ECONUMO_SPA_DIR=web/dist/spa \
APP_ENV=dev \
PORT="$PORT" \
exec /tmp/econumo-local serve
