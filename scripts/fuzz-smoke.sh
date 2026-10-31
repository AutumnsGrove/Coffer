#!/bin/sh
# Run every fuzz target in the module for a short time. Runs inside the test container.
# Crashers land in testdata/fuzz/ next to the target; commit them (DEVELOPMENT.md rule 9).
set -eu

t="${FUZZTIME:-5s}"
found=0
for pkg in $(go list ./...); do
  for target in $(go test -list '^Fuzz' "$pkg" 2>/dev/null | grep '^Fuzz' || true); do
    found=$((found + 1))
    echo "fuzz ${pkg} ${target} for ${t}"
    go test -run '^$' -fuzz "^${target}\$" -fuzztime "$t" "$pkg"
  done
done
echo "fuzz-smoke: ${found} target(s) ran"
