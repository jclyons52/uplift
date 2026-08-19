package deps

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// stdlibHints maps a package name to the Go equivalent (stdlib, or a
// first-party jclyons52 port, or "wrap") that decides whether a port repo is
// needed. This is curated knowledge, not a mechanical rule — exactly the
// "too similar to something Go could do" judgement the tool helps surface.
var stdlibHints = map[string]string{
	// trivial / single-purpose — absorb, don't create a repo
	"escape-string-regexp": "tiny (regexp.QuoteMeta ≈) — absorb",
	"strip-ansi":           "small (rune/ANSI strip) — absorb or one-file lib",
	"imurmurhash":          "tiny hash — absorb",
	"is-glob":              "small matcher — absorb or one-file lib",
	"is-path-inside":       "small path check (filepath) — absorb",
	"graphemer":            "small — absorb or one-file lib",
	"glob-parent":          "small — absorb",
	"cross-spawn":          "wrap os/exec — absorb as helper",
	"find-up":              "small (filepath walk) — absorb",

	// node builtins — Go stdlib
	"debug": "port as leaf lib (env-gated logging) or absorb as helper",

	// leaf libraries worth their own repo (the port strategy)
	"chalk":                     "port as leaf lib (ANSI styling) — own repo",
	"text-table":                "port as leaf lib (align) — own repo, or text/tabwriter",
	"fast-deep-equal":           "tiny (reflect.DeepEqual-ish) — absorb",
	"esutils":                   "port as leaf lib (JS util) — own repo",
	"esquery":                   "port as leaf lib (selector engine) — own repo",
	"esprima":                   "port as leaf lib (JS parser) — own repo",
	"espree":                    "port as leaf lib (JS parser) — own repo",
	"eslint-scope":              "port as leaf lib (scope analysis) — own repo",
	"eslint-visitor-keys":       "tiny map — absorb",
	"@eslint-community/regexpp": "port as leaf lib (regex parser) — own repo",
	"ajv":                       "port as large lib (JSON-schema) — own repo, big",
	"js-yaml":                   "use gopkg.in/yaml.v3 — wrap, don't port",
	"lodash":                    "port or use existing Go lib; large",
}

// leafSmallLOC is the threshold (lines) below which an external leaf is
// judged "too small to split" (the leftPad class).
const leafSmallLOC = 40

// Render writes a human-friendly dependency report to a string.
func (r *Result) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Module graph for %s\n\n", r.Root)

	var internal []*Node
	var builtin []*Node
	var external []*Node
	for _, name := range r.Order {
		n := r.Nodes[name]
		switch n.Kind {
		case KindInternal:
			internal = append(internal, n)
		case KindBuiltin:
			builtin = append(builtin, n)
		default:
			external = append(external, n)
		}
	}
	sortNodes(external)

	var extLeaves, extRoots int
	for _, n := range external {
		if r.IsLeaf(n.Name) {
			extLeaves++
		}
		if !r.HasExternalChildren(n.Name) && len(n.Requirers) == 0 {
			// unreferenced transitive leaf
		}
		if len(n.Requirers) == 0 {
			extRoots++
		}
	}

	fmt.Fprintf(&b, "Internal files: %d\nBuiltins: %d\nExternal packages: %d (%d leaves)\n",
		len(internal), len(builtin), len(external), extLeaves)

	fmt.Fprintf(&b, "\n=== EXTERNAL DEPENDENCIES ===\n")
	fmt.Fprintf(&b, "%-28s %-7s %-6s %-7s %-5s %-10s %-28s %s\n", "DEP", "KIND", "LEAF", "LOC", "EXP", "VERSION", "NEEDED-BY", "RECOMMEND")
	b.WriteString(strings.Repeat("-", 132) + "\n")
	for _, n := range external {
		leaf := r.IsLeaf(n.Name)
		leafMark := "yes"
		if !leaf {
			leafMark = "no"
		}
		needed := shortRequirers(n)
		rec := r.Recommend(n.Name)
		if r.Stale(n.Name) {
			rec = fmt.Sprintf("%s ↯ stale: v%s → latest v%s", rec, n.Version, n.Latest)
		}
		ver := n.Version
		if ver == "" {
			ver = "?"
		}
		exp := len(n.Exports)
		fmt.Fprintf(&b, "%-28s %-7s %-6s %-7d %-5d %-10s %-28s %s\n",
			n.Name, n.Kind, leafMark, n.Loc, exp, ver, needed, rec)
	}

	// node builtins actually used
	fmt.Fprintf(&b, "\n=== NODE BUILTINS USED ===\n")
	for _, n := range builtin {
		fmt.Fprintf(&b, "  %-24s needed by %s\n", n.Name, shortRequirers(n))
	}

	fmt.Fprintf(&b, "\n=== INTERNAL COUPLING (%d files) ===\n", len(internal))
	fmt.Fprintf(&b, "%-46s %-8s %s\n", "FILE", "LOC", "IMPORTS")
	for _, n := range internal {
		fmt.Fprintf(&b, "  %-44s %-8d %s\n", n.Name, n.Loc, shortList(n.Requires, 8))
	}
	return b.String()
}

func shortRequirers(n *Node) string {
	switch len(n.Requirers) {
	case 0:
		return "(transitive)"
	case 1:
		return n.Requirers[0]
	default:
		return fmt.Sprintf("%s +%d", n.Requirers[0], len(n.Requirers)-1)
	}
}

func shortList(items []string, max int) string {
	if len(items) == 0 {
		return "—"
	}
	if len(items) > max {
		return strings.Join(items[:max], ", ") + fmt.Sprintf(" …(+%d)", len(items)-max)
	}
	return strings.Join(items, ", ")
}

func sortNodes(ns []*Node) {
	sort.Slice(ns, func(i, j int) bool {
		// leaves first, then by name
		li, lj := ns[i].Loc, ns[j].Loc
		if li != lj {
			return li < lj
		}
		return ns[i].Name < ns[j].Name
	})
}

// WriteJSON serializes the graph for downstream tooling.
func (r *Result) WriteJSON(w io.Writer) error {
	type row struct {
		Name      string   `json:"name"`
		Kind      string   `json:"kind"`
		Loc       int      `json:"loc"`
		Exports   []string `json:"exports,omitempty"`
		Requirers []string `json:"neededBy"`
		Requires  []string `json:"requires"`
		Leaf      bool     `json:"leaf"`
		Version   string   `json:"version,omitempty"`
		Latest    string   `json:"latest,omitempty"`
		Stale     bool     `json:"stale"`
		Recommend string   `json:"recommend"`
	}
	var rows []row
	for _, name := range r.Order {
		n := r.Nodes[name]
		rows = append(rows, row{
			Name:      n.Name,
			Kind:      n.Kind.String(),
			Loc:       n.Loc,
			Exports:   n.Exports,
			Requirers: n.Requirers,
			Requires:  n.Requires,
			Leaf:      n.Kind == KindExternal && r.IsLeaf(name) || n.Kind == KindBuiltin,
			Version:   n.Version,
			Latest:    n.Latest,
			Stale:     r.Stale(name),
			Recommend: r.Recommend(name),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}
