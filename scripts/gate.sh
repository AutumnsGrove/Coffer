#!/bin/sh
# The full gate for a slice, run inside the test container by `make test`.
# gofmt, vet, staticcheck, tests with the race detector, then the coverage floors in coverage-gates.txt.
set -eu

unformatted="$(gofmt -l cmd internal)"
if [ -n "$unformatted" ]; then
  echo "gate: gofmt would change these files:" >&2
  echo "$unformatted" >&2
  exit 1
fi
go vet ./...
staticcheck ./...

out="$(mktemp)"
status=0
go test -race -count=1 -cover ./... >"$out" 2>&1 || status=$?
cat "$out"
if [ "$status" -ne 0 ]; then
  echo "gate: tests failed" >&2
  exit "$status"
fi

go run ./cmd/covgate coverage-gates.txt "$out"
