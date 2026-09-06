#!/usr/bin/env sh
# gofmt-check.sh — fail if any file this feature owns is not gofmt-clean.
#
# Scoped deliberately: the repo has ~18 pre-existing unformatted files, so a
# tree-wide check would fail on debt this feature did not introduce. Widen the
# list as those get cleaned up.
#
# Declared as an Aitri quality_gate. Aitri splits a gate command on whitespace
# and spawns it WITHOUT a shell, so the gate cannot itself contain "$(...)",
# quotes or env assignments — hence this script.
set -u
cd "$(dirname "$0")/.." || exit 2

DIRS="internal/dockerapi internal/isolation internal/docker internal/privileged cmd/ultron-helper"
out=$(gofmt -l $DIRS)
if [ -n "$out" ]; then
  echo "gofmt: the following files are not formatted:"
  echo "$out"
  exit 1
fi
echo "gofmt: clean ($DIRS)"
