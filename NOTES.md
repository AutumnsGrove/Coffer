# Notes

Findings and open items in the order they are learned, each with a date. Uncertain things go here,
then get promoted into `PLAN.md` once decided.

## 2026-10-31 — Planning start

### Where the research came from

Polaris thread `3ec8d2f8-0c0b-4aa9-8510-6dba91357966`, "Plaid API for Personal Finance Apps"
(2026-10-31, deepseek). Exported with:

```bash
curl -s "http://localhost:8899/api/threads/3ec8d2f8-0c0b-4aa9-8510-6dba91357966/export?format=md&scope=full&sources=true" -o thread.md
```

The export path and its gotcha (no `polaris` binary on `$PATH` inside the container) are documented in
the homelab repo at `docs/polaris.md`.

### What the research established

| Option | Cost for 2 accounts | Auth shape | Verdict |
|---|---|---|---|
| **CSV export** | $0 | Manual download | **Chosen for v1.** Free, no credentials, no ToS exposure, tolerates a month of lag |
| Plaid Trial | $0 (10 Items) | OAuth `redirect_uri` needing real HTTPS | Gold standard for a company; wrong for two personal accounts. Removing an Item does **not** free a slot |
| SimpleFIN Bridge | $15/yr | Copy-paste setup token, no redirect at all | The most Tailscale-native option if automation is ever wanted. 24 requests/day cap. Capital One confirmed (no pending transactions); Delta Community **unverified** |
| Teller | $0 (100 connections, dev tier) | Teller Connect widget + redirect URI | Free and capable, still needs the redirect |
| OFX Direct Connect | $0 | Direct request with credentials | Genuinely bespoke and free, but a Quicken report says Delta Community's endpoint has been failing since 2026-04-03 (`OL-297-A`) |

Two corrections the research made worth keeping:

- **An account + routing number cannot read transactions.** That is ACH, a money-*movement* rail, not a
  data API. The "apps that just read my bank info" (Upside) are using Plaid underneath.
- **Plaid's `redirect_uri` is a browser redirect**, not a server callback — so a Tailscale `*.ts.net`
  HTTPS URL would technically satisfy it. The real blockers are the Production application and
  per-Item billing, not network topology.

### Design conclusions taken from the thread

- Put the source behind a `Protocol`/interface from day one (~15 lines); CSV, OFX and Plaid become
  sibling implementations instead of a rewrite.
- **Dedupe is the hard part.** CSV has no stable IDs and months overlap, so:
  `fingerprint = sha256(account|posted|amount|normalised description|occurrence)`, with `occurrence`
  counting identical tuples earlier *in the same file*. That makes imports idempotent.
- Keep the raw row forever, and log every import (filename, hash, counts, date) so "did I already do
  this month?" is a query.
- Skip categories in v1; roll up by normalised merchant.
- Ship the "last import: N days ago" amber banner. It converts a willpower problem into a visible
  state, and it is the feature most likely to decide whether the app survives.

### Verified on the potato (2026-10-31)

| Check | Result |
|---|---|
| Port 8894 free | Yes. In use: 22 53 80 443 5335 8080 8081 8418 8766 8888 8890 8891 8892 8899 11434 |
| Go on the box | 1.26.5 linux/arm64 |
| Node on the box | v20.20.2 (build-time only; the binary embeds the build) |
| Docker | 29.6.1 with Compose v5.3 |
| `rclone` remotes | `r2:` only — **no crypt remote**, and by decision (2026-10-31) none will be created: the backup is a plain mirror |
| Resend (monthly reminder option) | Already configured for the health check, `maintenance-setup/secrets.json` |
| Nearby precedent | Warbler (`~/Warbler`) is the closest template: same stack, same compose shape, same update watcher |

### Unknown until real files arrive (blocking Phase 0)

- Capital One's credit-card export and bank-account export are different shapes; Delta Community is a
  third. **The date format, amount column and sign convention of each are unknown.**
- Whether either institution exports more than ~12 months of history in one file.
- Whether pending transactions appear in the export at all (Plaid's summary says Capital One does not
  expose pending transactions to SimpleFIN; for CSV it depends on whether the row is posted).

## 2026-10-31 — Real files arrived, and they are PDFs

No CSV could be downloaded from either institution; the two files in hand are monthly **statement
PDFs**. Both have a real text layer (not scans) and `pdftotext -layout` extracts them on macOS and
Linux. Structure only is recorded here; the statements themselves stay out of the repo (`*.pdf` is
now ignored) and parser fixtures must be synthetic.

### Capital One credit card statement (4 pages, clean table)

- Columns: `Trans Date`, `Post Date`, `Description`, `Amount`. **Dates have no year** ("Oct 1"); the
  year comes from the header's billing period ("Sep 27, 2026 - Oct 11, 2026"), and a December/January
  cycle needs year-rollover logic.
