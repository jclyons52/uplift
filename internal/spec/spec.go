// Package spec ingests TypeScript declaration files (.d.ts) and extracts the
// exported API surface into a stable, machine-readable contract (schema
// spec/v1) that a port can target and be checked against.
//
// This is the "port-to-spec" path: a library like ESLint ships its types
// separately (v8 → @types/eslint on DefinitelyTyped; v9 → bundled lib/types).
// The JS source may be 0% typed, but the .d.ts is the authoritative signature
// spec for the public API. Running this extractor over the .d.ts produces the
// contract the Go port must reproduce — class/interface names, extends,
// method and property signatures, type aliases, enums, and namespaces —
// turning "port from scratch" into "port to a spec".
package spec

import (
	"strings"

	tsmorph "github.com/jclyons52/ts-go-morph"
	"github.com/jclyons52/ts-go-morph/third_party/typescript-go/ts/ast"
)

// Schema is the stable schema identifier for a spec report.
const Schema = "spec/v1"

// Spec is the top-level API contract for one declaration file (or tree).
type Spec struct {
	Schema  string  `json:"schema"`            // always "spec/v1"
	Package string  `json:"package,omitempty"` // npm/package name when known
	File    string  `json:"file,omitempty"`    // source .d.ts name (dir mode: tree root)
	Entries []Entry `json:"entries"`
}

// Entry is one top-level exported declaration.
type Entry struct {
	Kind     string   `json:"kind"`              // class|interface|type|function|const|namespace|enum|var|re-export|declaration
	Name     string   `json:"name,omitempty"`    // exported symbol name
	Source   string   `json:"source,omitempty"`  // .d.ts file stem it came from (dir mode)
	Text     string   `json:"text,omitempty"`    // compact one-line signature
	TypeText string   `json:"type,omitempty"`    // for type alias / const/var
	Extends  []string `json:"extends,omitempty"` // class extends / interface extends
	Members  []Member `json:"members,omitempty"` // class/interface/namespace members
	Params   []Param  `json:"params,omitempty"`  // function entries
	Returns  string   `json:"returns,omitempty"` // function entries
	Exported bool     `json:"exported"`
	Default  bool     `json:"default,omitempty"`
}

// Member is a class/interface/namespace member.
type Member struct {
	Kind      string   `json:"kind"` // method|property|constructor|get|set|indexSignature|call|construct|function|class|interface|type|const|enum|namespace|member
	Name      string   `json:"name,omitempty"`
	Signature string   `json:"signature,omitempty"` // compact one-line, e.g. "verify(code, config, options) => LintResult[]"
	Type      string   `json:"type,omitempty"`      // property / function / alias type
	Static    bool     `json:"static,omitempty"`
	Params    []Param  `json:"params,omitempty"`
	Returns   string   `json:"returns,omitempty"`
	Members   []Member `json:"members,omitempty"` // nested (namespace bodies)
}

// Param is a single function/method/constructor parameter.
type Param struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Rest     bool   `json:"rest,omitempty"`
}

// Extract walks the exported declarations of a .d.ts SourceFile and returns
// the API contract. source is the attribution stem (usually the file base
// name without the .d.ts extension).
func Extract(p *tsmorph.Project, sf *tsmorph.SourceFile, source string) *Spec {
	s := &Spec{Schema: Schema, File: source}
	src := sf.Text()
	for _, stmt := range sf.Statements() {
		if stmt.IsModuleDeclaration() {
			// Ambient module declarations — dts-bundle output especially —
			// wrap the whole public surface in `declare module "..." { ... }`
			// and are NOT themselves marked exported. Surface the module
			// body as a container entry so the contract isn't empty.
			md, _ := stmt.AsModuleDeclaration()
			e := namespaceEntry(md, source, src)
			e.Name = strings.Trim(e.Name, `"`)
			e.Text = "module " + e.Name
			s.Entries = append(s.Entries, *e)
			continue
		}
		if !exportVisible(stmt) {
			continue
		}
		if e := extractTop(stmt, source, src); e != nil {
			s.Entries = append(s.Entries, *e)
		}
	}
	return s
}

// exportVisible reports whether a top-level statement contributes to the
// public surface (an `export` modifier, a default export, or an
// `export =` assignment).
func exportVisible(n tsmorph.Node) bool {
	return n.IsExported() || n.IsDefaultExport() || n.IsExportAssignment()
}

