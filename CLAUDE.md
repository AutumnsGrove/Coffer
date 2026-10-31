# CLAUDE.md

Guidance for any AI agent working in this repository. Read `PLAN.md` (what and why) and
`DEVELOPMENT.md` (how and in what order) before writing code. This file is the short version of the
rules that are easiest to break.

## About the user

The user uses **she/her** pronouns. Use them in handoff docs, commit messages, comments and summaries,
and never guess or default to "he/him".

## What this is

Coffer: a private, self-hosted monthly finance tracker for one person. It reads the monthly PDF
statements from her two banks (Capital One credit card, Delta Community checking and savings),
reconciles them against what each statement prints about itself, stores integer cents in SQLite, and
shows the month. Go 1.26 backend, SvelteKit frontend, one Docker image, tailnet only.

## Commands (everything runs in Docker)

```bash
make test         # gofmt, vet, staticcheck, race detector, coverage floors. THE gate.
make bad          # only the bad-path subtests (fast safety sweep)
make sad          # only the sad-path subtests
make fuzz P=./internal/version T=FuzzValid FUZZTIME=60s
make fuzz-smoke   # every fuzz target, 5 seconds each
make vuln         # govulncheck (needs network)
make image VERSION=r1.abc1234 && make smoke
```

A local `go test ./internal/x` is fine while iterating. It never counts as passing: only `make test`
does. Do not use `go run` on the host to "check it works"; use the container.

## The rules

1. **Test first, always.** Write the failing test, run it, see it fail for the right reason, then write
   the least code that passes. Bug fixes start with a regression test. Never write code without a test
   that demands it.
2. **Name subtests `happy/...`, `sad/...`, `bad/...`.** `make bad` and `make sad` depend on it. Every
   boundary that takes outside input needs all three. The per-slice lists are in `DEVELOPMENT.md` section 9.
3. **Every parser gets a fuzz target** with a property (not just "does not panic"). Commit any crasher
   under `testdata/fuzz/`.
4. **Money is `int64` cents. Never a float**, not in code, not in tests, not on the wire.
5. **No real financial data, ever.** Not in fixtures, tests, docs, commit messages or logs. Fixtures are
   synthetic and generated. `internal/privacy` scans the repo on every `make test` and fails the build
   on an unstamped PDF or a run of 10 or more digits. Do not put a digit string in source; build it at run
   time (see `digits()` in `internal/privacy/privacy_test.go`). Findings report where, never what.
6. **Ask before adding a dependency.** Runtime: only `modernc.org/sqlite` and `ledongthuc/pdf`. Approved
   dev-only: `pgregory.net/rapid`, Playwright, `staticcheck` and `govulncheck` (pinned in the
   Dockerfile), `gremlins` later. Anything else needs her approval first.
7. **Logic does not live in `main`.** `main()` calls `run(args, stdout, stderr) int`, which is tested.
   `cmd/` has a lower coverage floor because `main()` itself cannot be tested.
8. **Treat strings from outside as hostile**: files, requests, arguments, statement text, even the build
   version. Validate, bound the length, never echo them into output or logs.
9. **Coverage floors only go up** (`coverage-gates.txt`). Do not lower one to make a build pass.
10. **Do not commit or push unless she asks.** When she does: present tense, first line under 50
    characters, no emojis, no em dashes. Commit the red test before the code that makes it pass when asked
    to commit in stages.
11. **Update `PLAN.md` and `NOTES.md` in the same change** when a decision changes or something new is
    learned. Uncertainty goes in `NOTES.md`, not in code comments.
12. **No emojis anywhere**: code, docs, UI, commits.

## Layout

```
cmd/coffer          the server (so far only --version)
cmd/covgate         coverage-floor checker used by the gate
internal/version    build stamp, validated
internal/privacy    repo scanner: unstamped PDFs, long digit runs
internal/covgate    parses `go test -cover`, applies coverage-gates.txt
scripts/            gate.sh (the full gate), fuzz-smoke.sh
spike/pdfread       THROWAWAY proof that the two PDFs can be read and reconciled. A separate Go
                    module; reference only. Read it for the layouts, do not import it.
docs/mockups        static HTML/CSS visual spec for the UI (Lexend, IBM Plex Mono, Lucide icons)
```

## Where things stand

Slice 0 (the harness) is done: the Docker test environment, the gate, CI, the privacy scan, the version
stamp. **Next is Slice 1, money** (`DEVELOPMENT.md` section 9): parse and format amounts as integer
cents with no silent rounding. `spike/pdfread/money.go` holds a working first draft of `cents` and
`fmtCents` to learn from. Write the tests listed in Slice 1 first, including `FuzzParseMoney`, and make
the draft pass them. Do not copy it in unchanged.
