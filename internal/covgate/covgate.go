// Package covgate enforces the per-package coverage floors from DEVELOPMENT.md section 8.
//
// It reads the text printed by `go test -cover ./...` and a thresholds file, and reports every
// package that falls short. It fails closed: no coverage lines, a package with no tests, or a
// number that is not a number all count as failures, never as passes.
package covgate

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const modPrefix = "github.com/AutumnsGrove/Coffer/"

var (
	reCovered  = regexp.MustCompile(`^ok\s+(\S+)\s+\S+\s+coverage: ([0-9]+(?:\.[0-9]+)?)% of statements`)
	reNoTests  = regexp.MustCompile(`^\?\s+(\S+)\s+\[no test files\]`)
	errNoLines = errors.New("covgate: no coverage lines found in the test output (did the tests build?)")
)

// Pkg is one package's result. Pkg is relative to the module, e.g. "internal/money".
type Pkg struct {
	Pkg     string
	Pct     float64
	NoTests bool
}

// Parse reads `go test -cover` output. Packages with no statements are skipped; packages with no
// test files are kept so Check can fail them.
func Parse(out string) ([]Pkg, error) {
	var pkgs []Pkg
	for _, ln := range strings.Split(out, "\n") {
		if m := reNoTests.FindStringSubmatch(ln); m != nil {
			pkgs = append(pkgs, Pkg{Pkg: strings.TrimPrefix(m[1], modPrefix), NoTests: true})
			continue
		}
		m := reCovered.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		pct, err := strconv.ParseFloat(m[2], 64)
		if err != nil || pct > 100 {
			return nil, fmt.Errorf("covgate: impossible coverage %q for %s", m[2], m[1])
		}
		pkgs = append(pkgs, Pkg{Pkg: strings.TrimPrefix(m[1], modPrefix), Pct: pct})
	}
	if len(pkgs) == 0 {
		return nil, errNoLines
	}
	return pkgs, nil
}

// Thresholds are the minimum percentages: one default and optional per-path overrides.
type Thresholds struct {
	Default float64
	byKey   map[string]float64
}

// LoadThresholds parses lines of "<path-or-default> <percent>". Blank lines and # comments are
// ignored. A default is required and a key may appear only once.
func LoadThresholds(text string) (Thresholds, error) {
	th := Thresholds{byKey: map[string]float64{}}
	seen := map[string]bool{}
	for i, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) != 2 {
			return th, fmt.Errorf("covgate: thresholds line %d: want \"<path> <percent>\"", i+1)
		}
		v, err := strconv.ParseFloat(f[1], 64)
		if err != nil || math.IsNaN(v) || v < 0 || v > 100 {
			return th, fmt.Errorf("covgate: thresholds line %d: %q is not a percentage", i+1, f[1])
		}
		if seen[f[0]] {
			return th, fmt.Errorf("covgate: thresholds line %d: %q appears twice", i+1, f[0])
		}
		seen[f[0]] = true
		if f[0] == "default" {
			th.Default = v
		} else {
			th.byKey[f[0]] = v
		}
	}
	if !seen["default"] {
		return th, errors.New("covgate: thresholds must contain a \"default\" line")
	}
	return th, nil
}

// Min returns the floor for pkg: the longest key that is pkg itself or a path prefix of it,
// otherwise the default. "internal/parse" covers "internal/parse/delta" but not "internal/parser".
func (t Thresholds) Min(pkg string) float64 {
	best, bestLen := t.Default, -1
	for k, v := range t.byKey {
		if (pkg == k || strings.HasPrefix(pkg, k+"/")) && len(k) > bestLen {
			best, bestLen = v, len(k)
		}
	}
	return best
}

// Check returns one message per package that is below its floor or has no tests.
func Check(pkgs []Pkg, th Thresholds) []string {
	var fails []string
	for _, p := range pkgs {
		min := th.Min(p.Pkg)
		switch {
		case p.NoTests:
			fails = append(fails, fmt.Sprintf("%s: no test files", p.Pkg))
		case !(p.Pct >= min): // written so NaN fails
			fails = append(fails, fmt.Sprintf("%s: coverage %.1f%% is below the %.0f%% floor", p.Pkg, p.Pct, min))
		}
	}
	return fails
}
