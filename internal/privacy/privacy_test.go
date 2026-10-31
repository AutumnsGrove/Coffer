package privacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The privacy scanner is the project's guard against committing real financial data
// (DEVELOPMENT.md rule 6). It looks for two things: PDFs that are not stamped as synthetic, and
// long digit runs that look like account or card numbers in text files. It reports where, and
// never what: a finding must not repeat the number it matched.

// digits returns n identical digits, built at run time so this file holds no digit string for the
// scanner to flag (and no number that could be mistaken for a real one).
func digits(n int) string { return strings.Repeat("7", n) }

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rules(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}

func TestScan(t *testing.T) {
	t.Run("happy/clean tree has no findings", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "README.md", "Coffer tracks a month. Port 8894, version r142.7024ab0.\n")
		write(t, root, "internal/x/x.go", "package x\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("happy/stamped synthetic pdf is allowed", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "testdata/ok.pdf", "%PDF-1.4\n/Producer (coffer-synthetic)\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("happy/go pseudo-version timestamps are not account numbers", func(t *testing.T) {
		root := t.TempDir()
		// a real pseudo-version carries a 14-digit UTC timestamp: v0.0.0-<yyyymmddhhmmss>-<sha>
		write(t, root, "go.mod", "require example.com/x v0.0.0-"+digits(14)+"-abcdef012345\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("happy/allow marker on the same line", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "notes.md", "build "+digits(13)+" privacy:allow\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("sad/unstamped pdf is flagged", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "testdata/real.pdf", "%PDF-1.4\n/Producer (PDFlib)\n")
		fs, _ := Scan(root)
		if got := rules(fs); len(got) != 1 || got[0] != RulePDFNotSynthetic {
			t.Fatalf("rules = %v, want [%s]", got, RulePDFNotSynthetic)
		}
	})

	t.Run("sad/account-length digit run is flagged with its line", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "docs/a.md", "fine\nacct "+digits(10)+" here\n")
		fs, _ := Scan(root)
		if len(fs) != 1 || fs[0].Rule != RuleLongDigits || fs[0].Line != 2 {
			t.Fatalf("findings = %+v, want one %s on line 2", fs, RuleLongDigits)
		}
	})

	t.Run("sad/boundary: nine digits pass, ten fail", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "nine.md", digits(9)+"\n")
		write(t, root, "ten.md", digits(10)+"\n")
		fs, _ := Scan(root)
		if len(fs) != 1 || filepath.Base(fs[0].Path) != "ten.md" {
			t.Fatalf("findings = %+v, want only ten.md", fs)
		}
	})

	t.Run("sad/digits split by hyphens are not an account number", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "ids.md", "thread 3ec8d2f8-0c0b-4aa9-8510-6dba91357966\n")
		fs, _ := Scan(root)
		if len(fs) != 0 {
			t.Fatalf("findings = %+v, want none", fs)
		}
	})

	t.Run("bad/a finding never repeats the number it matched", func(t *testing.T) {
		root := t.TempDir()
		num := digits(10)
		write(t, root, "a.md", "acct "+num+"\n")
		fs, _ := Scan(root)
		if len(fs) != 1 {
			t.Fatalf("findings = %+v", fs)
		}
		if strings.Contains(fs[0].String(), num) {
			t.Fatalf("finding leaks the matched number: %q", fs[0].String())
		}
	})

	t.Run("bad/binary files are not scanned for digits", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "font.woff2", "wOF2\x00\x01 "+digits(20)+" \x00")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("bad/skipped directories are not entered", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, ".git/config", "id "+digits(20)+"\n")
		write(t, root, "node_modules/x/readme.md", "id "+digits(20)+"\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want no findings", fs, err)
		}
	})

	t.Run("bad/oversized file is skipped, not read into memory", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "big.md", strings.Repeat("x", MaxFileBytes+1)+"\n"+digits(20)+"\n")
		fs, err := Scan(root)
		if err != nil || len(fs) != 0 {
			t.Fatalf("Scan = %v, %v; want the oversized file skipped", fs, err)
		}
	})

	t.Run("bad/symlink loop does not hang", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
			t.Skip("symlinks unavailable")
		}
		write(t, root, "a.md", "ok\n")
		if _, err := Scan(root); err != nil {
			t.Fatalf("Scan returned %v", err)
		}
	})

	t.Run("bad/missing root is an error, not a clean bill of health", func(t *testing.T) {
		if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
			t.Fatal("Scan of a missing directory returned no error")
		}
	})
}

// TestRepoIsClean is the real gate: it scans this repository.
func TestRepoIsClean(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	fs, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		t.Errorf("privacy finding: %s", f)
	}
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
