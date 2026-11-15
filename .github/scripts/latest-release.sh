#!/bin/sh
# Prints the highest published release (a plain vX.Y.Z GitHub release, no
# draft, no pre-release), `dev` when the repository has none, and fails when
# GitHub cannot be asked: an edge build stamps this as its version, and a
# wrong guess would put the wrong What's new card in front of every
# production user. Published releases, not tags: the release workflow pushes
# the tag first, so a release that failed afterwards leaves a tag but never
# counts. The highest version, not the Latest badge or the newest one created:
# an old-line patch for self-hosters (v1.6.2 after v1.7.0) never moves edge,
# and a newest release published without push_latest still does.
# Needs GITHUB_REPOSITORY and a GH_TOKEN that can read the repository.
set -eu

tags="$(gh api "repos/${GITHUB_REPOSITORY}/releases?per_page=100" --jq '.[] | select(.draft or .prerelease | not) | .tag_name')"
highest="$(printf '%s\n' "$tags" | { grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true; } | sort -V | tail -n 1)"
echo "${highest:-dev}"