// extractTop dispatches a top-level statement to its kind-specific extractor.
func extractTop(n tsmorph.Node, source, src string) *Entry {
	switch {
	case n.IsClassDeclaration():
		cd, _ := n.AsClassDeclaration()
		return classEntry(cd, source, src)
	case n.IsInterfaceDeclaration():
		id, _ := n.AsInterfaceDeclaration()
		return interfaceEntry(id, source, src)
	case n.IsFunctionDeclaration():
		fd, _ := n.AsFunctionDeclaration()
		return functionEntry(fd, source, src)
	case n.IsTypeAliasDeclaration():
		td, _ := n.AsTypeAliasDeclaration()
		return aliasEntry(td, source, src)
	case n.IsEnumDeclaration():
		ed, _ := n.AsEnumDeclaration()
		return enumEntry(ed, source)
	case n.IsVariableStatement():
		vs, _ := n.AsVariableStatement()
		return variableEntry(vs, source, src)
	case n.IsModuleDeclaration():
		md, _ := n.AsModuleDeclaration()
		return namespaceEntry(md, source, src)
	case n.IsExportDeclaration():
		return exportDeclEntry(n, source, src)
	default:
		// Exotic or unknown top-level export: keep name + text so nothing
		// silently drops off the surface.
		return &Entry{Kind: "declaration", Name: n.Name(), Text: oneLine(sliceText(n.ASTNode(), src)), Source: source, Exported: n.IsExported(), Default: n.IsDefaultExport()}
	}
}

// classEntry builds a class contract from its members and heritage.
func classEntry(cd tsmorph.ClassDeclaration, source, src string) *Entry {
	e := &Entry{Kind: "class", Name: cd.Name(), Source: source, Exported: true, Default: cd.IsDefaultExport()}
	if ex := cd.Extends(); strings.TrimSpace(ex) != "" {
		e.Extends = append(e.Extends, ex)
	}
	e.Extends = append(e.Extends, cd.Implements()...)
	for _, c := range cd.Children() {
		e.Members = append(e.Members, memberOf(c, src)...)
	}
	e.Text = classText(e)
	return e
}

func classText(e *Entry) string {
	s := "class " + e.Name
	if len(e.Extends) > 0 {
		s += " extends " + strings.Join(e.Extends, ", ")
	}
	return s
}

// interfaceEntry builds an interface contract from its members.
func interfaceEntry(id tsmorph.InterfaceDeclaration, source, src string) *Entry {
	e := &Entry{Kind: "interface", Name: id.Name(), Source: source, Exported: id.IsExported(), Default: id.IsDefaultExport()}
	e.Extends = append(e.Extends, id.Extends()...)
	for _, m := range id.Members() {
		e.Members = append(e.Members, memberOf(m, src)...)
	}
	e.Text = "interface " + e.Name
	return e
}

// functionEntry builds a top-level function contract.
func functionEntry(fd tsmorph.FunctionDeclaration, source, src string) *Entry {
	e := &Entry{Kind: "function", Name: fd.Name(), Source: source, Exported: fd.IsExported(), Default: fd.IsDefaultExport()}
	e.Params, e.Returns = extractSignature(fd.ASTNode(), src)
	e.Text = renderSig(fd.Name(), e.Params, e.Returns)
	return e
}

// aliasEntry builds a `type X = ...` contract.
func aliasEntry(td tsmorph.TypeAliasDeclaration, source, src string) *Entry {
	ty, _ := td.TypeNode()
	t := strings.TrimSpace(sliceText(ty.ASTNode(), src))
	return &Entry{Kind: "type", Name: td.Name(), TypeText: t, Source: source, Exported: td.IsExported(), Default: td.IsDefaultExport(), Text: "type " + td.Name() + " = " + t}
}

// enumEntry builds an enum contract with its member names.
func enumEntry(ed tsmorph.EnumDeclaration, source string) *Entry {
	e := &Entry{Kind: "enum", Name: ed.Name(), Source: source, Exported: ed.IsExported(), Default: ed.IsDefaultExport()}
	for _, m := range ed.Members() {
		e.Members = append(e.Members, Member{Kind: "member", Name: m.Name()})
	}
	return e
}

// variableEntry builds a `const`/`let`/`var` contract (typed constants).
func variableEntry(vs tsmorph.VariableStatement, source, src string) *Entry {
	kind := vs.DeclarationKind()
	if kind == "" {
		kind = "const"
	}
	for _, d := range vs.Declarations() {
		ty := ""
		if tn, ok := d.TypeNode(); ok {
			ty = strings.TrimSpace(sliceText(tn.ASTNode(), src))
		}
		e := &Entry{Kind: kind, Name: d.Name(), TypeText: ty, Source: source, Exported: vs.IsExported(), Default: vs.IsDefaultExport()}
		e.Text = kind + " " + e.Name + ":" + e.TypeText
		return e
	}
	return nil
}

