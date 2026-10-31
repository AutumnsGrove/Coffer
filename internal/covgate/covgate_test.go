package covgate

import (
	"math"
	"strings"
	"testing"
)

// covgate enforces the coverage floors from DEVELOPMENT.md section 8. It reads the output of
// `go test -cover ./...` and a thresholds file, and says which packages fall short.

const mod = "github.com/AutumnsGrove/Coffer/"

func line(pkg string, pct string) string {
	return "ok  \t" + mod + pkg + "\t0.5s\tcoverage: " + pct + "% of statements\n"
}

func TestParse(t *testing.T) {
	t.Run("happy/reads package and percentage", func(t *testing.T) {
		pk, err := Parse(line("internal/money", "97.5"))
		if err != nil || len(pk) != 1 || pk[0].Pkg != "internal/money" || pk[0].Pct != 97.5 {
			t.Fatalf("Parse = %+v, %v", pk, err)
		}
	})
	t.Run("happy/package with no test files is recorded as such", func(t *testing.T) {
		pk, err := Parse("?   \t" + mod + "internal/x\t[no test files]\n" + line("internal/y", "90.0"))
		if err != nil || len(pk) != 2 || !pk[0].NoTests || pk[1].NoTests {
			t.Fatalf("Parse = %+v, %v", pk, err)
		}
	})
	t.Run("happy/a package with no statements is skipped", func(t *testing.T) {
		pk, err := Parse("ok  \t" + mod + "internal/z\t0.1s\tcoverage: [no statements]\n" + line("internal/y", "90.0"))
		if err != nil || len(pk) != 1 {
			t.Fatalf("Parse = %+v, %v", pk, err)
		}
	})
	t.Run("sad/no coverage lines at all is an error, not a pass", func(t *testing.T) {
		if _, err := Parse("FAIL\tsomething\nbuild failed\n"); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("bad/empty input", func(t *testing.T) {
		if _, err := Parse(""); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("bad/impossible percentages are rejected", func(t *testing.T) {
		for _, pct := range []string{"101.0", "NaN", "-5.0", "1e9"} {
			if pk, err := Parse(line("internal/x", pct)); err == nil && len(pk) > 0 {
				t.Errorf("accepted %q: %+v", pct, pk)
			}
		}
	})
	t.Run("bad/hostile package text does not break parsing", func(t *testing.T) {
		in := "ok  \t" + mod + "internal/x\x1b[31m\t0.5s\tcoverage: 90.0% of statements\n"
		if _, err := Parse(in); err != nil {
			t.Fatalf("Parse returned %v", err)
		}
	})
}

func TestLoadThresholds(t *testing.T) {
	t.Run("happy/default and overrides, comments and blanks ignored", func(t *testing.T) {
		th, err := LoadThresholds("# floors\n\ndefault 85\ninternal/money 95\n")
		if err != nil || th.Default != 85 || th.Min("internal/money") != 95 {
			t.Fatalf("th = %+v, %v", th, err)
		}
	})
	t.Run("sad/missing default is an error", func(t *testing.T) {
		if _, err := LoadThresholds("internal/money 95\n"); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("bad/malformed lines are rejected", func(t *testing.T) {
		for _, in := range []string{"default eighty\n", "default 85\nonlyonefield\n", "default 85\nx 150\n", "default 85\nx -1\n", "default 85\nx 90\nx 91\n", ""} {
			if _, err := LoadThresholds(in); err == nil {
				t.Errorf("accepted %q", in)
			}
		}
	})
}

func TestMin(t *testing.T) {
	th, _ := LoadThresholds("default 85\ninternal/parse 95\ninternal/parse/delta 97\n")
	cases := []struct {
		name, pkg string
		want      float64
	}{
		{"happy/default", "internal/other", 85},
		{"happy/exact key", "internal/parse", 95},
		{"happy/key matches sub-packages", "internal/parse/capone", 95},
		{"happy/longest key wins", "internal/parse/delta", 97},
		{"sad/a key is a path prefix, not a substring", "internal/parser", 85},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := th.Min(c.pkg); got != c.want {
				t.Errorf("Min(%q) = %v, want %v", c.pkg, got, c.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	th, _ := LoadThresholds("default 85\ninternal/money 95\n")
	t.Run("happy/everything meets its floor", func(t *testing.T) {
		pk, _ := Parse(line("internal/money", "95.0") + line("internal/other", "85.0"))
		if fails := Check(pk, th); len(fails) != 0 {
			t.Fatalf("fails = %v", fails)
		}
	})
	t.Run("sad/below the floor names the package and both numbers", func(t *testing.T) {
		pk, _ := Parse(line("internal/money", "94.9"))
		fails := Check(pk, th)
		if len(fails) != 1 || !strings.Contains(fails[0], "internal/money") || !strings.Contains(fails[0], "94.9") || !strings.Contains(fails[0], "95") {
			t.Fatalf("fails = %v", fails)
		}
	})
	t.Run("sad/a package with no tests fails", func(t *testing.T) {
		pk, _ := Parse("?   \t" + mod + "internal/x\t[no test files]\n" + line("internal/y", "90.0"))
		if fails := Check(pk, th); len(fails) != 1 || !strings.Contains(fails[0], "internal/x") {
			t.Fatalf("fails = %v", fails)
		}
	})
	t.Run("bad/NaN never passes", func(t *testing.T) {
		if fails := Check([]Pkg{{Pkg: "internal/x", Pct: math.NaN()}}, th); len(fails) != 1 {
			t.Fatalf("fails = %v", fails)
		}
	})
}
