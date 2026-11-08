#!/bin/sh
# The repository's gate, run before every pull request: the SDK as openapi.json generates it,
# formatted, vetted with its tests and examples, and built. The tests run once before each release,
# by scripts/test.sh, not here. Go reads nothing from the network: the module requires nothing
# beyond the standard library, GOPROXY=off makes a fetch fail rather than happen, and
# GOTOOLCHAIN=local keeps to the Go installed.
set -eu
cd "$(dirname "$0")/.."
export GOTOOLCHAIN=local GOPROXY=off GOFLAGS='-mod=readonly -p=2'

sh scripts/generate.sh

# What the generator writes, by the list it wrote now and the one the commit holds: a generated file
# that differs from the commit, is new to it or is gone from it fails the gate.
generated=$({
  cat .openapi-generator/FILES
  git show HEAD:.openapi-generator/FILES 2>/dev/null || true
} | grep -E '^[A-Za-z0-9_]+\.go$' | sort -u)
changed=$(git status --porcelain --untracked-files=all -- api_version.go .openapi-generator $generated)
if [ -n "$changed" ]; then
  echo "The SDK differs from what openapi.json generates: run scripts/generate.sh and commit it." >&2
  echo "$changed" >&2
  exit 1
fi

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "gofmt would change these files: run gofmt -w on them." >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
go build ./...
echo "Gates: GREEN"
