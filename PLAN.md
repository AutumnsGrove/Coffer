# Coffer — Plan

A private, self-hosted finance tracker. Not a bank connection and not a budgeting engine: a place
where a monthly CSV export turns into a readable picture of where the money went, kept forever on my
own machine.

The source of truth for what gets built. Read this before writing code; update it when a decision
changes, in the same commit as the change. Anything still unknown belongs in
[§3 Open questions](#3-open-questions), not in code.

## Contents

1. [What it is](#1-what-it-is)
2. [Decisions already made](#2-decisions-already-made)
3. [Open questions](#3-open-questions)
4. [Stack](#4-stack)
5. [Architecture](#5-architecture)
6. [Money, and why it is not a float](#6-money-and-why-it-is-not-a-float)
7. [Data model](#7-data-model)
8. [Importing a CSV, carefully](#8-importing-a-csv-carefully)
9. [The screens](#9-the-screens)
10. [API](#10-api)
11. [Where it runs](#11-where-it-runs)
12. [Backups and export](#12-backups-and-export)
13. [Phases](#13-phases)
14. [Costs](#14-costs)
15. [Rules that are not negotiable](#15-rules-that-are-not-negotiable)
16. [Working agreement](#16-working-agreement)

---

## 1. What it is

Once a month — not daily, deliberately — I download each of my two banks' monthly **statement** (a PDF
today; a CSV wherever one is available) and drop it into Coffer. Coffer:

1. **identifies the file's shape** from a per-institution column mapping (a config entry, never
   branching code), including which column is the amount, which date format it uses, and whether
   spending is positive or negative;
2. **normalises each row** — trims, strips `POS DEBIT`/`CHECKCARD` prefixes and trailing store and
   card numbers — and computes a **fingerprint** so the same transaction can never be imported twice;
3. **shows a preview** before writing anything: how many rows are new, how many are already known, the
   date range they cover, and the money in and out;
4. **stores the ledger** — normalised rows *and* the original row, so a parser fix never means finding
   the file again;
5. **shows the month** — spend, income, net, the merchants that actually recur, and per-account totals;
6. **tells me when it is overdue.** A banner saying "last import: 34 days ago" in amber, because the
   failure mode this app has to beat is me forgetting, not me miscalculating.

The research behind all of this (Plaid vs SimpleFIN vs Teller vs OFX vs CSV, and why CSV wins for two
personal accounts) is in Polaris thread `3ec8d2f8-0c0b-4aa9-8510-6dba91357966`. The short version:
Plaid is the gold standard for a company onboarding strangers, and the wrong tool for two accounts
behind a tailnet. CSV costs nothing, stores no credentials, has no terms-of-service exposure, and
tolerates a month of lag — which is the cadence I actually want anyway.

### Goals

- One honest picture of a month's money, from files I already have access to.
- Imports that are **idempotent**: re-uploading the same month, or overlapping ranges, changes nothing.
- Raw rows kept forever; every derived number re-computable from them.
- Merchant-level insight before category-level insight ("$340 at Publix this month" is most of the
  value of a taxonomy for a fraction of the work).
- A reminder that works without willpower: the app knows when it is overdue and says so.
- Local-first. SQLite on the potato (one file, `data/coffer.db`), a plain mirror to my own private R2 bucket, plain-CSV export that outlives the app.
- Data source behind an interface, so OFX or an aggregator can be added as a sibling implementation
  later without touching the importer, the database or the UI.

### Non-goals (v1)

- Live or daily bank connections. No Plaid, no SimpleFIN, no Teller, no scraping, no credentials.
- Editing transactions that a bank recorded. The ledger reflects the statement; notes and merchant
  names are mine to change, amounts and dates are not.
- Budgets and envelopes. Later, if the month view proves it's wanted.
- Categories as a taxonomy. v1 rolls up by normalised merchant; rules come after I see which merchants
  actually recur.
- Accounting: no double-entry, no reconciliation, no multi-currency, no tax reporting.
- Multi-user, sharing, accounts, publishing.
- Mobile apps. A phone-friendly PWA.

## 2. Decisions already made

| Topic | Decision |
|---|---|
| Name | **Coffer** — decided 2026-10-31 |
| Data source (v1) | **Monthly statement PDFs.** Manual, per-institution. Confirmed 2026-10-31: neither Capital One (new account) nor Delta Community offers a CSV or QFX download to this account, so PDF is the only way in, not a fallback. CSV stays a sibling `Source` for a future bank that has one |
| PDF reading | **Built into the Go server**, as a `Source` sibling to CSV: text layer → rows, never OCR, nothing sent anywhere. Library: **`ledongthuc/pdf`**, approved and proven in the Phase 0 spike (see NOTES.md); line geometry is rebuilt on top of its `Interpret` because its own text API fails on Delta's files |
| Statement checks | A PDF import must **prove itself** before the preview is offered: Delta rows are checked against the printed running balance, Capital One rows against the printed totals. A statement that does not reconcile is refused, naming the row where it diverges |
| Balances | Stored per account from each statement (opening, closing, as-of date), shown on Home labelled with the statement date, never typed in by hand |
| Statement dedupe | A statement is identified by (account, period) as well as by file hash, so a re-downloaded copy of the same month is caught even though its bytes differ. Row fingerprints still back it up |
| One file, many accounts | A Delta statement holds savings and checking together. The import step maps each section to an account, so one file can write to several |
| Data source (later) | OFX / Teller / Plaid sit behind the same `Source` interface — siblings, never rewrites |
| Import shape | Two-step: upload → **preview** (new / duplicate / range / totals) → confirm. Nothing is written before the confirm |
| Dedupe | Deterministic fingerprint: `sha256(account_key, posted, amount_minor, description_raw (whitespace-collapsed only), occurrence)`, `UNIQUE` in the database. Hashes the bank's text, **not** the normalised form, so improving the normaliser can never change a fingerprint and re-import duplicates. Idempotency is a required property, not a nice-to-have |
| Account at upload | Chosen explicitly on the Import screen (pre-selected from the last import with the same header signature). The header signature identifies the file's *shape*, never its account |
| Transfers | **Simple transfer flag in v1.** Any transaction can be marked a transfer; a suggested-pair hint appears when an equal and opposite amount exists in another account within 5 days. Transfers are excluded from spend, income and net, and shown as their own line |
| Undo import | Settings > Import history has **Undo**, which deletes every row from that `import_id` in one transaction and keeps the `imports` row marked `undone_at`. This is the only way a row is ever deleted. The file hash is released so the corrected file can be re-imported |
| Mappings | For CSV: embedded defaults plus an optional `mappings.json` in `/data`, keyed by header signature, so a new bank needs no rebuild. **For PDF this does not hold**: a statement layout is code, one parser per institution, covered by golden tests on synthetic fixtures. A bank redesigning its statement is a code change and a release. No in-app editor in v1 |
| Occurrence counter | Why two identical $4.75 coffees on one day both import, while re-importing the same file inserts nothing |
| Raw rows | The statement lines each row was read from (type line plus detail lines) are stored with it, forever, as JSON, so a normaliser or merchant-rule fix re-derives without re-importing |
| Original PDFs | **Not kept** (decided 2026-10-31). The PDF is read in memory and discarded; only extracted text and numbers are stored. Cost: a bug in how a *layout is read* (not how a row is interpreted) is fixed by re-downloading the statement, which the bank keeps. Benefit: no file with a name, address and account number ever sits on disk or in R2 |
| Accounts | Three to start: `capone-credit`, `deltacu-checking`, `deltacu-savings` (confirmed 2026-10-31). **Accounts are added in Settings**, any time: a key (fixed once created), a display name (editable), institution, type, and which statement layout reads it. An account is archived, never deleted. A new account at an institution that already has a parser needs no code; a new institution needs a new parser (a release) |
| Column mappings | Per-institution config (data file, keyed by header signature), never `if institution == …` in code |
| Insight granularity | **Normalised merchant rollup** in v1; categories/rules deferred |
| Cadence | Monthly. The UI assumes a monthly ritual and shows the age of the last import; there is no daily-refresh button because there is nothing to refresh |
| Overdue banner | Part of v1, not polish. Amber after the configured interval (default 35 days, so a monthly habit with a few days of slack never trips it) |
| Money | **Integer minor units** (cents) in the database and on the wire. Never a float. See [§6](#6-money-and-why-it-is-not-a-float) |
| Storage | SQLite (`modernc.org/sqlite`, pure Go, no cgo) — the same choice as Warbler |
| Language / UI | **Go + SvelteKit**, single binary with the frontend embedded via `go:embed`, matching Polaris, bwmon, Warbler and Meridian |
| Hosting | The potato, in Docker Compose, image pinned by digest in GHCR, `tailscale serve` on **port 8894** (verified free 2026-10-31) |
| Updates | The established pattern: host-side watcher, digest pinned in `.env`, health check, rollback |
| Auth | None. Tailnet-only is the boundary, same as Polaris and Warbler |
| Backups | Nightly compressed snapshot + `rclone` mirror to R2 to the existing private `r2:` remote as plain files (decided 2026-10-31: no crypt layer), plus a CSV/JSON export on demand |
| Discovery | A Meridian tile whose vital is **days since last import** — the one number that means something here |
| Reminder (optional) | A monthly email through the Resend setup already in `maintenance-setup/`, if the in-app banner proves insufficient |
| Licence | MIT, matching the rest of the homelab |

## 3. Open questions

Answer these before Phase 1 finishes; the ones marked *(blocking)* gate the importer.

1. ~~**Which accounts exist in v1, and what do I call them?**~~ **Decided 2026-10-31:** `capone-credit`,
   `deltacu-checking`, `deltacu-savings`, with more addable from Settings at any time (§2). Keys are
   fixed once created because they appear in every fingerprint; display names stay editable.
2. **Real files** *(partly answered 2026-10-31)*: one statement PDF from each institution is in hand and
   analysed in NOTES.md. The Capital One account is new, so its only statement is a first cycle with four
   purchases and an **empty** "Payments, Credits and Adjustments" block: that layout is unverified until
   the first payment posts, and until then it is built from a synthetic fixture and flagged as such.
   Still wanted: a Delta statement that includes a transfer. Every parser decision comes from the real
   files, not from guessing layouts.
12. ~~**Which PDF library?**~~ **Decided 2026-10-31: `github.com/ledongthuc/pdf`**, approved, and the
    spike confirmed it. Pure Go, so the image stays distroless. One caveat found: its own
    `Content()` and `GetTextByRow()` lose all positions on Delta's PDFs (every glyph at one point), so
    Coffer rebuilds line geometry from the library's public `Interpret` and font decoder (about 150
    lines, `spike/pdfread/extract.go`). The library is a single-maintainer pseudo-version, so it is
    pinned in `go.sum` and should be vendored before Phase 4.
13. ~~**Is a CSV or QFX download available?**~~ **Answered 2026-10-31: no**, for either institution.
    PDF statements are the only source, so the PDF parser is the core of v1 rather than an add-on.
3. **Sign convention per institution** *(blocking)*: is a purchase positive or negative in each
   export? Resolved by question 2, but worth stating: the importer has a per-mapping `invert` flag and
   the preview shows the result so a wrong guess is visible before it's committed.
4. **Currencies**: USD only? (Recommendation: yes, and reject any row that isn't, loudly.)
5. ~~**Balances**~~ **Decided 2026-10-31: store and show on Home.** Statements print opening and closing
   balances, so nothing is typed by hand. Stored per account with an as-of date (see `account_balances`),
   shown on Home labelled with the statement date, and used by the import checks. A balance is always
   "as of the last statement", never live.
6. ~~**Transfers between my own accounts**~~ **Decided 2026-10-31:** simple transfer flag in v1 with a
   suggested-pair hint; transfers are excluded from spend, income and net. See §2.
7. ~~**Backfill**~~ **Decided 2026-10-31: yes, the last 12 months of Delta Community statements** (about 12
   PDFs to download by hand, Phase 3). Capital One is a new account with nothing to backfill.
8. ~~**Backup encryption**~~ **Decided 2026-10-31: plain mirror**, no `rclone crypt`. The bucket is private and
   yours; the cost is that anyone with the R2 credentials could read the statements' contents, and that
   the credentials on the potato are the only protection. Easy to add later: a crypt remote over the same
   bucket, with a one-time re-upload.
9. **Merchant normalisation**: is an automatic strip-the-noise rule set plus a manual "this is the same
   merchant" merge good enough, or is a per-merchant override table needed on day one?
   (Recommendation: automatic rules + a rename/merge action, which is the same table.)
10. **Monthly reminder day**: which day should the banner consider "overdue"? (Recommendation: 35 days
    since the last import, whatever day that lands on.)
11. **Agents**: does Coffer ever get an agent (Nova, as OmniReviewer plans to)? (Recommendation: no.
    This is arithmetic over my own rows; deterministic code is the feature. Revisit only if a
    "summarise this month in words" step proves worth a model call.)

## 4. Stack

| Layer | Choice | Why |
|---|---|---|
| Language | Go 1.26 | Already on the box; every other service here is Go |
| HTTP | `net/http` + `http.ServeMux` (method+path patterns, Go 1.22+) | No router dependency needed for ~12 routes |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no cgo, so arm64 cross-builds stay trivial (Warbler's choice for the same reason) |
| Migrations | Embedded numbered `.sql` files + a `schema_version` table, applied on boot inside one transaction | Boring, reviewable, works offline |
| CSV | `encoding/csv` + a per-institution mapping config | The mapping is data, the reader is code |
| Frontend | SvelteKit + TypeScript, `adapter-static`, SPA mode, pnpm | Same as Polaris, bwmon, Warbler |
| Embedding | `go:embed` of the built web bundle into the single binary | No Node.js at runtime |
| Fonts/icons | Bundled only (a font file, Lucide icons) | No outside requests, verifiable by test |
| Tests | `go test` for parsing/fingerprint/idempotency, fuzz targets for CSV and money parsing, `vitest` for pure UI logic, golden files per institution fixture | The dedupe and money paths are where silent corruption lives |
| Container | Multi-stage → distroless, non-root, read-only rootfs, `/data` volume, loopback port 8894 | Matches Warbler exactly |
| Images | GHCR, multi-arch (amd64+arm64), pinned by digest in `.env`, `r<count>.<sha>` version stamp | Matches Warbler |
| Updates | Host watcher (`coffer-update.path`/`.service`), health check, rollback | Matches the homelab pattern; the app never talks to GHCR |
| CI | GitHub Actions: tests on every push; publish on main and on `v*` tags; no secrets in the repo | Matches the rest |

**No outside dependencies in the server.** `modernc.org/sqlite` is the one exception, and it is
vendored-or-pinned deliberately; anything else needs asking first. **`ledongthuc/pdf` is the second
exception**, approved 2026-10-31 after the Phase 0 spike showed it reads both real statements (§3
question 12).

## 5. Architecture

```
 PHONE / LAPTOP (Tailscale on)
 ┌──────────────────────────────────────────────────────────────┐
 │ SvelteKit PWA                                                │
 │  Import (file picker) · Month view · Transactions · Settings │
 └──────────────────────────┬───────────────────────────────────┘
                            │ HTTPS via tailscale serve :8894
 THE POTATO (Docker)        ▼
 ┌──────────────────────────────────────────────────────────────┐
 │ Go service (127.0.0.1:8894)                                  │
 │   net/http API + embedded SvelteKit build                    │
 │   importer: mapping → normalise → fingerprint → preview      │
 │   sources:  CSV (v1) │ OFX (later) │ aggregator (later)      │
 │   SQLite: accounts · imports · transactions · merchants      │
 │   data/coffer.db · data/exports/                             │
 └──────────────────────────┬───────────────────────────────────┘
                            │ nightly: zstd snapshot + rclone
                            ▼
                    Cloudflare R2  r2:coffer-backups
```

- **The `Source` interface is the load-bearing abstraction** (~15 lines). A source yields normalised
  transactions; the importer, dedupe, database and UI never learn which one produced them.
- **Nothing is fetched.** Every byte in this app arrives because I uploaded a file or typed a value.
  The only outbound traffic the container causes is the R2 mirror (host-side) — and none at all from
  the container itself if the mirror stays in the host watcher.
- **The app never deletes a transaction, except by undoing a whole import.** Re-imports are no-ops via
  the fingerprint; a genuinely wrong row is corrected by an explicit action that keeps the original in
  `raw`; a wrong *file* (bad sign convention, wrong account) is removed as a unit with Undo.

## 6. Money, and why it is not a float

Two rules, both boring, both load-bearing:

1. **Store minor units as integers.** `amount_minor INTEGER NOT NULL` — 1234 means $12.34 for a USD
   account. Floats lose cents; `0.1 + 0.2 != 0.3` in a budget app is a support ticket that lasts
   forever. Aggregations are `SUM(amount_minor)` in SQL, exact by construction.
2. **Parse money as text, not as float.** `"$-1,234.56"`, `"(1234.56)"` (accounting negative),
   `"1234,56"` (comma decimal) and `"1 234.56"` all appear in real exports. A single
   `ParseMoney(string) (int64, error)` handles the forms the mappings declare, and a fuzz test proves
   it never silently rounds or truncates.

Explicitly *not* doing: a decimal library dependency, or storing formatted strings. The formatting
belongs to the UI layer and nowhere else.

## 7. Data model

```sql
accounts (
  id INTEGER PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,           -- "capone-checking"; part of every fingerprint
  institution TEXT NOT NULL,          -- "Capital One" | "Delta Community"
  name TEXT NOT NULL,                 -- what I call it
  type TEXT NOT NULL,                 -- checking | savings | credit | loan | cash
  currency TEXT NOT NULL DEFAULT 'USD',
  backlog_from DATE,                  -- earliest date CSV history will cover
  created_at, archived_at
)

imports (                             -- one row per file, always
  id INTEGER PRIMARY KEY,
  account_id INTEGER REFERENCES accounts(id),
  filename TEXT NOT NULL,
  file_hash TEXT NOT NULL,            -- sha256 of the bytes: same file twice = refused early
                                      -- (UNIQUE among rows where undone_at IS NULL, via partial index)
  undone_at DATETIME,                 -- set by Undo; the rows are deleted, this record stays
  source TEXT NOT NULL,               -- csv | ofx | …
  mapping TEXT NOT NULL,              -- which column mapping was used
  imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  rows_seen INTEGER, rows_new INTEGER, rows_duplicate INTEGER, rows_rejected INTEGER,
  range_from DATE, range_to DATE,
  statement_key TEXT,                 -- "account|period" for a statement; partial UNIQUE while undone_at IS NULL
  checks TEXT,                        -- JSON: what reconciled (opening, closing, per-row balances) and how
  notes TEXT
)

transactions (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id),
  import_id INTEGER REFERENCES imports(id),
  posted DATE NOT NULL,               -- the bank's posting date, no time
  amount_minor INTEGER NOT NULL,      -- negative = money out
  currency TEXT NOT NULL DEFAULT 'USD',
  description_raw TEXT NOT NULL,      -- exactly as the bank printed it
  description_norm TEXT NOT NULL,     -- normalised, for matching and search; NOT in the fingerprint
  merchant_id INTEGER REFERENCES merchants(id),
  fingerprint TEXT NOT NULL UNIQUE,
  is_transfer INTEGER NOT NULL DEFAULT 0,   -- v1: excluded from spend/income/net
  transfer_pair_id INTEGER REFERENCES transactions(id),  -- optional link to the other leg
  note TEXT,                          -- mine, never the bank's
  raw TEXT NOT NULL                   -- the statement lines this row was read from, as JSON (never the PDF)
)

merchants (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,          -- display name, editable
  first_seen DATE, last_seen DATE,
  aliases TEXT                        -- JSON array of the normalised forms merged into it
)

account_balances (                   -- from each statement; Home shows the latest per account
  account_id INTEGER NOT NULL REFERENCES accounts(id),
  as_of DATE NOT NULL,                -- the statement's end date
  balance_minor INTEGER NOT NULL,     -- credit cards: amount owed, as printed
  import_id INTEGER NOT NULL REFERENCES imports(id),   -- undo removes it with the import
  PRIMARY KEY (account_id, as_of)
)

category_rules / categories          -- v1.5+, not in v1 by decision

view: month_summary                  -- in/out/net per month, derived, never stored
view: merchant_month                 -- SUM by merchant per month, derived
```

Fingerprint:

```
fingerprint = sha256(account_key | posted | amount_minor | description_raw_collapsed | occurrence)
```

`description_raw_collapsed` is the bank's text with runs of whitespace collapsed and nothing else done
to it. Normalisation rules (prefix stripping, store numbers) are free to evolve because they never
touch the fingerprint; a rule change re-derives `description_norm` and `merchant_id` from stored rows.

`occurrence` is the count of *identical* tuples already seen **earlier in the same file** (0 for the
first), which is what makes two genuinely identical coffees both import while a re-upload of the same
file produces the same fingerprints and inserts nothing. It is computed per import batch, deterministically,
and covered by a fuzz test.

## 8. Importing a CSV, carefully

The flow, and the reason for each step:

| Step | What happens | Why it exists |
|---|---|---|
| 1. Upload | File goes to `POST /api/imports` (multipart), held in memory under a size cap | Nothing written yet |
| 2. Identify | `sha256(file)` checked against `imports.file_hash` — an exact re-upload is refused immediately with "already imported on <date>" | The cheapest possible dedupe |
| 3. Map | Pick the mapping whose header signature matches (`/data/mappings.json` overrides embedded defaults); if none matches, fail with the actual headers shown. Then the user picks the **account** | A new bank export should be a config entry, not a code change; shape and account are separate facts |
| 4. Parse | `encoding/csv` with `FieldsPerRecord = -1`; every rejected row is counted and shown | A malformed row must not kill the file, and must not vanish silently |
| 5. Normalise | Trim, collapse whitespace, strip leading `POS DEBIT`, `CHECKCARD`, `ACH DEBIT`, trailing `#1234`-style store numbers and card suffixes | Same merchant, one name |
| 6. Fingerprint | As above, with the per-batch occurrence counter | Idempotency |
| 7. Preview | New rows, duplicates, rejected rows, date range, money in, money out, and the first few rows rendered | The point where a wrong sign convention is caught |
| 8. Commit | One SQLite transaction: insert `imports`, then `transactions`; any error rolls the whole file back | Half an import is worse than none |
| 9. Report | Row counts, plus a link into the affected months | So the result is checkable, not just trusted |

`POST /api/imports/preview` and `POST /api/imports` are separate endpoints on purpose: the preview is
a pure function of the file and the mapping, and returns the same structure the commit will act on.

## 9. The screens

| Screen | Contents |
|---|---|
| **Home / This month** | In, out, net. The overdue banner if applicable. Top 8 merchants with amounts and counts. Per-account balances (as of the last statement) and totals. Previous/next month. |
| **Month** | A single month in full: every transaction grouped by merchant or by day (toggle), month-over-month comparison for the same merchants. |
| **Transactions** | Filter by account, month, merchant, text; edit note, rename merchant, merge merchant. Amounts and dates are read-only. |
| **Import** | File picker, then preview table, then confirm. Rejected rows listed with the reason. |
| **Settings** | Accounts, mappings, overdue threshold, backup status/last mirror, app version, update button. |

Design rules, inherited from the rest of the homelab: **read on a phone, import on a laptop** (bank
CSVs are a desktop download, so Import is designed laptop-first and merely works on a phone; every
other screen is phone-first), no emojis, plain-language labels, amounts right-aligned in a
tabular-figure font, and colour used only for state (overdue, transfer, a rise or fall against last month) — never decoratively. Type is **Lexend** for words and **IBM Plex Mono** for every amount (decided 2026-10-31: Lexend has no tabular figures, and a monospaced face keeps every column of money aligned). Both are bundled.

## 10. API

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | `{"status":"ok","version":"r…"}` and nothing else |
| GET | `/api/status` | version, last import age, last backup age, counts — the Meridian vital reads this |
| GET | `/api/accounts` | accounts with per-account totals for the current month |
| GET | `/api/summary?month=YYYY-MM` | in/out/net, merchant rollup, account split |
| GET | `/api/transactions?month=&account=&merchant=&q=&limit=&offset=` | filtered ledger |
| PATCH | `/api/transactions/{id}` | note, merchant assignment (never amount/date) |
| GET | `/api/merchants` · PATCH `/api/merchants/{id}` | list, rename/merge |
| POST | `/api/imports/preview` | parse + dedupe, write nothing |
| POST | `/api/imports` | commit one file |
| GET | `/api/imports` | history: file, range, counts, when, undone |
| POST | `/api/imports/{id}/undo` | delete that import's rows in one transaction; record stays, marked undone |
| GET · POST · PATCH | `/api/accounts` · `/api/accounts/{id}` | create, rename, archive |
| GET | `/api/mappings` | which mappings are active and whether each is embedded or overridden |
| POST | `/api/transactions/{id}/transfer` | mark/unmark a transfer, optionally with a pair |
| GET | `/api/settings` · PATCH | overdue threshold, display preferences |
| GET | `/api/backup/status` | last snapshot, last mirror, size |
| GET | `/api/export.csv?from=&to=` | the ledger back out, as CSV |
| GET | `/api/export.json` | everything, for the app to be irrelevant in ten years |

Every response is JSON with explicit integer minor units and ISO dates. No floats anywhere in the
wire format.

## 11. Where it runs

| Fact | Value |
|---|---|
| URL | `https://lepotato.taild95077.ts.net:8894` (tailnet only) |
| Port | `8894` — **verified free** on the potato 2026-10-31 (in use: 22 53 80 443 5335 8080 8081 8418 8766 8888 8890 8891 8892 8899 11434) |
| Container | Compose project at `~/Coffer`, image pinned by digest in `.env`, publishes `127.0.0.1:8894` only, non-root, read-only rootfs, `/data` as the only writable mount |
| Data | `~/Coffer/data/coffer.db` (+ WAL), `~/Coffer/data/exports/` |
| Secrets | None required in v1. The R2 mirror uses the box's existing `rclone` config; if that ever becomes a key in the container, it goes in `.env` |
| Health | `GET /healthz` → only `status` and `version`, so it is safe to leave unauthenticated on the tailnet |
| Updates | Host watcher + `coffer-update.path` / `.service`, digest pinned, health check, rollback |
| Meridian | Tile with probe `http://localhost:8894/healthz`, vital = **days since last import** (from `/api/status`), and a control target only once it has a watcher |
| Health check | `maintenance-setup/service-healthcheck.sh` gains `check_http "Coffer" "http://localhost:8894/healthz"` in the same change that deploys it — never before |

## 12. Backups and export

Two different promises, both kept:

- **Backups** protect against disk loss: nightly `zstd` snapshot of `coffer.db` (via SQLite's online
  backup API, not `cp` — WAL), kept locally for 30 days, then mirrored to R2 to R2 through the existing
  `r2:` remote, unencrypted by decision (§3 question 8). Verified with a real restore, not a design.
- **Export** protects against *me* losing interest in the code: `GET /api/export.json` and
  `/api/export.csv` produce a complete, documented, plain-text dump. If Coffer vanished tomorrow, the
  data would still be readable, and this is tested by a round-trip (export → wipe a scratch database →
  import → diff).

Financial data is the most sensitive thing on this box. It never goes to a third party, never to a
model provider, and reaches R2 only as a plain mirror into your own private bucket, by choice.

## 13. Phases

Each phase ends with "Done when". Stop and ask at every **[HUMAN]** item.

### Phase 0: Look at real files (blocking, no code)

- **[HUMAN]** answer open questions 1, 2, 12 and 13, and hand over the extra statements from question 2.
- **Spike the PDF library** (question 12) in a throwaway Go program: extract positioned text from both
  real statements and rebuild rows. No server code yet.
- Write the layout description for each institution against the real files: Capital One (year from the
  billing period, sign by section, totals to check) and Delta (type line plus detail line, trailing-minus
  debits, running balance, sub-accounts, rows split across pages).
- Decide nothing else. This phase exists so the parser is designed from evidence.

**Done when:** both real statements reconcile in the throwaway program (every row's running balance
or the printed totals agree), the PDF library is chosen and approved, and the layout descriptions are in
the repo as **synthetic** fixtures (a real statement is never committed).

### Phase 1: The ledger that cannot be double-imported

- Go service skeleton, SQLite + migrations, `/healthz`, embedded empty SvelteKit shell.
- `Source` interface + the two PDF statement parsers (Capital One, Delta Community) with their
  reconciliation checks. A CSV source is optional and deferred: no current bank needs it.
- Fingerprint + occurrence + the dedupe path, with fuzz and golden tests **first**.
- Preview and commit endpoints; a minimal Import screen that can be driven from a phone.

**Done when:** the same file imported twice inserts nothing the second time; two identical same-day
purchases both import; an overlapping date range imports only the new rows; a malformed row is rejected
by name, not silently dropped.

### Phase 2: The month, readable

- Month view, home view, merchant rollup, month-over-month comparison.
- The overdue-import banner.
- Transactions list with filters, note editing, and merchant rename/merge.
- Transfer flag, suggested-pair hint, and exclusion from the month totals; Undo import.

**Done when:** a month of real transactions reads correctly on a phone, the numbers match the bank's
own totals to the cent, and the banner appears when an import is genuinely overdue.

### Phase 3: Fill it in and protect it

- **[HUMAN]** backfill a year of history (open question 7).
- Nightly snapshot + R2 mirror + restore exercised for real.
- JSON/CSV export with the round-trip test.
- `coffer verify`: files present, database integrity, unreferenced rows, currency sanity.

**Done when:** a year of data is in, a restore has actually been performed from R2, and export/import
round-trips clean.

### Phase 4: Make it part of the house

- Dockerfile, compose file, GHCR multi-arch publish, digest pinning, host update watcher, rollback test.
- `tailscale serve` entry, Meridian tile, health-check entry, README/docs in the homelab repo.
- Optional: monthly reminder email through Resend.

**Done when:** it is on the potato, in Meridian, in the health check, and the README matches what
`docker ps` says.

### Phase 5: Later, only if wanted

Candidates in likely order: category rules over merchants, budgets
(envelopes per month), a balance-snapshot field, then — maybe — an OFX or Teller `Source` behind the
same interface. Plaid stays a design possibility, not a plan.

## 14. Costs

| Item | Cost |
|---|---|
| Bank data | $0 — CSV exports are free and included with the accounts |
| Aggregator | $0 in v1, and none planned |
| R2 mirror | effectively $0 (a compressed database of a few MB a year) |
| Electricity | the potato is already running |
| Development | the only real cost, which is why v1 is CSV and not an OAuth application |

## 15. Rules that are not negotiable

1. **No bank credentials.** Not in the app, not in config, not in a log. The only inputs are files I
   hand it.
2. **Nothing leaves the box.** No aggregator, no analytics, no model provider, no telemetry, no
   outside fonts or scripts. Financial data is mirrored only to my own private R2 bucket, as a plain copy by choice (no crypt layer).
3. **Imports are idempotent, always.** A duplicate insert is a data-corruption bug, not a quirk.
4. **Raw rows are kept.** Any derived value must be recomputable from what was stored.
5. **Money is integers.** Minor units end to end, parsed from text, never a float.
6. **The bank's numbers are the bank's.** Dates and amounts are read-only; my notes and merchant names
   are mine.
7. **Nothing is written before the preview is confirmed.**
8. **Export outlives the app.** If the code vanished, the JSON dump stays readable.
9. **Tailnet-only.** No Funnel, no public port, no accounts.

## 16. Working agreement

- One phase at a time; read the matching section first; commit and push per phase.
- **Stop and ask at every [HUMAN] item** — especially Phase 0's real files.
- Keep scope tight: no new dependency, provider or service without asking.
- Small readable functions; the importer and money parsing are pure and tested hard, because they are
  where silent corruption lives.
- No telemetry, no third-party scripts, fonts or icons.
- Plain-language docs for a reader new to the vocabulary; explain terms where they appear.
- Findings and uncertainties go in `NOTES.md` as they are learned.
- No emojis anywhere: UI, docs, commit messages.
