package main

import (
	"fmt"
	"strings"
)

// cents parses "$1,234.56", "1234.56-", "-$5.00" or "(5.00)" into integer cents. It insists on exactly
// two decimals, so a misread never rounds silently (PLAN.md section 6).
func cents(s string) (int64, error) {
	t := strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg, t = true, t[1:len(t)-1]
	}
	if strings.HasSuffix(t, "-") {
		neg, t = true, strings.TrimSuffix(t, "-")
	}
	if strings.HasPrefix(t, "-") {
		neg, t = !neg, strings.TrimPrefix(t, "-")
	}
	t = strings.NewReplacer("$", "", ",", "", " ", "").Replace(t)
	dot := strings.IndexByte(t, '.')
	if dot < 0 || len(t)-dot-1 != 2 {
		return 0, fmt.Errorf("not a two-decimal amount: %q", s)
	}
	var n int64
	for _, c := range t[:dot] + t[dot+1:] {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

func fmtCents(n int64) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}
