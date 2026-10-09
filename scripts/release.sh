#!/usr/bin/env bash
# Release lazytuck: tag, GitHub release with archives, cask bump in t1mdotcom/homebrew-tap.
# Usage: scripts/release.sh 0.1.0
set -euo pipefail

die() { echo "release: $*" >&2; exit 1; }

version=${1:-}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "usage: scripts/release.sh <major.minor.patch>"
tag="v$version"

cd "$(git rev-parse --show-toplevel)"
command -v goreleaser >/dev/null || die "goreleaser not installed (brew install goreleaser)"
command -v gh >/dev/null || die "gh not installed"

# V14: only from a clean main that matches origin/main, after the full check.
[[ $(git branch --show-current) == main ]] || die "not on main"
[[ -z $(git status --porcelain) ]] || die "working tree not clean"
git fetch -q origin
[[ $(git rev-parse HEAD) == $(git rev-parse origin/main) ]] || die "main differs from origin/main"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "tag $tag already exists"

make check

git tag -a "$tag" -m "lazytuck $tag"
git push -q origin "$tag"

# GoReleaser builds the archives, creates the GitHub release and pushes Casks/lazytuck.rb.
# If this step fails, fix the cause and rerun: GITHUB_TOKEN=$(gh auth token) goreleaser release --clean
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean

echo "released $tag — brew install --cask t1mdotcom/tap/lazytuck"
