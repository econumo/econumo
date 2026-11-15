#!/bin/sh
# Run: sh .github/scripts/highest-release-tag_test.sh
set -eu

S="$(dirname "$0")/highest-release-tag.sh"

check() {
	if [ "$1" != "$2" ]; then
		echo "FAIL: got '$1', want '$2'" >&2
		exit 1
	fi
}

check "$(printf 'v1.9.3\nv1.10.0\nv1.2.0\n' | sh "$S")" "v1.10.0"
check "$(printf 'v1.8.0-rc1\nv2\nfoo\nv1.7.3\n' | sh "$S")" "v1.7.3"
check "$(printf 'abc123\trefs/tags/v1.7.3\ndef456\trefs/tags/v1.7.4\n' | sh "$S")" "v1.7.4"
check "$(printf '' | sh "$S")" ""
check "$(printf 'v1.8.0-rc1\n' | sh "$S")" ""

echo ok
