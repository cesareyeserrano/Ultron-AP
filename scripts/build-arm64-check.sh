#!/usr/bin/env sh
# build-arm64-check.sh — cross-compile both binaries for the production target.
#
# The Pi has no source checkout, so linux/arm64 is the only target that matters
# for deployment. A change that builds on darwin but not arm64 is a broken
# deploy, and this catches it before the binaries are copied.
#
# Declared as an Aitri quality_gate; see gofmt-check.sh for why it is a script
# rather than an inline command.
set -eu
cd "$(dirname "$0")/.." || exit 2

GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/ultron-ap
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/ultron-helper
echo "build: linux/arm64 OK for ultron-ap and ultron-helper"
