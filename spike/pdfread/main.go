// Phase 0 spike: can ledongthuc/pdf recover rows from the two real statements?
// Throwaway. Usage: pdfread dump <file.pdf> <page> <from> <to>
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ledongthuc/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pdfread dump <file> <page> <fromRow> <toRow>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "dump":
		dump(os.Args[2], atoi(os.Args[3]), atoi(os.Args[4]), atoi(os.Args[5]))
	case "runs":
		runs(os.Args[2], atoi(os.Args[3]), atoi(os.Args[4]))
	case "check":
		check(os.Args[2], os.Args[3])
	case "lines":
		lines(os.Args[2], atoi(os.Args[3]), atoi(os.Args[4]), atoi(os.Args[5]))
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

// dump prints rows from..to of one page as "y | x:text x:text ...".
func dump(path string, page, from, to int) {
	f, r, err := pdf.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Printf("pages: %d\n", r.NumPage())
	rows, err := r.Page(page).GetTextByRow()
	if err != nil {
		panic(err)
	}
	fmt.Printf("rows on page %d: %d\n", page, len(rows))
	for i, row := range rows {
		if i < from || i > to {
			continue
		}
		fmt.Printf("%3d y=%d |", i, row.Position)
		for _, t := range row.Content {
			fmt.Printf(" %.0f:%q", t.X, t.S)
		}
		fmt.Println()
	}
}

// runs prints the first n non-empty text runs of a page with their raw coordinates.
func runs(path string, page, n int) {
	f, r, err := pdf.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	texts := r.Page(page).Content().Text
	fmt.Printf("runs on page %d: %d\n", page, len(texts))
	shown := 0
	for _, t := range texts {
		if t.S == "" || shown >= n {
			continue
		}
		s := t.S
		if len(s) > 28 {
			s = s[:28]
		}
		fmt.Printf("x=%6.1f y=%6.1f w=%5.1f sz=%4.1f %q\n", t.X, t.Y, t.W, t.FontSize, s)
		shown++
	}
}

// lines prints visual lines from..to of one page, rebuilt by pageSegments, each capped at 130 chars.
func lines(path string, page, from, to int) {
	f, r, err := pdf.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	ls := groupLines(pageSegments(r.Page(page)), 1.5)
	fmt.Printf("page %d: %d lines\n", page, len(ls))
	for i, l := range ls {
		if i < from || i > to {
			continue
		}
		t := l.Text()
		if len(t) > 130 {
			t = t[:130]
		}
		fmt.Printf("%3d y=%6.1f | %s\n", i, l.Y, t)
	}
}
