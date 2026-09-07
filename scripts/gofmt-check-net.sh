#!/usr/bin/env sh
# gofmt-check-net.sh — gofmt gate scoped to the packages net-sample-retention touches.
#
# Scoped rather than tree-wide: the repo carries ~18 pre-existing unformatted
# files and a global check would fail on debt this feature did not introduce.
#
# A script rather than an inline gate command because Aitri splits a gate
# command on whitespace and spawns it WITHOUT a shell, so "$(...)" and quotes
# would never be expanded.
set -u
cd "$(dirname "$0")/.." || exit 2

DIRS="internal/config internal/database internal/server internal/network/gatewayprobe internal/ups cmd/ultron-ap"
out=$(gofmt -l $DIRS)
if [ -n "$out" ]; then
  echo "gofmt: the following files are not formatted:"
  echo "$out"
  exit 1
fi
echo "gofmt: clean ($DIRS)"
