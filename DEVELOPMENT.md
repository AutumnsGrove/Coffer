# Coffer: Development plan

How Coffer gets built. `PLAN.md` says **what** and **why**; this file says **in what order, and how we
know each piece works**. Nothing here has been built yet. The only code in the repo is the throwaway
`spike/pdfread`, which proved that both banks' statements can be read and reconciled. Everything else is
designed, not written.

The method is test-driven, Docker-first, and fuzzed. A slice is not done because the code exists; it is
done when its tests, named in §9 before the code is written, pass inside the container.

## Contents

1. [Rules of engagement](#1-rules-of-engagement)
2. [Happy, sad and bad paths](#2-happy-sad-and-bad-paths)
3. [Test layers and tools](#3-test-layers-and-tools)
4. [Fuzzing and properties](#4-fuzzing-and-properties)
5. [What is real and what is faked](#5-what-is-real-and-what-is-faked)
6. [Synthetic fixtures](#6-synthetic-fixtures)
7. [Docker](#7-docker)
8. [Coverage and gates](#8-coverage-and-gates)
9. [The slices, in order](#9-the-slices-in-order)
10. [Approvals needed](#10-approvals-needed)
11. [Definition of done](#11-definition-of-done)

---

## 1. Rules of engagement

1. **No production code without a failing test that demands it.** Red, green, refactor. The red test is
   committed first (`test: ...`), then the code that turns it green (`feat: ...`).
2. **A bug is a test first.** Every defect gets a regression test that fails on the bug, before the fix.
3. **Every slice lists its happy, sad and bad paths up front** (§9). A slice with only happy paths is not
   finished, and reviewers check the list, not the coverage number.
4. **Tests run in Docker.** The container is the canonical environment, the same one CI uses, so "works
   on my laptop" has no meaning here. A quick local `go test` for one package is allowed while iterating;
   it never counts as a gate.
5. **Real SQLite, always.** `modernc.org/sqlite` is pure Go and fast; mocking the database would hide the
   bugs we care about (constraints, transactions, WAL, crashes).
6. **No real financial data in tests, ever.** Fixtures are synthetic and generated at test time (§6). A
   privacy test fails the build if a real-looking statement ends up in the repo.
7. **Money is integers in every test too.** No test uses a float for an amount, including in expected
   values.
8. **Determinism.** Time, randomness and ordering are injected. A test that depends on the wall clock or
   map iteration order is a bug.
9. **Fuzz crashers are kept.** Anything a fuzzer finds is committed under `testdata/fuzz/` and runs on
   every build thereafter.

## 2. Happy, sad and bad paths

Three words, used the same way everywhere, including in test names.

| Path | Meaning | Example |
|---|---|---|
| **happy** | Valid input, expected outcome | A clean Delta statement imports; every row reconciles |
| **sad** | Valid-looking input, correct refusal or unwelcome outcome | A statement whose balances do not add up is refused and the failing row is named; a duplicate statement imports nothing; an empty month renders |
| **bad** | Malformed, hostile or broken environment | A truncated PDF, a 2 GB upload, a binary file renamed `.pdf`, a crash mid-commit, a full disk, two imports at once, SQL or HTML in a description |

**Naming convention.** Table tests use subtests named `happy/...`, `sad/...`, `bad/...`, so
`go test -run '/bad'` runs every bad path in the project and `-run '/sad'` every sad one. A slice's list
in §9 maps one to one to those subtests.

**What "bad" always includes**, checked for every boundary that takes outside input (file, request,
config, database):

- empty, one byte, and enormous (past the size cap)
- wrong type (not a PDF, wrong bank's PDF, encrypted PDF, scanned PDF with no text layer)
- truncated, bit-flipped, duplicated, reordered
- hostile strings in descriptions (quotes, SQL, HTML, control characters, right-to-left marks, NUL,
  invalid UTF-8, very long)
- the same thing twice, and the same thing concurrently
- the environment failing under it (read-only filesystem, disk full, process killed mid-write)

## 3. Test layers and tools

| Layer | What it proves | Tool | Speed |
|---|---|---|---|
| **Unit** | Pure functions: money, dates, normaliser, fingerprint, line grammars | `testing`, table-driven | milliseconds |
| **Golden** | A parser turns a fixture into exactly this ledger; JSON responses keep their shape | golden files in `testdata/`, `-update` flag | milliseconds |
| **Fuzz** | Nothing panics, hangs or silently corrupts, on inputs nobody thought of | native `go test -fuzz` (stdlib) | seconds to minutes |
| **Property / model** | Invariants hold across random sequences of operations (§4) | `pgregory.net/rapid` (approved) | seconds |
| **Integration** | Real SQLite, real parsers, real HTTP handlers together | `httptest`, temp databases | sub-second each |
| **Crash / environment** | Atomic commits survive `kill -9`; disk-full and read-only fail cleanly | subprocess tests, tmpfs volumes in Docker | seconds |
| **Container** | The shipped image behaves: non-root, read-only rootfs, no egress, health, restart | build-tagged `e2e` tests driving `docker compose` | tens of seconds |
| **Frontend unit** | Pure UI logic: amount formatting, month navigation, state mapping | `vitest` | milliseconds |
| **Browser e2e** | The real UI against the real server, on the user's flows, at phone and laptop widths; screenshots against the mockups; accessibility checks | Playwright (approved) | seconds each |
| **Static** | Style, vet, races, known vulnerabilities | `go vet`, `staticcheck`, `govulncheck`, `-race` | seconds |

Test-only dependencies never enter the shipped binary. The server's runtime dependency list stays at
`modernc.org/sqlite` and `ledongthuc/pdf`.

## 4. Fuzzing and properties

### Native fuzz targets

Each target is seeded from its table test, so the happy, sad and bad cases are the starting corpus.

| Target | The property it checks |
|---|---|
| `FuzzParseMoney` | Never panics; accepts only two-decimal forms; `format(parse(x)) == x` for accepted input; never rounds; never overflows `int64` silently |
| `FuzzNormalise` | Never panics; idempotent (`n(n(x)) == n(x)`); output is valid UTF-8; never longer than a bound |
| `FuzzFingerprint` | Deterministic; length-prefixed encoding so no two different field tuples collide (a `|` in a description cannot forge another row) |
| `FuzzOccurrence` | Any permutation of the same rows yields the same set of fingerprints |
| `FuzzDeltaLine` | The line grammar never panics and never invents an amount |
| `FuzzCapOneLine` | Same, for Capital One |
| `FuzzExtractPDF` | Arbitrary and mutated PDFs: no panic, no hang (hard timeout), bounded memory |
| `FuzzImportSequence` | Random import, undo and re-import sequences keep every invariant below |
| `FuzzMigrate` | Migrations apply to any prior schema version, or refuse cleanly |

Mutation testing (`gremlins`, approved for later) comes after Slice 9: it breaks the importer on purpose
and checks that a test notices, which measures what coverage cannot.

Cadence: 30 seconds per target on every pull request; 10 minutes per target on a nightly schedule.

### Invariants (property tests and the model test)

These must hold after *any* sequence of operations:

1. **Idempotency.** Importing the same statement again changes nothing.
2. **Undo is an exact inverse.** Import then undo returns the database to byte-equal row content.
3. **No duplicate fingerprints**, and no fingerprint changes when the normaliser changes.
4. **Reconciliation.** For every account, opening balance plus the sum of its rows equals the closing
   balance of the latest statement.
5. **Totals.** `in + out = net` for every month; transfers are in neither; month totals equal the SQL sum
   of their rows.
6. **Amounts and dates of an imported row never change** through any API.
7. **Integers on the wire.** No money field is ever serialised with a decimal point or exponent.
8. **Nothing is written by a preview.**

## 5. What is real and what is faked

| Thing | In tests | Why |
|---|---|---|
| SQLite | **Real**, temp file or in-memory | The bugs live in constraints, transactions and WAL |
| Parsers, normaliser, fingerprint, importer | **Real** | They are the product |
| PDF library | Real for a few integration tests; behind an `Extractor` interface, so parser tests run on plain text fixtures without any PDF | Fast, and parser logic stays testable on its own |
| Clock | **Fake** (`Clock` interface) | The overdue banner, retention and "36 days ago" need exact control |
| Mirror (`rclone` to R2) | **Fake** runner | Network and credentials must never be in tests; failures (R2 down, partial upload, auth) are simulated |
| Update watcher | **Fake** | Rollback paths are simulated, not performed |
| Filesystem faults | **Real, in Docker** (tiny tmpfs for disk-full, read-only mount) | Faking these proves nothing |
| HTTP | `httptest` in-process, plus the real container in e2e | Both fast and faithful |
| Frontend API | A stub server serving the contract's golden JSON | UI tests do not need the Go server |

The static HTML in `docs/mockups/` is the **visual spec**. Browser tests screenshot each real screen at 390
and 1280 pixels and compare against screenshots of the matching mockup, so a UI change that drifts from
the approved design fails loudly. The "nothing leaves the box" rule is enforced three ways: the browser
suite fails on any request that leaves the origin; the server sends
`Content-Security-Policy: default-src 'self'` on every response (a Go test asserts it, and a browser
enforces it); and the container suite runs the whole app with `--network none`.

## 6. Synthetic fixtures

Real statements hold a name, an address and account numbers, so they are never committed (`*.pdf` is
ignored). Tests need PDFs anyway, so we **generate** them.

- A small pure-Go PDF writer (about 100 lines, test-only) emits statements whose *structure* matches the
  two real layouts: for Delta, a single text block of fixed-width Courier lines stepped with `Td`, glyph
  codes through a custom encoding and `ToUnicode` map, plus a background image for the headings; for
  Capital One, positioned proportional text with `Tm`. Content is invented merchants and amounts.
- Every fixture is stamped `Producer: coffer-synthetic`; the privacy test rejects any PDF in the tree
  without it.
- A **mutation set** derives sad and bad fixtures from a good one: flip one sign; drop a row; duplicate a
  row; swap two rows; split a row across a page; orphan a detail line at the top of a page; remove the
  ending balance; remove a page; change the period; truncate; corrupt the xref table; encrypt it; replace
  the text layer with an image; add a decompression bomb.
- The real files stay on the author's machine. A manual `make verify-real PATH=...` runs the reconcile
  check on a real statement and prints counts and pass or fail only, as the spike does.

## 7. Docker

Docker is how everything runs: tests, fuzzing, the web build, the e2e suite and the shipped image.

**Files** (to be created in Slice 0): `Dockerfile`, `compose.yml` (production shape), `compose.test.yml`,
`Makefile`, `.github/workflows/`.

**Dockerfile stages**

| Stage | Contents | Purpose |
|---|---|---|
| `deps` | Go modules and pnpm store, cached | Fast rebuilds |
| `test` | Full Go toolchain with gcc (the race detector needs cgo), Node, pnpm | Runs every test in this document |
| `web` | `pnpm build` of the SvelteKit app | Static bundle for embedding |
| `build` | `CGO_ENABLED=0` static Go binary with the bundle embedded | The one artefact |
| `final` | distroless, non-root, read-only rootfs, `/data` the only writable mount | What ships |

**Commands** (wrapped by `make`)

| Command | Runs |
|---|---|
| `make test` | `go vet`, `staticcheck`, `go test -race -cover ./...`, `vitest` |
| `make fuzz T=FuzzParseMoney` | One fuzz target for a chosen time |
| `make bad` | Only `/bad` subtests, for a fast safety sweep |
| `make e2e` | Builds `final`, starts it, runs the container and browser suites |
| `make image` | Builds the production image |
| `make verify-real PATH=...` | Reconcile check on a real statement (local only) |

**Container tests** (build tag `e2e`) assert the claims `PLAN.md` makes about the image:

- runs as a non-root user; root filesystem is read-only; writing anywhere but `/data` fails
- `/healthz` returns only `status` and `version`
- works with `--network none`: the app makes no outbound requests, ever
- survives `docker kill` during an import: the database is either fully before or fully after
- restart keeps data; a full-disk `/data` volume yields a clear error, not a corrupt database
- a backup taken from the running container restores into a scratch container and matches
- the same suite passes on **amd64 and arm64**, because the potato is arm64

## 8. Coverage and gates

Coverage is a floor, not the goal. A line can be covered and still wrong, which is why §9 lists named
paths and the fuzz and property layers exist.

| Gate | Threshold |
|---|---|
| Statement coverage, `money`, `fingerprint`, `normalise`, `parse/*`, `importer`, `store` | 95% |
| Statement coverage, everything else in Go | 85% |
| Frontend pure logic | 90% |
| `go vet`, `staticcheck`, `govulncheck` | zero findings |
| `go test -race` | clean |
| Fuzz targets | all seeds pass; no unresolved crashers in `testdata/fuzz/` |
| Privacy test | no PDF without the synthetic stamp; no file matching a long-digit account pattern |
| Every sad and bad path in the slice's §9 list | has a named subtest |

CI runs on every push and pull request: the full `make test`, a 30-second fuzz pass per target, and
`make e2e` on amd64 and arm64. A nightly job runs the long fuzz. The publish job runs only after all
of that is green.

## 9. The slices, in order

Each slice is built test-first. The three columns are the test list written **before** the code.

### Slice 0: the harness (done 2026-10-31)
Docker stages, compose, Makefile, CI, the privacy test, a deliberately failing test that is then made to
pass. **Done when:** an empty project is green on amd64 and arm64 and `make bad` runs (and finds nothing).

What it delivered, beyond the list: `make test` is one gate (gofmt, vet, staticcheck, race detector,
per-package coverage floors from `coverage-gates.txt`); `internal/privacy` scans the repo on every run;
`internal/covgate` enforces the floors and fails closed; `internal/version` validates the build stamp
and carries the first fuzz target; the production image is 1.6 MB, non-root, and runs with a read-only
root, no network and all capabilities dropped. Verified on arm64 and on emulated amd64, and the gate was
shown to fail (impossible coverage floor, badly formatted file). Two things the gate caught on its own
first run: a Go pseudo-version timestamp that looked like an account number, and coverage below the
floor on two packages. `CLAUDE.md` carries these conventions for whoever builds the next slice.

### Slice 1: money
| happy | sad | bad |
|---|---|---|
| `$1,234.56`; `1234.56-` (debit); `(5.00)`; `-$5.00`; `0.00`; `$0.01`; format round-trip | `12.5`; `12.345`; `12`; empty; `$`; `1,23.45`; two signs | `1e5`; `NaN`; `٣.٠٠` (non-ASCII digits); NUL; overflow past `int64`; very long digit strings; right-to-left marks |

### Slice 2: dates and periods
| happy | sad | bad |
|---|---|---|
| `Oct 1` resolved against a period; December to January rollover; leap day; two-digit years | a date outside the statement period (flagged, not guessed); a post date before its transaction date | `Feb 30`; month 13; `31/09`; empty; a period that ends before it starts |

### Slice 3: the normaliser
| happy | sad | bad |
|---|---|---|
| Strip store numbers and 22-digit references; collapse whitespace; case folding; stable across runs | A description that normalises to empty keeps the raw text; unfamiliar type lines pass through | 1 MB string; invalid UTF-8; control characters; SQL and HTML payloads; NFC and NFD forms |

### Slice 4: the fingerprint
Length-prefixed field encoding, not a `|` join, so a delimiter inside a description cannot collide.
| happy | sad | bad |
|---|---|---|
| Deterministic; two identical same-day rows get occurrences 0 and 1; permuting rows keeps the set | Same row in two accounts differs; the normaliser changing never changes it | A description containing the delimiter; a million identical rows; empty fields |

### Slice 5: the store (SQLite, migrations)
| happy | sad | bad |
|---|---|---|
| Migrations apply to an empty database and are idempotent; constraints (`UNIQUE` fingerprint, statement key); foreign keys on; integer-only amounts | A database newer than the binary is refused; a `UNIQUE` violation rolls the whole file back | `kill -9` mid-commit; disk full; read-only volume; two writers; `PRAGMA integrity_check` after abuse; file permissions `0600` |

### Slice 6: PDF fixtures and the extractor
| happy | sad | bad |
|---|---|---|
| Synthetic Delta and Capital One PDFs yield the expected lines (including glyph-coded text and the `Td` stepping the library loses) | No text layer (scanned) refused with a clear message; encrypted PDF refused; zero-page PDF | Truncated; bit-flipped; not a PDF; huge page count; decompression bomb; infinite content stream (hard timeout); `FuzzExtractPDF` |

### Slice 7: the Capital One reader
| happy | sad | bad |
|---|---|---|
| Purchases and a payments-and-credits block; fees and interest; year-crossing cycle; all four reconciliation checks pass | Totals disagree (first failing row named); an unmatched line inside a section; a refund shown as a negative purchase | Missing summary; duplicated section; a different card's statement; foreign-currency row (rejected loudly) |

### Slice 8: the Delta reader
| happy | sad | bad |
|---|---|---|
| Two sub-accounts; rows spanning a page break; an orphaned detail line; deposits; chain and ending balance pass | One misread sign: the chain fails and names exactly that row; missing `Ending Balance`; overdrawn (trailing-minus balance); a block with zero rows | A page missing (chain breaks and is reported); a page duplicated; rows shuffled; a different period than expected; an unknown sub-account ID |

### Slice 9: the import service
Preview, commit, undo, the statement key. The model test lives here.
| happy | sad | bad |
|---|---|---|
| Preview writes nothing; commit is atomic; two identical same-day purchases both import; undo then re-import works; sections map to accounts by `statement_ref` | Same file twice (hash); same period re-downloaded with different bytes (statement key); unknown `statement_ref` asked about, never guessed; reconciliation failure refuses the whole file | Two imports at once (serialised); context cancelled mid-commit (rolled back); `FuzzImportSequence` with random import and undo; an oversize body |

### Slice 10: the HTTP API
| happy | sad | bad |
|---|---|---|
| Every endpoint's golden JSON; integers and ISO dates only; `PATCH` changes only note and merchant | 404; 405; 409 on duplicate; 422 on a refused statement, with the reason | Malformed multipart; 413 oversize; 415 not a PDF; a `PATCH` that tries to change an amount or date is refused; request smuggling; no CORS; `/healthz` leaks nothing |

### Slice 11: summaries, balances, transfers, the overdue banner
| happy | sad | bad |
|---|---|---|
| Month totals match the SQL sum; merchant rollup; per-account balances as of the statement date; suggested transfer pair (equal and opposite, within 5 days, different accounts) | Empty month; a refund; two possible pairs (no automatic pairing); exactly 35 and 36 days with a fake clock | A pair across the same account; an import in the future; clock going backwards |

### Slice 12: the frontend
| happy | sad | bad |
|---|---|---|
| `formatCents` (never floats, true minus sign); month navigation; state-to-label mapping; each screen matches its mockup at 390 and 1280 pixels; the import, undo and add-account flows end to end in a real browser | Empty states; overdue banner; refused-statement message; rejected-row list | API down; slow API; a 500; a description with HTML (rendered as text, never as markup); **any request leaving the origin fails the test**; the Content-Security-Policy header test; axe accessibility checks pass; keyboard-only walk through each flow |

### Slice 13: backups, export, verify
| happy | sad | bad |
|---|---|---|
| Snapshot via the online backup API restores to a matching database; export, wipe, import round-trips byte-equal; `coffer verify` passes on a good database | The fake mirror reports R2 down, auth failure, partial upload: status shows failure, never silent | Corrupted snapshot detected; a snapshot taken during a write; retention deleting only what it should |

### Slice 14: the container and updates
The container suite from §7, plus the update watcher's rollback simulated against the fake, and
`shellcheck` on the scripts.

## 10. Approvals needed

`PLAN.md` says to ask before adding any dependency. All of these are **development-only** and never enter
the shipped binary.

| Dependency | Used for | Recommendation |
|---|---|---|
| `pgregory.net/rapid` | Property-based and state-machine tests (§4 invariants) | **Approved.** Native fuzz covers inputs; this covers sequences of operations |
| Playwright | Browser e2e, screenshots against the mockups, axe checks | **Approved** (2026-10-31). Runs in its own Docker image, never in the shipped one |
| `staticcheck`, `govulncheck` | Static analysis and vulnerability scanning, pinned and run via `go run` | **Approved** |
| A mutation tester (`gremlins`) | Proves the tests fail when the code is broken | **Approved, for later**, once there is code worth mutating |
| A Svelte component-test library | Rendering components in a test | **Not needed for now.** Playwright covers rendered behaviour; `vitest` covers pure logic |

## 11. Definition of done

A slice is done when **all** of these are true:

- every path in its §9 table is a named, passing subtest, written before the code
- the §8 gates pass inside Docker on amd64 and arm64
- its fuzz target (if any) ran its pass with no crasher, and past crashers are committed
- `PLAN.md` and `NOTES.md` say what the code now does, in the same commit as the change
- no real financial data is anywhere in the diff