// namespaceEntry builds a `declare namespace X { ... }` contract, recursing
// into the module body's statements as nested members.
func namespaceEntry(md tsmorph.ModuleDeclaration, source, src string) *Entry {
	e := &Entry{Kind: "namespace", Name: md.Name(), Source: source, Exported: md.IsExported(), Default: md.IsDefaultExport(), Text: "namespace " + md.Name()}
	body, ok := md.Node.GetBody()
	if !ok {
		return e
	}
	if mb, ok := body.AsModuleBlock(); ok {
		for _, stmt := range mb.Node.GetStatements() {
			e.Members = append(e.Members, memberOf(stmt, src)...)
		}
	}
	return e
}

// exportDeclEntry captures re-exports (`export { a, b } from "x"` and
// `export * from "x"`) as surface pointers.
func exportDeclEntry(n tsmorph.Node, source, src string) *Entry {
	ed, _ := n.AsExportDeclaration()
	return &Entry{Kind: "re-export", Name: strings.Join(ed.NamedExports(), ", "), Source: source, Text: oneLine(sliceText(n.ASTNode(), src)), Exported: true}
}

// memberOf turns a class/interface member or namespace statement into one or
// more Member entries.
func memberOf(n tsmorph.Node, src string) []Member {
	a := n.ASTNode()
	// Tokens (comments, synthetic keywords) and nil children carry no surface.
	if a == nil || ast.IsToken(a) {
		return nil
	}
	switch {
	case n.IsMethodDeclaration() || n.IsMethodSignatureDeclaration():
		return []Member{signatureMember(n, "method", n.Name(), src)}
	case a.Kind == ast.KindConstructor:
		return []Member{signatureMember(n, "constructor", "constructor", src)}
	case n.IsGetAccessorDeclaration():
		_, ret := extractSignature(a, src)
		ty := ret
		return []Member{{Kind: "get", Name: n.Name(), Type: ty, Signature: "get " + n.Name() + "():" + ty, Returns: ty}}
	case n.IsSetAccessorDeclaration():
		params, _ := extractSignature(a, src)
		return []Member{{Kind: "set", Name: n.Name(), Signature: renderSig("set "+n.Name(), params, ""), Params: params}}
	case n.IsPropertyDeclaration() || n.IsPropertySignatureDeclaration():
		_, ty := extractPropertyType(a, src)
		return []Member{{Kind: "property", Name: n.Name(), Type: ty, Signature: n.Name() + ":" + ty, Static: isStatic(a)}}
	case n.IsIndexSignatureDeclaration():
		return []Member{{Kind: "indexSignature", Signature: oneLine(sliceText(a, src))}}
	case n.IsCallSignatureDeclaration():
		return []Member{signatureMember(n, "call", "", src)}
	case n.IsConstructSignatureDeclaration():
		return []Member{signatureMember(n, "construct", "", src)}
	case n.IsFunctionDeclaration():
		return []Member{signatureMember(n, "function", n.Name(), src)}
	case n.IsClassDeclaration():
		cd, _ := n.AsClassDeclaration()
		ce := classEntry(cd, "", src)
		m := Member{Kind: "class", Name: ce.Name, Signature: classText(ce), Type: strings.Join(ce.Extends, ", "), Static: isStatic(a)}
		m.Members = ce.Members
		return []Member{m}
	case n.IsInterfaceDeclaration():
		id, _ := n.AsInterfaceDeclaration()
		ie := interfaceEntry(id, "", src)
		return []Member{{Kind: "interface", Name: ie.Name, Signature: "interface " + ie.Name, Type: strings.Join(ie.Extends, ", "), Members: ie.Members}}
	case n.IsTypeAliasDeclaration():
		td, _ := n.AsTypeAliasDeclaration()
		ty, _ := td.TypeNode()
		t := strings.TrimSpace(sliceText(ty.ASTNode(), src))
		return []Member{{Kind: "type", Name: td.Name(), Signature: "type " + td.Name() + " = " + t, Type: t}}
	case n.IsEnumDeclaration():
		ed, _ := n.AsEnumDeclaration()
		m := Member{Kind: "enum", Name: ed.Name()}
		for _, mm := range ed.Members() {
			m.Members = append(m.Members, Member{Kind: "member", Name: mm.Name()})
		}
		return []Member{m}
	case n.IsVariableStatement():
		vs, _ := n.AsVariableStatement()
		var out []Member
		for _, d := range vs.Declarations() {
			t := ""
			if ty, ok := d.TypeNode(); ok {
				t = strings.TrimSpace(sliceText(ty.ASTNode(), src))
			}
			out = append(out, Member{Kind: "const", Name: d.Name(), Signature: "const " + d.Name() + ":" + t, Type: t})
		}
		return out
	case n.IsModuleDeclaration():
		md, _ := n.AsModuleDeclaration()
		ne := namespaceEntry(md, "", src)
		return []Member{{Kind: "namespace", Name: ne.Name, Signature: "namespace " + ne.Name, Members: ne.Members}}
	default:
		// Unknown member: keep name + text so the surface isn't dropped.
		return []Member{{Kind: "member", Name: n.Name(), Signature: oneLine(sliceText(a, src))}}
	}
}

