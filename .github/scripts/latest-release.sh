#!/bin/sh
# Prints the tag of the GitHub release marked Latest, `dev` when the repository
# has no release, and fails on anything else (an API error, a non-plain tag):
# an edge build stamps this as its version, and a wrong guess would put the
# wrong What's new card in front of every production user. The Latest badge,
# not the highest tag, because the release workflow sets it last: a tag whose
# release failed or that was published without push_latest never counts.
# Needs GITHUB_REPOSITORY and a GH_TOKEN that can read the repository.
set -eu

err="$(mktemp)"
trap 'rm -f "$err"' EXIT

if tag="$(gh api "repos/${GITHUB_REPOSITORY}/releases/latest" --jq .tag_name 2>"$err")"; then
	if printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
		echo "$tag"
		exit 0
	fi
	echo "latest release has a non-release tag: '$tag'" >&2
	exit 1
fi
if grep -q 'HTTP 404' "$err"; then
	echo dev
	exit 0
fi
cat "$err" >&2
exit 1
