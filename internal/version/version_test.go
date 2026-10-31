package version

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The version string ends up in /healthz, in logs and in the update watcher's comparison, so it is
// treated as untrusted text: anything that is not exactly "dev" or "r<count>.<sha>" is refused.

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// happy
		{"happy/unstamped build", "dev", true},
		{"happy/count and short sha", "r142.7024ab0", true},
		{"happy/single digit count", "r1.abcdef0", true},
		{"happy/full 40 char sha", "r7." + strings.Repeat("a", 40), true},

		// sad: plausible but wrong
		{"sad/empty", "", false},
		{"sad/no count", "r.abcdef0", false},
		{"sad/no sha", "r142.", false},
		{"sad/uppercase r", "R142.7024ab0", false},
		{"sad/uppercase sha", "r142.7024AB0", false},
		{"sad/sha too short", "r142.abc", false},
		{"sad/leading space", " r142.7024ab0", false},
		{"sad/trailing newline", "r142.7024ab0\n", false},

		// bad: malformed or hostile
		{"bad/markup", "r1.<script>", false},
		{"bad/header injection", "r1.abcdef0\r\nSet-Cookie: x=1", false},
		{"bad/nul byte", "r1.abcdef0\x00", false},
		{"bad/non-ascii digit", "r١.abcdef0", false},
		{"bad/count overflow", "r" + strings.Repeat("9", 20) + ".abcdef0", false},
		{"bad/very long", "r1." + strings.Repeat("a", 10000), false},
		{"bad/invalid utf8", "r1.\xff\xfe\xfd\xfc\xfb\xfa\xf9", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Valid(tc.in); got != tc.want {
				t.Errorf("Valid(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	t.Run("happy/unstamped build reports dev", func(t *testing.T) {
		if got := String(); got != "dev" {
			t.Errorf("String() = %q, want %q", got, "dev")
		}
	})

	t.Run("happy/stamped build reports the stamp", func(t *testing.T) {
		old := stamp
		t.Cleanup(func() { stamp = old })
		stamp = "r142.7024ab0"
		if got := String(); got != "r142.7024ab0" {
			t.Errorf("String() = %q, want the stamp", got)
		}
	})

	t.Run("bad/an invalid stamp is never echoed", func(t *testing.T) {
		old := stamp
		t.Cleanup(func() { stamp = old })
		stamp = "r1.<script>alert(1)</script>"
		if got := String(); got != "dev" {
			t.Errorf("String() = %q, want fallback %q", got, "dev")
		}
	})
}

// FuzzValid asserts a property, not just "does not panic": whatever Valid accepts is short,
// printable ASCII made only of [a-z0-9.], so it is safe in JSON, headers and file names.
func FuzzValid(f *testing.F) {
	for _, seed := range []string{"dev", "r142.7024ab0", "", "r1.<script>", "r1.abcdef0\r\n", "R1.abcdef0", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !Valid(s) {
			return
		}
		if len(s) > 64 || !utf8.ValidString(s) {
			t.Fatalf("accepted an unsafe string of length %d", len(s))
		}
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.') {
				t.Fatalf("accepted a string with %q in it: %q", r, s)
			}
		}
	})
}
