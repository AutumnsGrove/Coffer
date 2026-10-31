package main

import (
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Seg is one shown string (a Tj or TJ) with the page position where it starts.
type Seg struct {
	X, Y float64
	Size float64
	Font string
	Text string
}

// mat is a PDF affine matrix [a b c d e f].
type mat [6]float64

var ident = mat{1, 0, 0, 1, 0, 0}

func mul(a, b mat) mat {
	return mat{
		a[0]*b[0] + a[1]*b[2], a[0]*b[1] + a[1]*b[3],
		a[2]*b[0] + a[3]*b[2], a[2]*b[1] + a[3]*b[3],
		a[4]*b[0] + a[5]*b[2] + b[4], a[4]*b[1] + a[5]*b[3] + b[5],
	}
}

func translate(tx, ty float64) mat { return mat{1, 0, 0, 1, tx, ty} }

// pageSegments walks a page's content streams with the library's own interpreter and
// tracks just enough text state (cm, Tm, Td, T*, TL, Tf) to know where every string starts.
// ledongthuc/pdf's Content() loses this for some producers (Delta's PDFlib output reports
// every glyph at one point), so the geometry is rebuilt here from the operators.
func pageSegments(p pdf.Page) []Seg {
	var (
		out        []Seg
		ctm        = ident
		saved      []mat
		tm, tlm    = ident, ident
		leading    float64
		size       float64
		enc        pdf.TextEncoding
		font       pdf.Font
		fontByName = map[string]pdf.Font{}
	)

	setFont := func(name string, sz float64) {
		size = sz
		f, ok := fontByName[name]
		if !ok {
			f = p.Font(name)
			fontByName[name] = f
		}
		font = f
		enc = font.Encoder()
	}
	move := func(tx, ty float64) {
		tlm = mul(translate(tx, ty), tlm)
		tm = tlm
	}
	decode := func(raw string) string {
		if enc == nil {
			return raw
		}
		return enc.Decode(raw)
	}
	advance := func(raw string) {
		var w float64
		for i := 0; i < len(raw); i++ {
			cw := font.Width(int(raw[i]))
			if cw == 0 {
				cw = 500
			}
			w += cw / 1000 * size
		}
		tm = mul(translate(w, 0), tm)
	}
	show := func(raw string) {
		if raw == "" {
			return
		}
		pos := mul(tm, ctm)
		out = append(out, Seg{X: pos[4], Y: pos[5], Size: size, Font: font.BaseFont(), Text: decode(raw)})
		advance(raw)
	}

	do := func(stk *pdf.Stack, op string) {
		defer func() {
			for stk.Len() > 0 { // never let unhandled operands leak into the next operator
				stk.Pop()
			}
		}()
		switch op {
		case "q":
			saved = append(saved, ctm)
		case "Q":
			if n := len(saved); n > 0 {
				ctm, saved = saved[n-1], saved[:n-1]
			}
		case "cm":
			var m mat
			for i := 5; i >= 0; i-- {
				m[i] = stk.Pop().Float64()
			}
			ctm = mul(m, ctm)
		case "BT":
			tm, tlm = ident, ident
		case "Tf":
			sz := stk.Pop().Float64()
			setFont(stk.Pop().Name(), sz)
		case "TL":
			leading = stk.Pop().Float64()
		case "Td":
			ty := stk.Pop().Float64()
			move(stk.Pop().Float64(), ty)
		case "TD":
			ty := stk.Pop().Float64()
			leading = -ty
			move(stk.Pop().Float64(), ty)
		case "Tm":
			var m mat
			for i := 5; i >= 0; i-- {
				m[i] = stk.Pop().Float64()
			}
			tm, tlm = m, m
		case "T*":
			move(0, -leading)
		case "Tj":
			show(stk.Pop().RawString())
		case "'":
			raw := stk.Pop().RawString()
			move(0, -leading)
			show(raw)
		case "TJ":
			arr := stk.Pop()
			var b strings.Builder
			for i := 0; i < arr.Len(); i++ {
				if el := arr.Index(i); el.Kind() == pdf.String {
					b.WriteString(el.RawString())
				}
			}
			show(b.String())
		}
	}

	contents := p.V.Key("Contents")
	if contents.Kind() == pdf.Array {
		for i := 0; i < contents.Len(); i++ {
			Interpret(contents.Index(i), do)
		}
	} else {
		Interpret(contents, do)
	}
	return out
}

// Interpret is a thin alias so the file reads top to bottom.
func Interpret(v pdf.Value, do func(*pdf.Stack, string)) { pdf.Interpret(v, do) }

// Line is one visual line: segments that share a baseline, left to right.
type Line struct {
	Y    float64
	Segs []Seg
}

// Text joins the segments, padding the gap between them in proportion to the x distance, so a
// monospaced layout keeps its columns and a proportional one keeps at least a visible gap.
func (l Line) Text() string {
	var b strings.Builder
	col := 0
	for _, s := range l.Segs {
		charW := s.Size * 0.5
		if charW <= 0 {
			charW = 5
		}
		target := int(s.X / charW)
		for col < target {
			b.WriteByte(' ')
			col++
		}
		b.WriteString(s.Text)
		col += len([]rune(s.Text))
	}
	return strings.TrimRight(b.String(), " ")
}

// groupLines clusters segments whose Y values are within tol, top of the page first.
func groupLines(segs []Seg, tol float64) []Line {
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].Y > segs[j].Y })
	var lines []Line
	for _, s := range segs {
		if n := len(lines); n > 0 && lines[n-1].Y-s.Y <= tol {
			lines[n-1].Segs = append(lines[n-1].Segs, s)
			continue
		}
		lines = append(lines, Line{Y: s.Y, Segs: []Seg{s}})
	}
	for i := range lines {
		sort.SliceStable(lines[i].Segs, func(a, b int) bool { return lines[i].Segs[a].X < lines[i].Segs[b].X })
	}
	return lines
}
