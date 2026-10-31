package main

import (
	"fmt"
	"os"
)

func sum(ts []Txn) (n int64) {
	for _, t := range ts {
		n += t.Amount
	}
	return
}

func verdict(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

// check prints reconciliation results only: counts, totals and pass/fail, never row contents.
func check(kind, path string) {
	switch kind {
	case "delta":
		res, err := parseDelta(path)
		if err != nil {
			panic(err)
		}
		fmt.Printf("period %s to %s, %d sub-accounts, lines lost inside a ledger: %d, trailer lines ignored: %d\n", res.PeriodFrom, res.PeriodTo, len(res.Accounts), res.Unattached, res.Trailer)
		for _, u := range res.UnattachedText {
			fmt.Printf("  unattached: %q\n", u)
		}
		allOK := len(res.Accounts) > 0
		for _, a := range res.Accounts {
			last := a.Opening
			if n := len(a.Rows); n > 0 {
				last = a.Rows[n-1].Balance
			}
			endOK := a.HasEnding && last == a.Ending
			chainOK := len(a.ChainFailures) == 0
			var in, out int64
			for _, t := range a.Rows {
				if t.Amount > 0 {
					in += t.Amount
				} else {
					out += t.Amount
				}
			}
			fmt.Printf("  ID %s %-14s rows=%d in=%s out=%s opening=%s ending(printed)=%s hasEnding=%v\n",
				a.ID, a.Name, len(a.Rows), fmtCents(in), fmtCents(out), fmtCents(a.Opening), fmtCents(a.Ending), a.HasEnding)
			fmt.Printf("    running-balance chain: %s (%d failures)   last row balance == printed ending: %s   rows without detail line: %d\n",
				verdict(chainOK), len(a.ChainFailures), verdict(endOK), a.NoDetail)
			if !chainOK {
				fmt.Printf("    first failing row index: %d\n", a.ChainFailures[0])
			}
			allOK = allOK && chainOK && (endOK || len(a.Rows) == 0)
		}
		fmt.Printf("DELTA STATEMENT: %s\n", verdict(allOK && res.Unattached == 0))
	case "capone":
		res, err := parseCapOne(path)
		if err != nil {
			panic(err)
		}
		fmt.Printf("period %s to %s, purchases=%d credits=%d unmatched lines=%d\n", res.PeriodFrom, res.PeriodTo, len(res.Purchases), len(res.Credits), res.Unmatched)
		fmt.Printf("  summary as printed: %v\n", res.Summary)
		s := res.Summary
		purch := -sum(res.Purchases)
		cred := sum(res.Credits)
		c1 := purch == s["transactions"]
		c2 := cred == s["payments"]+s["credits"]
		c3 := s["previous"]-s["payments"]-s["credits"]+s["transactions"]+s["fees"]+s["interest"] == s["new"]
		c4 := res.PrintedTotals["transactions"] == purch
		fmt.Printf("  rows sum to summary Transactions (%s vs %s): %s\n", fmtCents(purch), fmtCents(s["transactions"]), verdict(c1))
		fmt.Printf("  credit rows sum to Payments + Other Credits (%s vs %s): %s\n", fmtCents(cred), fmtCents(s["payments"]+s["credits"]), verdict(c2))
		fmt.Printf("  previous - credits + purchases + fees + interest == new balance: %s\n", verdict(c3))
		fmt.Printf("  rows sum to printed 'Total Transactions for This Period': %s\n", verdict(c4))
		fmt.Printf("CAPITAL ONE STATEMENT: %s\n", verdict(c1 && c2 && c3 && c4 && res.Unmatched == 0))
	default:
		fmt.Fprintln(os.Stderr, "check delta|capone <file>")
	}
}