- **Sign is carried by the section, not the number.** Purchases are positive under "Transactions";
  payments and credits sit in a separate "Payments, Credits and Adjustments" block. So a purchase is
  money out by being in the first block.
- **Billing cycles are not calendar months** (this one is Sep 27 to Oct 11, a short first cycle).
  Months in the app therefore have to come from posting date, never from the statement.
- The statement states its own checksum: previous balance, payments, transactions, new balance, and a
  "Total Transactions" figure. A parser can prove it read every row.

### Delta Community statement (6 pages, monospaced ledger)

- **Several sub-accounts in one file**: a savings block and a checking block, each starting from
  "Balance Forward" and each with an ID. One PDF is more than one Coffer account.
- Columns: posting date, effective date, description, payment/credit or debit, finance charge, fees,
  transaction amount, **running balance on every row**.
- **Debits are a trailing minus** (`12.60-`); credits are unsigned.
- **A description is two or more lines**: a type line ("Withdrawal Check Card", "Recurring Withdrawal
  Check Card", "Withdrawal POS #...", "Deposit ACH ...", "Deposit Journal Voucher") and a detail line
  holding the real purchase date, a long per-transaction reference number, the merchant, and city and
  state. ACH rows carry `TYPE:`, `CO:` and `NAME:` lines instead.
- **Rows can split across a page break.** A page ends with "--- Continued on following page ---" and
  the detail line of the last row can land at the top of the next page.
- The running balance gives a per-row check (previous balance plus amount equals the next balance),
  which catches a misread sign or a dropped row immediately.
- Many near-identical small card purchases at the same merchant, some on the same day. The long
  reference number in the detail line looks unique per transaction, which could be a stronger dedupe
  key than the occurrence counter, where it is present.

### What this changes

- Statement PDFs are **non-overlapping and self-verifying**, which is better for dedupe and for "the
  numbers match the bank to the cent" than CSV; they are worse for parsing robustness. A bank
  redesigning its PDF breaks the parser, where a CSV header rarely changes.
- The plan's normaliser prefixes (`POS DEBIT`, `CHECKCARD`) do not match what Delta actually prints, and
  a description is a type line plus a detail line, not one string.
- Whether a CSV or QFX download exists in either bank's desktop online banking (not the phone app) is
  **unverified**; worth one more look before committing to PDF parsing.

## 2026-10-31 — Phase 0 spike: both statements reconcile

`spike/pdfread` (throwaway, no statement data in it) reads both real files with `ledongthuc/pdf` and
checks them against what each statement prints about itself. Results, counts only:

| Statement | Rows | Checks | Result |
|---|---|---|---|
| Capital One credit card | 4 purchases, 0 credits | rows = summary Transactions; rows = printed period total; previous + purchases + fees + interest = new balance; credits = Payments + Other Credits | all pass |
| Delta, savings block | 0 | opening = printed ending | pass |
| Delta, checking block | 78 | every row: previous balance + amount = printed balance; last row = printed Ending Balance; opening + in − out = ending | all pass, 0 failures |

A negative test misreads one sign on purpose: the chain check fails and names the row, and every row
after it still passes (the chain restarts from the printed balance), so one bad row points at itself.

What the spike learned that the plan did not know:

- **The library's own text APIs fail on Delta.** `Content().Text` reports every glyph at one position
  and `GetTextByRow()` returns one row for the page. Delta's pages are a single text block of
  fixed-width Courier lines (each a `Tj` string, columns made of spaces) stepped down with `Td`, plus
  a background **image** holding the headings and address box. The fix is small: walk the content
  stream with the library's public `Interpret`, track `cm`, `Tm`, `Td`, `TL`, `T*`, and read each shown
  string's start point and font. That gives the same lines `pdftotext -layout` does.
- **Capital One needs no workaround** in the library, but the same line-rebuild code serves both, so
  there is one extractor.
- **Delta headings are not text.** The ledger is found by its first `Balance Forward` line, not by
  searching for column titles. Everything after a block's `Ending Balance` is trailer (dividend
  totals, a fee table, a promotion) and is ignored; a line lost *inside* a ledger is a failure.
- **A dropped row breaks the chain**, so the running balance is a stronger check than a row count:
  78 rows passing means none are missing, without knowing the true count.
- The Capital One *payments and credits* block is empty in the only file, so that path is checked
  only against a synthetic fixture. Cycles that cross a year (Dec to Jan) are handled in code but
  untested.

Library risk: a single-maintainer pseudo-version (`v0.0.0-20261020...`). Pin it, and vendor it before
Phase 4.

### Open items carried in PLAN.md §3

Accounts and their names, real CSVs, sign conventions, currency, balances, transfers, backfill,
backup encryption, merchant merging, the overdue interval, and whether any agent belongs anywhere near
this. Each has a recommendation.
