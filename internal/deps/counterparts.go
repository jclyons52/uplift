package deps

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A counterpart maps an npm/TS package to the Go way of getting its
// behaviour without re-porting: either an existing Go module to use, or the
// Go stdlib, or an explicit "port it" verdict when no good equivalent exists.
// This is the seed of a DefinitelyTyped-style shared registry (kept here as
// a JSON data file so it can grow / move to a shared repo unchanged).

// counterJSON embeds the curated npm→Go counterpart registry.
//
//go:embed counterparts.json
var counterJSON []byte

// CounterpartVerdict classifies how to obtain a package in Go.
type CounterpartVerdict string

const (
	UseExisting CounterpartVerdict = "use_existing" // adopt a known Go module
	UseStdlib   CounterpartVerdict = "stdlib"       // Go stdlib covers it with a direct call
	InlineIt    CounterpartVerdict = "inline"       // do NOT split into a repo; inline the canonical shim (see InlineShims)
	PortIt      CounterpartVerdict = "port"         // port it; no good equivalent
	PortPartial CounterpartVerdict = "port_partial" // partial equivalent; port the gap
)

// Counterpart is one npm → Go mapping.
type Counterpart struct {
	Go      string             `json:"go"`
	Verdict CounterpartVerdict `json:"verdict"`
	Note    string             `json:"note"`
}

// counterparts holds the embedded registry.
var counterparts = map[string]Counterpart{}

func init() {
	_ = json.Unmarshal(counterJSON, &counterparts)
}

// LoadCounterparts returns the merged counterpart registry. Overlay entries
// win over the embedded ones (so a user file can extend or fix it).
func LoadCounterparts(overlayJSON []byte) map[string]Counterpart {
	m := make(map[string]Counterpart, len(counterparts)+64)
	for k, v := range counterparts {
		m[k] = v
	}
	if len(overlayJSON) > 0 {
		var ov map[string]Counterpart
		if json.Unmarshal(overlayJSON, &ov) == nil {
			for k, v := range ov {
				m[k] = v
			}
		}
	}
	return m
}

// RecommendCounterpart returns the scaled-back verb for a package, or "" if
// the registry has no opinion.
func RecommendCounterpart(name string, reg map[string]Counterpart) Counterpart {
	c, ok := reg[name]
	if !ok {
		return Counterpart{}
	}
	return c
}

// RenderCounterparts writes a human index of the registry (and which are
// referenced by a given Result, when passed).
func RenderCounterparts(reg map[string]Counterpart, referenced map[string]bool) string {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-36s %-9s %-30s %s\n", "NPM PACKAGE", "VERDICT", "GO COUNTERPART", "NOTE"))
	b.WriteString(strings.Repeat("-", 120) + "\n")
	for _, n := range names {
		c := reg[n]
		mark := " " // not referenced by this project
		if referenced != nil && referenced[n] {
			mark = "*"
		}
		goCol := c.Go
		if c.Verdict == UseStdlib {
			goCol = "(stdlib)"
		}
		if c.Verdict == InlineIt {
			goCol = "(inline)"
		}
		if c.Verdict == PortIt || c.Verdict == PortPartial {
			goCol = "(port)"
		}
		fmt.Fprintf(&b, "%-36s %-9s %-30s %s%s\n", n, c.Verdict, goCol, mark, c.Note)
	}
	if referenced != nil {
		b.WriteString("\n* referenced by this codebase\n")
	}
	return b.String()
}
