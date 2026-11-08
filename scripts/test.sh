#!/bin/sh
# The SDK's tests, run once before each release, as the TypeScript SDK's specs are. Each test
# package's TestMain strips every variable shaped like a credential before a test runs. Go reads
# nothing from the network: the module requires nothing beyond the standard library, GOPROXY=off
# makes a fetch fail rather than happen, and GOTOOLCHAIN=local keeps to the Go installed. -count=1
# runs every test, never a result Go kept from an earlier run.
set -eu
cd "$(dirname "$0")/.."
export GOTOOLCHAIN=local GOPROXY=off GOFLAGS='-mod=readonly -p=2'

go test -count=1 ./...
