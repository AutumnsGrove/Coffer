package main

import (
	"bytes"
	"strings"
	"testing"
)

// main() stays a one-liner; all behaviour lives in run so it can be tested with plain buffers.

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // substring
		wantStderr string // substring
	}{
		{"happy/version", []string{"--version"}, 0, "coffer dev", ""},
		{"happy/version short flag", []string{"-version"}, 0, "coffer dev", ""},

		{"sad/no arguments prints usage", nil, 2, "", "usage:"},
		{"sad/empty argument", []string{""}, 2, "", "usage:"},

		{"bad/unknown flag", []string{"--bogus"}, 2, "", "usage:"},
		{"bad/version with extra arguments", []string{"--version", "extra"}, 2, "", "usage:"},
		{"bad/hostile argument is not echoed", []string{"\x1b[31mred\x1b[0m"}, 2, "", "usage:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
			if strings.Contains(stdout.String()+stderr.String(), "\x1b") {
				t.Errorf("output contains a terminal escape sequence")
			}
		})
	}
}
