// Package scaffold turns the deps analysis into concrete, separate library
// repos. For every external leaf the analysis marks "port as its own repo",
// it lays down a starter repo: go.mod, a compiling package stub, a parity
// test stub, and a README carrying the source/LOC/export metadata — so the
// next step (transpile the original JS via uplift) starts from a real repo
// rather than a blank directory.
package scaffold

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jclyons52/uplift/internal/deps"
)

// Options control scaffolding.
type Options struct {
	// Out is the directory where repos are created.
	Out string
	// ModulePrefix is the Go module prefix, e.g. "github.com/jclyons52".
	ModulePrefix string
	// SourceRoot is the node_modules directory holding the originals.
	SourceRoot string
	// Vendor copies the original node_modules package source into
	// <repo>/original/ as reference for the transpile step.
	Vendor bool
	// Only, when non-empty, restricts scaffolding to these package names.
	Only map[string]bool
}

// Repo is the plan for one separated library.
type Repo struct {
	Name      string   // npm package name (e.g. "chalk", "@humanwhocodes/object-schema")
	Dir       string   // directory name under Out
	Module    string   // full module path
	Package   string   // Go package name
	Loc       int      // original LOC
	Exports   []string // discovered public API
	NeededBy  []string // who requires it
	SourceDir string   // absolute node_modules source (when vendoring)
}

// WantsOwnRepo reports whether the analysis recommends separating a package.
func WantsOwnRepo(r *deps.Result, name string) bool {
	rec := r.Recommend(name)
	return strings.Contains(rec, "own repo") || strings.Contains(rec, "port as leaf")
}

// Plan computes the repos to create for the leaf packages the analysis
// recommends separating (respecting opts.Only).
func Plan(res *deps.Result, opts Options) []Repo {
	var out []Repo
	for _, name := range res.Order {
		n := res.Nodes[name]
		if n == nil || n.Kind != deps.KindExternal {
			continue
		}
		if len(opts.Only) > 0 {
			// explicit selection overrides the recommendation filter
			if !opts.Only[name] {
				continue
			}
		} else if !WantsOwnRepo(res, name) {
			continue
		}
		dir := sanitizeDir(name)
		src := ""
		if opts.SourceRoot != "" {
			src = filepath.Join(opts.SourceRoot, name)
		}
		out = append(out, Repo{
			Name:      name,
			Dir:       dir,
			Module:    strings.TrimSuffix(opts.ModulePrefix, "/") + "/" + dir + "-go",
			Package:   sanitizePkg(name),
			Loc:       n.Loc,
			Exports:   n.Exports,
			NeededBy:  n.Requirers,
			SourceDir: src,
		})
	}
	return out
}

// sanitizeDir makes an npm name a safe directory/module suffix:
// "chalk" -> "chalk", "@humanwhocodes/object-schema" -> "humanwhocodes-object-schema",
// "@nodelib/fs.stat" -> "nodelib-fs-stat". Any run of non-alphanumerics
// becomes a single hyphen, trimmed of leading/trailing hyphens.
func sanitizeDir(name string) string {
	name = strings.TrimPrefix(name, "@")
	var b strings.Builder
	lastDash := true // suppress a leading hyphen
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "pkg"
	}
	return out
}

// sanitizePkg derives a Go package name from an npm name, concatenating all
// identifier characters (so "eslint-visitor-keys" -> "eslintvisitorkeys").
func sanitizePkg(name string) string {
	d := sanitizeDir(name)
	var b strings.Builder
	for _, r := range d {
		if r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	p := b.String()
	if p == "" {
		return "pkg"
	}
	if p[0] >= '0' && p[0] <= '9' {
		p = "_" + p
	}
	if goKeywords[p] {
		p += "_"
	}
	return p
}

// goKeywords is the minimal set a package name must avoid.
var goKeywords = map[string]bool{
	"break": true, "default": true, "func": true, "interface": true,
	"select": true, "case": true, "defer": true, "go": true, "map": true,
	"struct": true, "chan": true, "else": true, "goto": true, "package": true,
	"switch": true, "const": true, "fallthrough": true, "if": true, "range": true,
	"type": true, "continue": true, "for": true, "import": true, "return": true,
	"var": true,
}

// Describe renders the repo metadata block (README body).
func (r Repo) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Name)
	fmt.Fprintf(&b, "Go port of the npm package `%s`, created by the uplift port toolchain.\n\n", r.Name)
	fmt.Fprintf(&b, "| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Original size | %d LOC |\n", r.Loc)
	if len(r.Exports) > 0 {
		fmt.Fprintf(&b, "| Public API | `%s` |\n", strings.Join(r.Exports, "`, `"))
	} else {
		fmt.Fprintf(&b, "| Public API | (entry is a function/other — see `original/`) |\n")
	}
	fmt.Fprintf(&b, "| Needed by | %s |\n", strings.Join(r.NeededBy, ", "))
	fmt.Fprintf(&b, "| Module | `%s` |\n\n", r.Module)
	b.WriteString("Workflow: transpile `original/` with `uplift`, then hand-clean. Keep the parity test green.\n")
	return b.String()
}
