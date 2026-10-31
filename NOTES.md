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
| `rclone` remotes | `r2:` only — **no crypt remote yet**, so the encrypted backup target in the plan has to be created |
| Resend (monthly reminder option) | Already configured for the health check, `maintenance-setup/secrets.json` |
| Nearby precedent | Warbler (`~/Warbler`) is the closest template: same stack, same compose shape, same update watcher |

### Unknown until real files arrive (blocking Phase 0)

- Capital One's credit-card export and bank-account export are different shapes; Delta Community is a
  third. **The date format, amount column and sign convention of each are unknown.**
- Whether either institution exports more than ~12 months of history in one file.
- Whether pending transactions appear in the export at all (Plaid's summary says Capital One does not
  expose pending transactions to SimpleFIN; for CSV it depends on whether the row is posted).

### Open items carried in PLAN.md §3

Accounts and their names, real CSVs, sign conventions, currency, balances, transfers, backfill,
backup encryption, merchant merging, the overdue interval, and whether any agent belongs anywhere near
this. Each has a recommendation.
