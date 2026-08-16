// Package tswriter is a minimal Go source writer with indentation tracking,
// mirroring the style of the codeWriter used by the TS plugins (and the
// DeclarationBlock writer in go-gqlcodegen).
package tswriter

import (
	"fmt"
	"strings"
)

// Writer accumulates Go source with indent-aware line writing.
type Writer struct {
	b     strings.Builder
	ind   int
	atBOL bool
}

// New returns an empty writer.
func New() *Writer {
	return &Writer{atBOL: true}
}

// Indent increases the indentation level.
func (w *Writer) Indent() { w.ind++ }

// Dedent decreases the indentation level.
func (w *Writer) Dedent() {
	if w.ind > 0 {
		w.ind--
	}
}

// Line writes one line of code, honoring the current indent.
func (w *Writer) Line(s string) {
	if w.atBOL {
		w.b.WriteString(strings.Repeat("\t", w.ind))
	}
	w.b.WriteString(s)
	w.b.WriteString("\n")
	w.atBOL = true
}

// Raw writes a string without newline handling (for multi-line literals
// already formatted).
func (w *Writer) Raw(s string) {
	w.b.WriteString(s)
	w.atBOL = false
}

// Blank writes an empty line.
func (w *Writer) Blank() {
	w.b.WriteString("\n")
	w.atBOL = true
}

// String returns the accumulated source.
func (w *Writer) String() string { return w.b.String() }

// Block runs fn with one extra level of indentation.
func (w *Writer) Block(fn func()) {
	w.Indent()
	fn()
	w.Dedent()
}

// Linef formats and writes one line.
func (w *Writer) Linef(format string, args ...any) {
	w.Line(fmt.Sprintf(format, args...))
}
