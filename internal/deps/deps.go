// Package deps analyzes a JavaScript/TypeScript codebase's module graph to
// support the "port each dependency as its own library in a separate repo"
// strategy. For every module it reports:
//
//   - which modules require it (need),
//   - whether it is a leaf node (owns its whole subtree),
//   - its size (LOC, function/export counts),
//   - whether Go's stdlib or a first-party port already covers it, and
//   - a recommendation about whether to split it out or absorb it.
//
// The point is a decision aid, not a substitute for judgement: it turns the
// "should I make this a repo?" question into a table with cheap signals.
package deps

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// Regexes used to catch module specifiers that the AST makes awkward.
var (
	importEqualsRe = regexp.MustCompile(`\bimport\s+[\w$]+\s*=\s*require\(\s*['"]([^'"]+)['"]`)
	fromRe         = regexp.MustCompile(`\bfrom\s+['"]([^'"]+)['"]`)
)

// Kind classifies a module.
type Kind int

const (
	KindInternal Kind = iota // a project file
	KindBuiltin              // a Node.js builtin (node:fs, path, ...)
	KindExternal             // an npm package
)

func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "internal"
	case KindBuiltin:
		return "builtin"
	default:
		return "external"
	}
}

// Node is one module in the graph.
type Node struct {
	Name      string   // canonical name: rel path, "node:fs", or bare package name (with subpath trimmed)
	Path      string   // absolute path (internal only)
	Kind      Kind     `json:"-"`
	Requirers []string // modules that require this one
	Requires  []string // modules this one requires
	Loc       int      // line count (internal: file; external: package .js source excluding node_modules)
	Exports   []string // exported symbol names discovered (external packages, best-effort)
}

// Result is the analyzed graph keyed by canonical node name.
type Result struct {
	Root        string
	Nodes       map[string]*Node
	Order       []string
	StdlibHints map[string]string
	// Counterparts is the npm→Go registry used to inform recommendations.
	Counterparts map[string]Counterpart
}

// AnalyzeOptions controls analysis behaviour.
type AnalyzeOptions struct {
	// Hints override/extends the built-in recommendation map when non-empty.
	// Keyed by package name → recommendation text.
	Hints map[string]string
	// CounterpartOverlay is optional JSON extending/fixing the npm→Go
	// counterpart registry for this run.
	CounterpartOverlay []byte
}

// Recommend returns a one-line decision for an external/builtin node.
func (r *Result) Recommend(name string) string {
	n := r.Nodes[name]
	if n == nil {
		return "unknown"
	}
	// A counterpart in the registry supplies a verdict first (use-existing /
	// stdlib override the mechanical heuristics; port/port_partial fall
	// through to them so size still drives the recommendation).
	if cp, ok := r.Counterparts[name]; ok {
		switch cp.Verdict {
		case UseExisting:
			return "use existing Go: " + cp.Go + " — " + cp.Note
		case UseStdlib:
			return "Go stdlib covers this — " + cp.Note
		}
	}
	if hint, ok := r.StdlibHints[name]; ok {
		return hint
	}
	switch n.Kind {
	case KindBuiltin:
		return "node builtin — use Go stdlib"
	case KindExternal:
	default:
		return "project file"
	}
	leaf := r.IsLeaf(name)
	hasChildren := r.HasExternalChildren(name)
	exports := len(n.Exports)
	switch {
	case !leaf && hasChildren:
		return "split as a repo WITH its subtree (owns children)"
	case leaf && n.Loc < 40:
		return "too small to split (leftPad-class) — absorb/inline; not worth a repo"
	case leaf && exports <= 2 && n.Loc < 300:
		return fmt.Sprintf("small leaf (%d exports, %d LOC) — absorb or one-file lib", exports, n.Loc)
	case leaf && n.Loc < 300:
		return fmt.Sprintf("small leaf library (%d exports) — port as its own repo (candidate)", exports)
	case leaf:
		return "leaf library — port as its own repo"
	default:
		return "port with its transitive deps"
	}
}

// IsLeaf reports whether an external node depends on no other external
// package (only builtins and the project).
func (r *Result) IsLeaf(name string) bool {
	for _, dep := range r.Nodes[name].Requires {
		d := r.Nodes[dep]
		if d != nil && d.Kind == KindExternal {
			return false
		}
	}
	// A node with zero external requires is trivially a leaf.
	return true
}

