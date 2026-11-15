#!/bin/sh
# Prints the highest vX.Y.Z tag among the lines on stdin (bare tags or
# `git ls-remote --tags --refs` lines), or nothing when there is none.
# Pre-release and malformed tags never count: an edge build stamps this as its
# version, and only a plain vX.Y.Z passes the app's release checks.
#   git ls-remote --tags --refs origin | sh .github/scripts/highest-release-tag.sh
set -eu

sed 's#.*refs/tags/##' | { grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true; } | sort -V | tail -n 1
