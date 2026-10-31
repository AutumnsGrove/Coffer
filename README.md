# Coffer

A private, self-hosted finance tracker for one person: drop in a monthly bank export, and get an
honest picture of the month — what came in, what went out, and which merchants it actually went to.
Kept on my own machine, backed up encrypted, exportable as plain CSV.

**Status: planned.** No code yet. `PLAN.md` is the source of truth for what gets built and why;
`NOTES.md` records what was verified and what is still unknown.

## Why

- **Monthly, not daily.** Every budgeting app is built for daily engagement, which is their metric, not
  mine. One deliberate session a month shows the *shape* of spending; daily shows noise.
- **CSV, not an aggregator.** Plaid, SimpleFIN and Teller all solve onboarding strangers at scale, with
  OAuth redirects, subscriptions, or a copy-pasted token. Two accounts behind a tailnet need none of
  that: a CSV costs nothing, stores no credentials, has no terms-of-service exposure, and a month of
  lag is fine.
- **The real failure mode is forgetting.** So the app knows when an import is overdue and says so,
  rather than depending on discipline.
- **Merchant insight beats categories.** "$340 at Publix this month" is most of the value of a category
  taxonomy for a fraction of the work, and it comes from data already on hand.
- **Mine, and durable.** SQLite on the potato, an encrypted mirror to R2, and a JSON/CSV export that
  would still be readable if this codebase vanished.

## What it will do

```
 monthly CSV export  →  mapping → normalise → fingerprint → preview → confirm
                                                                       │
                              SQLite ledger (raw rows kept forever) ◄───┘
                                     │
              month view · merchant rollup · overdue banner · export
                                     │
                        nightly snapshot → encrypted R2 mirror
```

Every import is **idempotent**: re-uploading the same month, or overlapping ranges, inserts nothing.
Two identical $4.75 coffees on the same day both import. Nothing is written until the preview is
confirmed.

## Read this first

- [`PLAN.md`](PLAN.md) — the build plan: stack, data model, import design, phases, decisions
- `PLAN.md` §3 — **open questions**, including the one that blocks everything: real CSV files from both
  institutions

## Stack, in one line

Go (stdlib HTTP, SQLite via `modernc.org/sqlite`, pure Go) with a SvelteKit frontend embedded into the
single binary, running in Docker on the potato behind `tailscale serve` on port 8894 — the same shape
as Polaris, bwmon, Warbler and Meridian.

## Licence

MIT — same as the rest of the homelab.
