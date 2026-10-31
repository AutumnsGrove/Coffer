package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Txn is one ledger row as read from a statement. Amount is integer cents, negative = money out.
type Txn struct {
	Posted  string // YYYY-MM-DD
	Amount  int64
	Desc    string // type text plus detail lines, as printed
	Balance int64  // running balance after this row (Delta only)
	HasBal  bool
}

// SubAccount is one balance-forward block inside a statement (Delta prints several per file).
type SubAccount struct {
	ID, Name        string
	Opening, Ending int64
	HasEnding       bool
	Rows            []Txn
	ChainFailures   []int // row indexes where previous balance + amount != printed balance
	NoDetail        int   // rows with no detail line
}

type DeltaResult struct {
	PeriodFrom, PeriodTo string
	Accounts             []*SubAccount
	Unattached           int // table lines that matched nothing and had nowhere to attach
	UnattachedText       []string
	Trailer              int // lines after a block's Ending Balance (totals, fee table, promotions)
}

var (
	reDeltaPeriod = regexp.MustCompile(`(\d{2})/(\d{2})/(\d{2})\s+(\d{2})/(\d{2})/(\d{2})`)
	reBalFwd      = regexp.MustCompile(`^(\d{2})/(\d{2})\s+ID\s+(\d{4})\s+(.+?)\s+Balance Forward\s+([\d,]+\.\d{2}-?)$`)
	reEnding      = regexp.MustCompile(`^(?:\d{2}/\d{2}\s+)?Ending Balance\s+([\d,]+\.\d{2}-?)$`)
	reDeltaRow    = regexp.MustCompile(`^(\d{2})/(\d{2})\s+(?:(\d{2})/(\d{2})\s+)?(\S.*?)\s+([\d,]+\.\d{2}-?)\s+([\d,]+\.\d{2}-?)$`)
)

// parseDelta reads a Delta Community statement. Courier lines below the column-header row are the
// ledger; each row is a header line (dates, type, amount, balance) plus one or more detail lines,
// and a detail line can land at the top of the next page.
func parseDelta(path string) (*DeltaResult, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	res := &DeltaResult{}
	var cur *SubAccount
	var last *Txn
	var prev int64
	year := ""
	var yTop float64

	for pg := 1; pg <= r.NumPage(); pg++ {
		var courier []Seg
		for _, s := range pageSegments(r.Page(pg)) {
			if strings.Contains(s.Font, "Courier") {
				courier = append(courier, s)
			}
		}
		// The column headings and the address box are a background image, so only the ledger and the
		// page header block are text. Page 1 starts its ledger with a "Balance Forward" line, and every
		// page's ledger starts at that same height, so that line's Y separates header from table.
		if pg == 1 {
			for _, l := range groupLines(append([]Seg(nil), courier...), 1.5) {
				if reBalFwd.MatchString(strings.TrimSpace(l.Text())) {
					yTop = l.Y + 1
					break
				}
			}
			if yTop == 0 {
				return nil, fmt.Errorf("no Balance Forward line on page 1")
			}
		}
		var table []Seg
		for _, s := range courier {
			if s.Y <= yTop {
				table = append(table, s)
			} else if res.PeriodFrom == "" {
				if m := reDeltaPeriod.FindStringSubmatch(s.Text); m != nil {
					year = "20" + m[3]
					res.PeriodFrom = fmt.Sprintf("20%s-%s-%s", m[3], m[1], m[2])
					res.PeriodTo = fmt.Sprintf("20%s-%s-%s", m[6], m[4], m[5])
				}
			}
		}

		for _, l := range groupLines(table, 1.5) {
			t := strings.TrimSpace(l.Text())
			switch {
			case t == "" || strings.Contains(t, "Continued on following page"):
			case reBalFwd.MatchString(t):
				m := reBalFwd.FindStringSubmatch(t)
				bal, err := cents(m[5])
				if err != nil {
					return nil, err
				}
				cur = &SubAccount{ID: m[3], Name: m[4], Opening: bal}
				res.Accounts = append(res.Accounts, cur)
				prev, last = bal, nil
			case reEnding.MatchString(t) && cur != nil:
				bal, _ := cents(reEnding.FindStringSubmatch(t)[1])
				cur.Ending, cur.HasEnding = bal, true
				last = nil
			case reDeltaRow.MatchString(t) && cur != nil:
				m := reDeltaRow.FindStringSubmatch(t)
				amt, err1 := cents(m[6])
				bal, err2 := cents(m[7])
				if err1 != nil || err2 != nil {
					res.Unattached++
					continue
				}
				if os.Getenv("COFFER_SPIKE_CORRUPT") == "1" && cur.ID == "0010" && len(cur.Rows) == 10 {
					amt = -amt // simulate a misread sign on one row
				}
				cur.Rows = append(cur.Rows, Txn{
					Posted:  fmt.Sprintf("%s-%s-%s", year, m[1], m[2]),
					Amount:  amt,
					Desc:    m[5],
					Balance: bal, HasBal: true,
				})
				idx := len(cur.Rows) - 1
				if prev+amt != bal {
					cur.ChainFailures = append(cur.ChainFailures, idx)
				}
				prev = bal
				last = &cur.Rows[idx]
			default:
				if last != nil {
					last.Desc += " | " + t
				} else if cur != nil && cur.HasEnding {
					res.Trailer++
				} else {
					res.Unattached++
					res.UnattachedText = append(res.UnattachedText, t)
				}
			}
		}
	}
	for _, a := range res.Accounts {
		for _, t := range a.Rows {
			if !strings.Contains(t.Desc, " | ") {
				a.NoDetail++
			}
		}
	}
	return res, nil
}
