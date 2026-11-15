#!/bin/sh
# Run: sh .github/scripts/latest-release_test.sh
# Drives latest-release.sh against a fake `gh` on PATH.
set -eu

S="$(cd "$(dirname "$0")" && pwd)/latest-release.sh"
FAKE="$(mktemp -d)"
trap 'rm -rf "$FAKE"' EXIT

cat > "$FAKE/gh" <<'GH'
#!/bin/sh
echo "$*" > "$FAKE_DIR/args"
case "$FAKE_GH" in
	ok) echo "v1.8.0" ;;
	notfound) echo 'gh: Not Found (HTTP 404)' >&2; exit 1 ;;
	error) echo 'gh: Server Error (HTTP 500)' >&2; exit 1 ;;
	prerelease) echo "v1.8.0-rc1" ;;
esac
GH
chmod +x "$FAKE/gh"

run() {
	FAKE_GH="$1" FAKE_DIR="$FAKE" GITHUB_REPOSITORY=econumo/econumo PATH="$FAKE:$PATH" sh "$S" 2>/dev/null
}

check() {
	if [ "$1" != "$2" ]; then
		echo "FAIL: $3: got '$1', want '$2'" >&2
		exit 1
	fi
}

check "$(run ok)" "v1.8.0" "latest release"
check "$(cat "$FAKE/args")" "api repos/econumo/econumo/releases/latest --jq .tag_name" "gh call"
check "$(run notfound)" "dev" "no release yet"
if run error >/dev/null; then echo "FAIL: an API error must fail" >&2; exit 1; fi
if run prerelease >/dev/null; then echo "FAIL: a non-plain tag must fail" >&2; exit 1; fi

echo ok
