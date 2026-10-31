// Command covgate fails the build when a package's test coverage is below its floor.
//
//	covgate <thresholds-file> <go-test-cover-output-file>
//
// Exit codes: 0 all packages meet their floor, 1 at least one does not, 2 it could not check
// (bad arguments, unreadable or empty input). "Could not check" is never a pass.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/AutumnsGrove/Coffer/internal/covgate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: covgate <thresholds-file> <go-test-output-file>")
		return 2
	}
	thText, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "covgate:", err)
		return 2
	}
	th, err := covgate.LoadThresholds(string(thText))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	out, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(stderr, "covgate:", err)
		return 2
	}
	pkgs, err := covgate.Parse(string(out))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if fails := covgate.Check(pkgs, th); len(fails) > 0 {
		for _, f := range fails {
			fmt.Fprintln(stderr, "coverage gate:", f)
		}
		return 1
	}
	fmt.Fprintf(stdout, "coverage gate: %d package(s) meet their floors\n", len(pkgs))
	return 0
}
