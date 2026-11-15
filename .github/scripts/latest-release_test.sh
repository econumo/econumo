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
	newest) printf 'v1.6.2\nv1.7.0\nv1.6.1\nv1.10.0\nv1.9.3\n' ;;
	oldpatch) printf 'v1.6.2\nv1.7.0\nv1.6.1\n' ;;
	odd) printf 'v1.8.0-rc1\nfoo\nv1.7.0\n' ;;
	none) ;;
	error) echo 'gh: Server Error (HTTP 500)' >&2; exit 1 ;;
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

check "$(run newest)" "v1.10.0" "highest version, not text order"
check "$(cat "$FAKE/args")" "api repos/econumo/econumo/releases?per_page=100 --jq .[] | select(.draft or .prerelease | not) | .tag_name" "gh call"
check "$(run oldpatch)" "v1.7.0" "an old patch published last never wins"
check "$(run odd)" "v1.7.0" "non-release tags ignored"
check "$(run none)" "dev" "no release yet"
if run error >/dev/null; then echo "FAIL: an API error must fail" >&2; exit 1; fi

echo ok
