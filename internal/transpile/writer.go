package transpile

import (
	"strings"
)

// goWriter is a minimal Go source writer with indentation, mirroring the
// DeclarationBlock writer used in go-gqlcodegen.
type goWriter struct {
	b     strings.Builder
	ind   int
	atBOL bool
}

func newGoWriter() *goWriter { return &goWriter{atBOL: true} }

// line writes one line with the current indentation.
func (w *goWriter) line(s string) {
	if w.atBOL {
		w.b.WriteString(strings.Repeat("\t", w.ind))
	}
	w.b.WriteString(s)
	w.b.WriteString("\n")
	w.atBOL = true
}

func (w *goWriter) blank() {
	w.b.WriteString("\n")
	w.atBOL = true
}

// raw appends pre-formatted (already indented) text.
func (w *goWriter) raw(s string) {
	w.b.WriteString(s)
	w.atBOL = true
}

func (w *goWriter) indent() { w.ind++ }
func (w *goWriter) dedent() {
	if w.ind > 0 {
		w.ind--
	}
}
func (w *goWriter) String() string { return w.b.String() }

func (w *goWriter) block(open string, fn func()) {
	w.line(open + " {")
	w.indent()
	fn()
	w.dedent()
	w.line("}")
}
