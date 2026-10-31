// Package privacy scans a source tree for things that must never be committed: a PDF that is not
// stamped as synthetic, and a long run of digits that looks like an account or card number.
//
// It reports where a problem is and which rule fired, never what matched, because CI logs are not a
// safe place to repeat an account number.
package privacy

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// RulePDFNotSynthetic flags a PDF without the synthetic producer stamp.
	RulePDFNotSynthetic = "pdf-not-synthetic"
	// RuleLongDigits flags a run of ten or more digits in a text file.
	RuleLongDigits = "long-digit-run"

	// MaxFileBytes is the largest file Scan will read. Bigger files are skipped.
	MaxFileBytes = 2 << 20

	syntheticStamp = "coffer-synthetic"
	allowMarker    = "privacy:allow"
)

// \b keeps hex strings and UUIDs out: a digit run touching a letter is not a number.
var longDigits = regexp.MustCompile(`\b[0-9]{10,}\b`)

var skipDirs = map[string]bool{".git": true, "node_modules": true, ".svelte-kit": true, ".vite": true}

var textExt = map[string]bool{
	".md": true, ".go": true, ".txt": true, ".json": true, ".html": true, ".css": true, ".svg": true,
	".yml": true, ".yaml": true, ".toml": true, ".sql": true, ".csv": true, ".sh": true, // go.mod and go.sum are not scanned: pseudo-versions carry 14-digit timestamps
}

var textNames = map[string]bool{"Makefile": true, "Dockerfile": true, ".gitignore": true, ".dockerignore": true}

// Finding is one problem. Rule says what kind; there is deliberately no field for the matched text.
type Finding struct {
	Path string
	Line int // 0 when the finding is about the whole file
	Rule string
}

func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Rule)
	}
	return fmt.Sprintf("%s: %s", f.Path, f.Rule)
}

// Scan walks root and returns every finding in lexical path order. A missing or unreadable root is
// an error: "could not look" must never read as "nothing found". Symlinks are not followed.
func Scan(root string) ([]Finding, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // symlinks, sockets, devices
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxFileBytes {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ext := strings.ToLower(filepath.Ext(path))

		switch {
		case ext == ".pdf":
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Contains(data, []byte(syntheticStamp)) {
				out = append(out, Finding{Path: rel, Rule: RulePDFNotSynthetic})
			}
		case textExt[ext] || textNames[d.Name()]:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.IndexByte(data, 0) >= 0 {
				return nil // binary content under a text name
			}
			for i, line := range bytes.Split(data, []byte("\n")) {
				if bytes.Contains(line, []byte(allowMarker)) {
					continue
				}
				if longDigits.Match(line) {
					out = append(out, Finding{Path: rel, Line: i + 1, Rule: RuleLongDigits})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
