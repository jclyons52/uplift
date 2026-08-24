package spec

import (
	"encoding/json"
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// extractDT parses the given .d.ts body and extracts its API contract.
func extractDT(t *testing.T, body string) *Spec {
	t.Helper()
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	sf := p.CreateSourceFile("/test/index.d.ts", body)
	if sf == nil {
		t.Fatalf("could not parse .d.ts")
	}
	return Extract(p, sf, "index")
}

func TestExtractBundledModule(t *testing.T) {
	// dts-bundle output wraps the entire API surface in a single ambient
	// `declare module "..."` block (regexpp's index.d.ts, @types/eslint).
	// The wrapper is not marked exported; its body carries the exports.
	s := extractDT(t, `declare module "escaped-syntax" {
  import * as AST from "escaped-syntax/ast";
  export class RegExpParser {
    parsePattern(source: string, options?: { u?: boolean }): AST.Pattern;
  }
  export function parseRegExpLiteral(source: string | RegExp, options?: { u?: boolean }): AST.RegExpLiteral;
  type Options = { u?: boolean };
  export type RegExpParserOptions = Options;
  export const latestEcmaVersion = 2025;
}`)
	if len(s.Entries) != 1 {
		t.Fatalf("want 1 container entry, got %d", len(s.Entries))
	}
	e := s.Entries[0]
	if e.Kind != "namespace" || e.Name != "escaped-syntax" {
		t.Fatalf("want namespace escaped-syntax, got %s %s", e.Kind, e.Name)
	}
	var sawParser, sawFn bool
	for _, m := range e.Members {
		if m.Kind == "class" && m.Name == "RegExpParser" {
			sawParser = true
			var hasParse bool
			for _, cm := range m.Members {
				if cm.Kind == "method" && cm.Name == "parsePattern" {
					hasParse = true
					if !strings.Contains(cm.Signature, "AST.Pattern") {
						t.Fatalf("parsePattern signature = %q", cm.Signature)
					}
				}
			}
			if !hasParse {
				t.Fatalf("RegExpParser missing parsePattern; members=%+v", m.Members)
			}
		}
		if m.Kind == "function" && m.Name == "parseRegExpLiteral" {
			sawFn = true
		}
	}
	if !sawParser || !sawFn {
		t.Fatalf("missing class/fn members; members=%+v", e.Members)
	}
	// Untyped consts (e.g. `export const latestEcmaVersion = 2025`) must not
	// panic the extractor (TypeNode returns a zero node + ok=false).
	var sawConst bool
	for _, m := range e.Members {
		if m.Kind == "const" && m.Name == "latestEcmaVersion" {
			sawConst = true
		}
	}
	if !sawConst {
		t.Fatalf("missing const member; members=%+v", e.Members)
	}
}

func TestExtractClass(t *testing.T) {
	s := extractDT(t, `export declare class Linter {
    constructor(config?: Linter.Config);
    verify(codeOrSourceCode: string | SourceCode, config: Linter.Config, filenameOrOptions?: string | Linter.FixOptions): LintResult[];
    static version: string;
    private _config;
}`)
	if len(s.Entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(s.Entries))
	}
	e := s.Entries[0]
	if e.Kind != "class" || e.Name != "Linter" {
		t.Fatalf("want class Linter, got %s %s", e.Kind, e.Name)
	}
	if e.Text != "class Linter" {
		t.Fatalf("text = %q", e.Text)
	}
	// constructor + verify + static version (private _config dropped by exportVisible? it's a child)
	var hasVerify, hasCtor bool
	for _, m := range e.Members {
		if m.Kind == "method" && m.Name == "verify" {
			hasVerify = true
			if !strings.Contains(m.Signature, "=> LintResult[]") {
				t.Fatalf("verify signature = %q", m.Signature)
			}
			if len(m.Params) != 3 {
				t.Fatalf("verify params = %d", len(m.Params))
			}
		}
		if m.Kind == "constructor" {
			hasCtor = true
		}
	}
	if !hasVerify || !hasCtor {
		t.Fatalf("missing verify/ctor; members=%+v", e.Members)
	}
}

func TestExtractInterfaceAndType(t *testing.T) {
	s := extractDT(t, `export interface Config {
    rules?: RulesRecord;
    settings?: Record<string, unknown>;
}
export type RulesRecord = Record<string, Rule.RuleModule | undefined>;
export interface Foo extends Config { extra: string; }
`)
	kinds := map[string]int{}
	for _, e := range s.Entries {
		kinds[e.Kind]++
	}
	if kinds["interface"] != 2 || kinds["type"] != 1 {
		t.Fatalf("kinds wrong: %v (entries=%d)", kinds, len(s.Entries))
	}
	var foo *Entry
	for i := range s.Entries {
		if s.Entries[i].Name == "Foo" {
			foo = &s.Entries[i]
		}
	}
	if foo == nil || len(foo.Extends) != 1 || foo.Extends[0] != "Config" {
		t.Fatalf("foo extends = %+v", foo)
	}
}

func TestExtractNamespaceAndEnum(t *testing.T) {
	s := extractDT(t, `export declare namespace Linter {
    interface Config { rules?: RulesRecord; }
    type FixOptions = { fix?: boolean; };
    function getSourceCode(code: string): string;
}
export declare enum Severity { Off = 0, Warn = 1, Error = 2 }`)
	var ns *Entry
	for i := range s.Entries {
		if s.Entries[i].Kind == "namespace" {
			ns = &s.Entries[i]
		}
	}
	if ns == nil {
		t.Fatalf("no namespace entry")
	}
	var hasInterface, hasFunc bool
	for _, m := range ns.Members {
		if m.Kind == "interface" && m.Name == "Config" {
			hasInterface = true
		}
		if m.Kind == "function" && m.Name == "getSourceCode" {
			hasFunc = true
			if len(m.Params) != 1 || m.Params[0].Name != "code" {
				t.Fatalf("getSourceCode params = %+v", m.Params)
			}
		}
	}
	if !hasInterface || !hasFunc {
		t.Fatalf("namespace missing members: %+v", ns.Members)
	}
	var en *Entry
	for i := range s.Entries {
		if s.Entries[i].Kind == "enum" {
			en = &s.Entries[i]
		}
	}
	if en == nil || len(en.Members) != 3 {
		t.Fatalf("enum members = %+v", en)
	}
}

func TestExtractRestOptionalParams(t *testing.T) {
	s := extractDT(t, `export declare function format(code: string, opts?: Options, ...rest: string[]): string;`)
	e := s.Entries[0]
	if e.Params[0].Name != "code" || e.Params[0].Optional {
		t.Fatalf("p0 = %+v", e.Params[0])
	}
	if !e.Params[1].Optional {
		t.Fatalf("p1 should be optional: %+v", e.Params[1])
	}
	if !e.Params[2].Rest || e.Params[2].Type != "string[]" {
		t.Fatalf("p2 should be rest string[]: %+v", e.Params[2])
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s := extractDT(t, `export declare class A { m(x: number): void; }`)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"schema":"spec/v1"`) {
		t.Fatalf("missing schema: %s", b)
	}
	if !strings.Contains(string(b), `"kind":"class"`) {
		t.Fatalf("missing class kind: %s", b)
	}
}
