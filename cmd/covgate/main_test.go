package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func files(t *testing.T, thresholds, output string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	th, out := filepath.Join(dir, "th.txt"), filepath.Join(dir, "out.txt")
	if err := os.WriteFile(th, []byte(thresholds), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(output), 0o644); err != nil {
		t.Fatal(err)
	}
	return th, out
}

const okLine = "ok  \tgithub.com/AutumnsGrove/Coffer/internal/x\t0.1s\tcoverage: 90.0% of statements\n"

func TestRun(t *testing.T) {
	t.Run("happy/all packages meet the floor", func(t *testing.T) {
		th, out := files(t, "default 85\n", okLine)
		var so, se bytes.Buffer
		if code := run([]string{th, out}, &so, &se); code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, se.String())
		}
	})
	t.Run("sad/a package below the floor exits 1 and says which", func(t *testing.T) {
		th, out := files(t, "default 95\n", okLine)
		var so, se bytes.Buffer
		code := run([]string{th, out}, &so, &se)
		if code != 1 || !strings.Contains(se.String(), "internal/x") {
			t.Fatalf("code = %d, stderr = %q", code, se.String())
		}
	})
	t.Run("bad/wrong number of arguments", func(t *testing.T) {
		var so, se bytes.Buffer
		if code := run([]string{"only-one"}, &so, &se); code != 2 {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("bad/missing files exit 2, never 0", func(t *testing.T) {
		var so, se bytes.Buffer
		if code := run([]string{"/no/such", "/no/such/either"}, &so, &se); code != 2 {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("bad/unparseable test output exits 2", func(t *testing.T) {
		th, out := files(t, "default 85\n", "build failed\n")
		var so, se bytes.Buffer
		if code := run([]string{th, out}, &so, &se); code != 2 {
			t.Fatalf("code = %d", code)
		}
	})
}
