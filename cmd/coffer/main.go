// Command coffer is the Coffer server. Slice 0 only knows how to report its version; the server
// itself arrives in later slices (DEVELOPMENT.md section 9).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/AutumnsGrove/Coffer/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main with its dependencies passed in, so tests drive it with plain buffers. It returns
// the process exit code. Arguments are never echoed back: they are untrusted input.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Fprintf(stdout, "coffer %s\n", version.String())
		return 0
	}
	fmt.Fprintln(stderr, "usage: coffer --version")
	return 2
}