// HasExternalChildren reports whether a node requires any external package.
func (r *Result) HasExternalChildren(name string) bool {
	for _, dep := range r.Nodes[name].Requires {
		if d := r.Nodes[dep]; d != nil && d.Kind == KindExternal {
			return true
		}
	}
	return false
}

// Dependency indexes -------------------------------------------------------

// FindNodeModules returns the node_modules directory for a base path, walking
// up the tree. Empty if none found.
func FindNodeModules(base string) string { return findNodeModules(base) }

// findNodeModules locates a node_modules dir for a base directory, walking up.
func findNodeModules(base string) string {
	dir, err := filepath.Abs(base)
	if err != nil {
		return ""
	}
	for {
		cand := filepath.Join(dir, "node_modules")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// packageJSON holds the dependency fields we read.
type packageJSON struct {
	Name         string            `json:"name"`
	Main         string            `json:"main"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
	Peer         map[string]string `json:"peerDependencies"`
	Optional     map[string]string `json:"optionalDependencies"`
	Type         string            `json:"type"`
}

// bareName turns a module specifier into the canonical package name,
// trimming subpaths: "chalk/source" -> "chalk", "@scope/pkg/x" -> "@scope/pkg".
func bareName(spec string) string {
	spec = strings.TrimPrefix(spec, "node:")
	if strings.HasPrefix(spec, "@") {
		parts := strings.Split(spec, "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		return spec[:i]
	}
	return spec
}

// builtins is the set of Node.js builtin module names.
var builtins = map[string]bool{
	"assert": true, "buffer": true, "child_process": true, "cluster": true,
	"console": true, "constants": true, "crypto": true, "dgram": true,
	"diagnostics_channel": true, "dns": true, "domain": true, "events": true,
	"fs": true, "http": true, "http2": true, "https": true, "module": true,
	"net": true, "os": true, "path": true, "perf_hooks": true, "process": true,
	"punycode": true, "querystring": true, "readline": true, "repl": true,
	"stream": true, "string_decoder": true, "timers": true, "tls": true,
	"trace_events": true, "tty": true, "url": true, "util": true, "v8": true,
	"vm": true, "wasi": true, "worker_threads": true, "zlib": true,
}

func canonicalName(spec string) (name string, kind Kind) {
	spec = strings.TrimSpace(spec)
	// strip query/hash
	if i := strings.IndexAny(spec, "?#"); i >= 0 {
		spec = spec[:i]
	}
	if strings.HasPrefix(spec, "node:") {
		return "node:" + spec[len("node:"):], KindBuiltin
	}
	if builtins[spec] {
		return "node:" + spec, KindBuiltin
	}
	if !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
		return bareName(spec), KindExternal
	}
	// relative/internal — caller resolves to an absolute path first
	return "", KindInternal
}

// Analyzer walks a set of project files and builds the full graph.
type Analyzer struct {
	root    string
	nm      string           // node_modules dir
	project map[string]*Node // abs path -> node (internal)
	Result  *Result
}

// createSF parses a source file, renaming the virtual path to a .ts/.tsx
// extension when the real file is .js (the TS parser otherwise refuses it).
func createSF(p *tsmorph.Project, absPath string, code []byte) *tsmorph.SourceFile {
	virt := filepath.ToSlash(absPath)
	ext := filepath.Ext(virt)
	if ext != ".ts" && ext != ".tsx" {
		base := filepath.Base(virt)
		virt = filepath.ToSlash(filepath.Join(filepath.Dir(absPath),
			strings.TrimSuffix(base, ext)+".ts"))
	}
	return p.CreateSourceFile(virt, string(code))
}

// analyzeEntryFiles parses each project file and records its edges.
func (a *Analyzer) analyzeEntryFiles() {
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		return
	}
	res := a.Result
	for _, f := range a.projectPaths() {
		code, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		sf := createSF(p, f, code)
		if sf == nil {
			continue
		}
		name := a.relName(f)
		node := res.Nodes[name]
		if node == nil {
			node = &Node{Name: name, Path: f, Kind: KindInternal}
			res.Nodes[name] = node
			res.Order = append(res.Order, name)
		}
		node.Loc = nonEmptyLines(code)
		for _, spec := range extractSpecs(sf) {
			dep := a.resolve(name, string(code), spec)
			if dep == "" {
				continue
			}
			node.Requires = addUnique(node.Requires, dep)
			if d := res.Nodes[dep]; d != nil {
				d.Requirers = addUnique(d.Requirers, name)
			}
		}
	}
}

// analyzeExternal expands each external package's own dependency tree by
// reading its package.json, recording size, and recursing.
func (a *Analyzer) analyzeExternal() {
	// expand until fixpoint (cycle-safe via seen)
	seen := map[string]bool{}
	var expand func(name string)
	expand = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		dir := filepath.Join(a.nm, name)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return
		}
		pj, _ := readPackage(dir)
		res := a.Result
		node := res.Nodes[name]
		if node == nil {
			node = &Node{Name: name, Kind: KindExternal}
			res.Nodes[name] = node
			res.Order = append(res.Order, name)
		}
		node.Loc = sourceLoc(dir)
		node.Exports = discoveredExports(dir, pj)
		var deps []string
		for d := range pj.Dependencies {
			deps = append(deps, d)
		}
		for d := range pj.Peer {
			deps = append(deps, d)
		}
		for d := range pj.Optional {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		for _, d := range deps {
			dname, kind := canonicalName(d)
			if kind == KindBuiltin {
				dname = "node:" + dname
			}
			node.Requires = addUnique(node.Requires, dname)
			if dnode := res.Nodes[dname]; dnode != nil {
				dnode.Requirers = addUnique(dnode.Requirers, name)
			} else {
				res.Nodes[dname] = &Node{Name: dname, Kind: kind, Requirers: []string{name}}
				res.Order = append(res.Order, dname)
			}
			if kind == KindExternal {
				expand(dname)
			}
		}
	}
	// start from every external node already in the graph
	var names []string
	for _, n := range a.Result.Nodes {
		if n.Kind == KindExternal {
			names = append(names, n.Name)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		expand(n)
	}
}

// resolve turns a specifier relative to `fromName` into a canonical dep name.
func (a *Analyzer) resolve(fromName, fromCode, spec string) string {
	name, kind := canonicalName(spec)
	if kind != KindInternal {
		// External package names that don't resolve to an installed package
		// are test fixtures / fake requires (e.g. `qux`, `_`, `{{importSource}}`)
		// buried in example strings — drop them.
		if kind == KindExternal {
			if a.nm == "" || !dirExists(filepath.Join(a.nm, name)) {
				return ""
			}
		}
		return a.ensureNode(name, kind)
	}
	// relative: resolve against the requiring file's directory
	from := a.Result.Nodes[fromName].Path
	base := filepath.Dir(from)
	abs, ok := resolveRelative(base, spec)
	if !ok {
		return ""
	}
	absKey := filepath.ToSlash(abs)
	// link to an existing internal node by path if present
	for _, n := range a.Result.Nodes {
		if n.Path == absKey {
			return n.Name
		}
	}
	inner, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		return ""
	}
	sf := createSF(p, abs, inner)
	if sf == nil {
		return ""
	}
	name = a.relName(abs)
	a.Result.Nodes[name] = &Node{Name: name, Path: filepath.ToSlash(abs), Kind: KindInternal}
	a.Result.Order = append(a.Result.Order, name)
	a.analyzeEntryFile(sf, abs)
	return name
}

// ensureNode returns a node, creating it and expanding external subtrees on
// first mention. For external packages this resolves their own node_modules
// tree (which may add leaves) via analyzeExternal.
func (a *Analyzer) ensureNode(name string, kind Kind) string {
	res := a.Result
	if n, ok := res.Nodes[name]; ok {
		return n.Name
	}
	res.Nodes[name] = &Node{Name: name, Kind: kind}
	res.Order = append(res.Order, name)
	if kind == KindExternal {
		a.analyzeExternal()
	}
	return name
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func (a *Analyzer) analyzeEntryFile(sf *tsmorph.SourceFile, abs string) {
	node := a.Result.Nodes[a.relName(abs)]
	for _, spec := range extractSpecs(sf) {
		dep := a.resolve(node.Name, "", spec)
		if dep == "" {
			continue
		}
		node.Requires = addUnique(node.Requires, dep)
		if d := a.Result.Nodes[dep]; d != nil {
			d.Requirers = addUnique(d.Requirers, node.Name)
		}
	}
}

func (a *Analyzer) projectPaths() []string {
	var out []string
	for _, n := range a.Result.Nodes {
		if n.Kind == KindInternal && n.Path != "" {
			out = append(out, n.Path)
		}
	}
	sort.Strings(out)
	return out
}

// relName makes an absolute path display as relative to the root.
func (a *Analyzer) relName(abs string) string {
	rel, err := filepath.Rel(a.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// Analyze builds the full module graph for a root directory. Custom
// recommendation hints (from a config file) may be supplied via opts.
func Analyze(root string, opts ...AnalyzeOptions) (*Result, error) {
	var hints map[string]string
	var overlay []byte
	for _, o := range opts {
		if len(o.Hints) > 0 {
			hints = o.Hints
		}
		if len(o.CounterpartOverlay) > 0 {
			overlay = o.CounterpartOverlay
		}
	}
	merged := make(map[string]string, len(stdlibHints)+len(hints))
	for k, v := range stdlibHints {
		merged[k] = v
	}
	for k, v := range hints {
		merged[k] = v
	}
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(root)
	a := &Analyzer{
		root: abs,
		nm:   findNodeModules(abs),
		Result: &Result{
			Root:         abs,
			Nodes:        map[string]*Node{},
			StdlibHints:  merged,
			Counterparts: LoadCounterparts(overlay),
		},
	}
	// seed internal nodes
	for _, f := range files {
		fabs, _ := filepath.Abs(f)
		name := a.relName(fabs)
		a.Result.Nodes[name] = &Node{Name: name, Path: filepath.ToSlash(fabs), Kind: KindInternal}
		a.Result.Order = append(a.Result.Order, name)
	}
	a.analyzeEntryFiles()
	// expand every external package's subtree
	a.analyzeExternal()
	return a.Result, nil
}

// Non-lowering helpers ------------------------------------------------------

func nonEmptyLines(code []byte) int {
	n := 0
	for _, line := range strings.Split(string(code), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func sourceLoc(dir string) int {
	total := 0
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(p)
		if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") {
			if strings.Contains(base, ".min.") {
				return nil
			}
			if b, err := os.ReadFile(p); err == nil {
				total += nonEmptyLines(b)
			}
		}
		return nil
	})
	return total
}

func readPackage(dir string) (packageJSON, error) {
	var pj packageJSON
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return pj, err
	}
	json.Unmarshal(b, &pj)
	return pj, nil
}

// Public-API signal: the set of exported symbol names a package exposes. This
// sharpens the "too small to split (leftPad-class)" vs "worth a repo" call:
// a leftPad-style dep has a single export while a real library has many.
func discoveredExports(dir string, pj packageJSON) []string {
	// 1) Try the package entry ("main", else index), honoring its real
	//    extension (.js, .cjs, .mjs, .ts — do NOT force .js).
	entry := pj.Main
	if entry == "" {
		entry = "index.js"
	}
	var candidates []string
	for _, cand := range []string{entry, entry + ".js", entry + ".cjs", entry + ".mjs"} {
		if strings.HasSuffix(cand, ".js") || strings.HasSuffix(cand, ".cjs") || strings.HasSuffix(cand, ".mjs") {
			candidates = append(candidates, filepath.Join(dir, cand))
		}
	}
	seenPkg := map[string]bool{}
	var out []string
	for _, full := range candidates {
		b, err := os.ReadFile(full)
		if err != nil || seenPkg[full] {
			continue
		}
		seenPkg[full] = true
		if got := entryExports(string(b)); len(got) > 0 {
			return got
		}
	}
	// 2) Fall back: scan the whole package source so lib/dist subfiles and
	//    function-with-properties entries are still counted.
	scanPkg(dir, func(src string) { out = union(out, entryExports(src)) })
	return out
}

// scanPkg walks a package dir (skipping nested node_modules) and calls fn
// with each JS/TS source file's text.
func scanPkg(dir string, fn func(src string)) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") || strings.Contains(base, ".min.") {
			return nil
		}
		switch filepath.Ext(p) {
		case ".js", ".jsx", ".cjs", ".mjs", ".ts", ".tsx":
		default:
			return nil
		}
		if b, err := os.ReadFile(p); err == nil {
			fn(string(b))
		}
		return nil
	})
}

func union(list []string, add []string) []string {
	for _, v := range add {
		list = addUnique(list, v)
	}
	return list
}

// entryExports extracts export names from a CommonJS/ESM entry source:
//
//	module.exports.foo / exports.foo  (member assignments)
//	module.exports = { a, b, "c": ..., [d]: ... }  (object-literal export)
//	export { a, b }                                 (ESM)
//	export default                                  (declared)
func entryExports(src string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, m := range memberExportRe.FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	// module.exports = { ... }  (allow whitespace)
	for _, m := range objExportRe.FindAllStringSubmatchIndex(src, -1) {
		bodyStart := m[1]
		for _, k := range objectKeys(src[bodyStart:]) {
			add(k)
		}
	}
	for _, m := range esmExportRe.FindAllStringSubmatch(src, -1) {
		for _, name := range strings.Split(m[1], ",") {
			add(strings.TrimSpace(name))
		}
	}
	if strings.Contains(src, "export default") {
		add("default")
	}
	return out
}

// objectKeys scans a string that begins just after "module.exports = {" and
// extracts the top-level property keys, balancing nested braces so a trailing
// object literal doesn't leak. Stops at the matching close brace.
func objectKeys(body string) []string {
	var keys []string
	depth := 0
	var cur strings.Builder
	flush := func() {
		t := strings.TrimSpace(cur.String())
		cur.Reset()
		if t == "" {
			return
		}
		// key is the part before ':' (or the whole token for shorthand)
		key := t
		if i := strings.IndexByte(t, ':'); i >= 0 {
			key = strings.TrimSpace(t[:i])
		}
		key = strings.Trim(key, `"'`)
		if key != "" && key != "..." && !strings.HasPrefix(key, "[") {
			keys = append(keys, key)
		}
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch c {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				flush()
				return keys
			}
			depth--
		case ',', '\n':
			if depth == 0 {
				flush()
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		cur.WriteByte(c)
	}
	return keys
}

var (
	memberExportRe = regexp.MustCompile(`\b(?:module\.exports|exports)\.([A-Za-z_$][\w$]*)\b`)
	objExportRe    = regexp.MustCompile(`module\.exports\s*=\s*\{`)
	esmExportRe    = regexp.MustCompile(`export\s*\{([^}]*)\}`)
)

func addUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// resolveRelative resolves a relative specifier to an existing file,
// trying the common extensions and index convention.
func resolveRelative(dir, spec string) (string, bool) {
	try := func(p string) (string, bool) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.ToSlash(p), true
		}
		return "", false
	}
	if p, ok := try(filepath.Join(dir, spec)); ok {
		return p, ok
	}
	for _, ext := range []string{".js", ".ts", ".tsx", ".jsx", ".mjs", ".cjs"} {
		if p, ok := try(filepath.Join(dir, spec+ext)); ok {
			return p, ok
		}
	}
	// index
	if st, err := os.Stat(filepath.Join(dir, spec)); err == nil && st.IsDir() {
		for _, f := range []string{"index.js", "index.ts", "index.mjs", "index.tsx"} {
			if p, ok := try(filepath.Join(dir, spec, f)); ok {
				return p, ok
			}
		}
	}
	return "", false
}

// extractSpecs pulls every module specifier out of a file: ES import
// declarations, import-equals, and CommonJS require()/dynamic import()
// uses. A regex pass covers the forms the AST makes awkward (import-equals,
// dynamic re-exports); the structured walkers cover the common ones.
func extractSpecs(sf *tsmorph.SourceFile) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, im := range sf.ImportDeclarations() {
		add(im.ModuleSpecifier())
	}
	// require("...") call expressions — AST-based so comment/string content
	// (e.g. rule docs showing `require("no-unused-vars")`) is not counted.
	for _, n := range sf.DescendantsOfKind(ast.KindCallExpression) {
		call, ok := n.AsCallExpression()
		if !ok {
			continue
		}
		fn, _ := call.Expression()
		if fn.Text() != "require" {
			continue
		}
		if args := call.Arguments(); len(args) == 1 {
			if lit, ok := args[0].AsStringLiteral(); ok {
				add(lit.Value())
			}
		}
	}
	src := sf.Text()
	// import x = require("...")
	for _, m := range importEqualsRe.FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	// re-export from "..."
	for _, m := range fromRe.FindAllStringSubmatch(src, -1) {
		if !strings.HasPrefix(m[1], "node:") {
			add(m[1])
		}
	}
	return out
}