// signatureMember extracts a structured method/function/call/construct
// signature member.
func signatureMember(n tsmorph.Node, kind, name, src string) Member {
	m := Member{Kind: kind, Name: name, Static: isStatic(n.ASTNode())}
	m.Params, m.Returns = extractSignature(n.ASTNode(), src)
	m.Signature = renderSig(funcName(kind, name), m.Params, m.Returns)
	return m
}

// funcName renders the readable preamble for a signature.
func funcName(kind, name string) string {
	switch kind {
	case "constructor", "construct":
		return "new " + name
	case "call":
		return "fn"
	case "get":
		return "get " + name
	case "set":
		return "set " + name
	default:
		if name != "" {
			return name
		}
		return "fn"
	}
}

// renderSig renders "name(params) => returns".
func renderSig(name string, params []Param, returns string) string {
	var parts []string
	for _, p := range params {
		n := p.Name
		if n == "" {
			n = "_"
		}
		t := p.Type
		if t == "" {
			t = "any"
		}
		if p.Rest {
			parts = append(parts, "..."+n+":"+t)
		} else if p.Optional {
			parts = append(parts, n+"?:"+t)
		} else {
			parts = append(parts, n+":"+t)
		}
	}
	s := name + "(" + strings.Join(parts, ", ") + ")"
	if strings.TrimSpace(returns) != "" {
		s += " => " + strings.TrimSpace(returns)
	}
	return s
}

// extractSignature reads parameters and return type from a function-like ast
// node (functions, methods, method signatures, call/construct signatures,
// accessors).
func extractSignature(a *ast.Node, src string) ([]Param, string) {
	if a == nil {
		return nil, ""
	}
	var params []Param
	for _, p := range a.Parameters() {
		pp := Param{}
		if nm := p.Name(); nm != nil {
			pp.Name = strings.TrimSpace(sliceText(nm, src))
		}
		if ty := p.Type(); ty != nil {
			pp.Type = strings.TrimSpace(sliceText(ty, src))
		}
		pp.Optional = p.QuestionToken() != nil
		if pd := p.AsParameterDeclaration(); pd != nil {
			pp.Rest = pd.DotDotDotToken != nil
		}
		params = append(params, pp)
	}
	ret := ""
	if t := a.Type(); t != nil {
		ret = strings.TrimSpace(sliceText(t, src))
	}
	return params, ret
}

// extractPropertyType returns the declared type text of a property (or
// index-signature key type); empty if undeclared.
func extractPropertyType(a *ast.Node, src string) (key, typ string) {
	if a == nil {
		return "", ""
	}
	if t := a.Type(); t != nil {
		return "", strings.TrimSpace(sliceText(t, src))
	}
	return "", ""
}

// isStatic reports whether the ast node carries the static modifier.
func isStatic(a *ast.Node) bool {
	return a != nil && ast.HasSyntacticModifier(a, ast.ModifierFlagsStatic)
}

// oneLine collapses whitespace.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// sliceText returns the source text of an ast node by slicing the source by
// [Pos, End). Unlike ast.Node.Text(), this never panics on node kinds the
// vendored compiler fails to render (e.g. type-reference nodes); it returns
// "" when the range is out of bounds.
func sliceText(a *ast.Node, src string) string {
	if a == nil {
		return ""
	}
	start, end := a.Pos(), a.End()
	if start < 0 || end > len(src) || start > end {
		return ""
	}
	return src[start:end]
}
