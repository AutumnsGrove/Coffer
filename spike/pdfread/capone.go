package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

type CapOneResult struct {
	PeriodFrom, PeriodTo string
	Purchases, Credits   []Txn
	Summary              map[string]int64 // as printed on page 1
	PrintedTotals        map[string]int64 // "Total Transactions for This Period" etc.
	Unmatched            int
}

var (
	months     = map[string]string{"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04", "May": "05", "Jun": "06", "Jul": "07", "Aug": "08", "Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12"}
	reCapPer   = regexp.MustCompile(`([A-Z][a-z]{2}) (\d{1,2}), (\d{4}) - ([A-Z][a-z]{2}) (\d{1,2}), (\d{4})`)
	reCapRow   = regexp.MustCompile(`^([A-Z][a-z]{2}) (\d{1,2}) ([A-Z][a-z]{2}) (\d{1,2}) (.+?) (-?\s?\$[\d,]+\.\d{2})$`)
	reCapSecCr = regexp.MustCompile(`#\d{4}: Payments, Credits and Adjustments`)
	reCapSecDb = regexp.MustCompile(`#\d{4}: Transactions$`)
	reCapSum   = map[string]*regexp.Regexp{
		"previous":     regexp.MustCompile(`Previous Balance\s+(-?\$[\d,]+\.\d{2})`),
		"payments":     regexp.MustCompile(`Payments\s+(-?\$[\d,]+\.\d{2})`),
		"credits":      regexp.MustCompile(`Other Credits\s+(-?\$[\d,]+\.\d{2})`),
		"transactions": regexp.MustCompile(`Transactions\s+\+\s*(\$[\d,]+\.\d{2})`),
		"fees":         regexp.MustCompile(`Fees Charged\s+\+\s*(\$[\d,]+\.\d{2})`),
		"interest":     regexp.MustCompile(`Interest Charged\s+\+\s*(\$[\d,]+\.\d{2})`),
		"new":          regexp.MustCompile(`New Balance\s+=\s*(\$[\d,]+\.\d{2})`),
	}
	reCapTotalTx = regexp.MustCompile(`Total Transactions for This Period\s+(\$[\d,]+\.\d{2})`)
)

// parseCapOne reads a Capital One credit card statement. The sign comes from the section a row sits
// in, not from the number: purchases are money out, "Payments, Credits and Adjustments" is money in.
func parseCapOne(path string) (*CapOneResult, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	res := &CapOneResult{Summary: map[string]int64{}, PrintedTotals: map[string]int64{}}
	var startYear, startMonth int
	section := ""

	var all []string
	for pg := 1; pg <= r.NumPage(); pg++ {
		for _, l := range groupLines(pageSegments(r.Page(pg)), 1.5) {
			all = append(all, strings.TrimSpace(l.Text()))
		}
	}

	for _, t := range all {
		if m := reCapPer.FindStringSubmatch(t); m != nil && res.PeriodFrom == "" {
			res.PeriodFrom = fmt.Sprintf("%s-%s-%02s", m[3], months[m[1]], m[2])
			res.PeriodTo = fmt.Sprintf("%s-%s-%02s", m[6], months[m[4]], m[5])
			fmt.Sscanf(m[3], "%d", &startYear)
			fmt.Sscanf(months[m[1]], "%d", &startMonth)
		}
		for k, re := range reCapSum {
			if m := re.FindStringSubmatch(t); m != nil {
				if _, seen := res.Summary[k]; !seen {
					if v, err := cents(m[1]); err == nil {
						res.Summary[k] = v
					}
				}
			}
		}
		if m := reCapTotalTx.FindStringSubmatch(t); m != nil {
			res.PrintedTotals["transactions"], _ = cents(m[1])
		}
		switch {
		case reCapSecCr.MatchString(t):
			section = "credit"
			continue
		case reCapSecDb.MatchString(t):
			section = "debit"
			continue
		case strings.HasPrefix(t, "Total") || strings.Contains(t, ": Total Transactions") || t == "Fees" || t == "Interest Charged":
			section = ""
			continue
		}
		if section == "" {
			continue
		}
		if m := reCapRow.FindStringSubmatch(t); m != nil {
			amt, err := cents(m[6])
			if err != nil {
				res.Unmatched++
				continue
			}
			var mo int
			fmt.Sscanf(months[m[3]], "%d", &mo) // the post date decides the year
			year := startYear
			if mo < startMonth {
				year++
			}
			tx := Txn{Posted: fmt.Sprintf("%d-%s-%02s", year, months[m[3]], m[4]), Desc: m[5]}
			if section == "debit" {
				tx.Amount = -amt
				res.Purchases = append(res.Purchases, tx)
			} else {
				tx.Amount = amt
				res.Credits = append(res.Credits, tx)
			}
		} else if !strings.HasPrefix(t, "Trans Date") && t != "" {
			res.Unmatched++
		}
	}
	return res, nil
}
