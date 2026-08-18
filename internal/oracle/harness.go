package oracle

// harnessGo is the Go-side test harness written next to the transpiled
// module+test. It provides the describe/it/assert scaffolding the transpiled
// test references (after the fix-up pass) and prints the parity number.
//
// Integrity: an `it` only counts as a pass if it executed at least one real
// assertion AND did not panic. A test whose assertions were gapped out by the
// transpiler has zero counts and is therefore a FAIL (never a vacuous pass),
// so "N/N PARITY" can only mean genuinely-checked behaviour.
const harnessGo = `package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

type oracleCase struct {
	name    string
	pass    bool
	asserts int // real assertion calls executed by this case
	err     string
}

var oracleCases []oracleCase
var curCase *oracleCase

func oracleDescribe(_ string, fn func()) { fn() }

func oracleIt(name string, fn func()) {
	c := oracleCase{name: name}
	prev := curCase
	curCase = &c
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.pass, c.err = false, fmt.Sprint(r)
			}
		}()
		fn()
		c.pass = true
	}()
	// Vacuous-pass guard: an it() with no executable assertion cannot
	// verify anything against the JS baseline — count it as a failure so
	// it never inflates the Go pass count.
	if c.asserts == 0 {
		c.pass = false
		if c.err == "" {
			c.err = "no executable assertion (gapped/dropped by transpiler?)"
		}
	}
	curCase = prev
	oracleCases = append(oracleCases, c)
}

func markAssert() {
	if curCase != nil {
		curCase.asserts++
	}
}

// numVal reports whether v is a JS-number-ish Go value and its float64 form.
// JS has a single number type; Go literals (int) and JSON.parse (float64)
// must compare equal.
func numVal(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// normObj coerces an *jsrtObj to a map[string]any so equality/marshalling
// treat ordered and unordered objects identically (order only matters to
// yaml-dump / Object.keys).
func normObj(v any) any {
	if o, ok := v.(*jsrtObj); ok {
		m := make(map[string]any, len(*o))
		for _, e := range *o {
			m[e.K] = e.V
		}
		return m
	}
	return v
}

// jsrtDeepEqual is reflect.DeepEqual with JS numeric + deep-map/slice
// coercion: 2 == 2.0, and numbers nested in maps/slices compare numerically.
func jsrtDeepEqual(a, b any) bool {
	a = normObj(a)
	b = normObj(b)
	av, aNum := numVal(a)
	bv, bNum := numVal(b)
	if aNum && bNum {
		return av == bv
	}
	switch aa := a.(type) {
	case map[string]any:
		bb, ok := b.(map[string]any)
		if !ok || len(aa) != len(bb) {
			return false
		}
		for k, va := range aa {
			vb, ok := bb[k]
			if !ok || !jsrtDeepEqual(va, vb) {
				return false
			}
		}
		return true
	case []any:
		bb, ok := b.([]any)
		if !ok || len(aa) != len(bb) {
			return false
		}
		for i := range aa {
			if !jsrtDeepEqual(aa[i], bb[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// jsonObj deep-normalizes jsrtObj/map/slice values to plain JSON-ready
// structures (nested jsrtObj → maps, recursively), so JSON.stringify and
// equality treat ordered objects like plain objects.
func jsonObj(v any) any {
	switch x := v.(type) {
	case *jsrtObj:
		m := make(map[string]any, len(*x))
		for _, e := range *x {
			m[e.K] = jsonObj(e.V)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = jsonObj(e)
		}
		return out
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = jsonObj(val)
		}
		return m
	}
	return v
}

func jsrtStringify(v any) string {
	b, err := json.Marshal(jsonObj(v))
	if err != nil {
		return ""
	}
	return string(b)
}

func jsrtParse(s any) any {
	var v any
	if err := json.Unmarshal([]byte(fmt.Sprint(s)), &v); err != nil {
		panic("JSON.parse: " + err.Error())
	}
	return v
}

// jsrtKeys returns the string keys of a string-keyed map as []any (JS
// Object.keys over a plain object), preserving insertion order for jsrtObj.
func jsrtKeys(m any) []any {
	switch x := m.(type) {
	case map[string]any:
		out := make([]any, 0, len(x))
		for k := range x {
			out = append(out, k)
		}
		return out
	case *jsrtObj:
		out := make([]any, 0, len(*x))
		for _, e := range *x {
			out = append(out, e.K)
		}
		return out
	}
	return []any{}
}

// jsrtValues returns the values of a string-keyed map as []any.
func jsrtValues(m any) []any {
	switch x := m.(type) {
	case map[string]any:
		out := make([]any, 0, len(x))
		for _, v := range x {
			out = append(out, v)
		}
		return out
	case *jsrtObj:
		out := make([]any, 0, len(*x))
		for _, e := range *x {
			out = append(out, e.V)
		}
		return out
	}
	return []any{}
}

// jsrtEntries returns [key, value] tuples for a string-keyed map as []any.
func jsrtEntries(m any) []any {
	switch x := m.(type) {
	case map[string]any:
		out := make([]any, 0, len(x))
		for k, v := range x {
			out = append(out, []any{k, v})
		}
		return out
	case *jsrtObj:
		out := make([]any, 0, len(*x))
		for _, e := range *x {
			out = append(out, []any{e.K, e.V})
		}
		return out
	}
	return []any{}
}

func assertStrictEqual(got, want any) {
	markAssert()
	if !jsrtDeepEqual(got, want) {
		panic(fmt.Sprintf("strictEqual: got %#v want %#v", got, want))
	}
}

func assertEqual(got, want any) { assertStrictEqual(got, want) }

func assertDeepEqual(got, want any) {
	markAssert()
	if !jsrtDeepEqual(got, want) {
		panic(fmt.Sprintf("deepEqual: got %#v want %#v", got, want))
	}
}

func assertIsTrue(v any) {
	markAssert()
	if b, ok := v.(bool); !ok || !b {
		panic(fmt.Sprintf("isTrue: got %#v", v))
	}
}

func assertIsFalse(v any) {
	markAssert()
	if b, ok := v.(bool); !ok || b {
		panic(fmt.Sprintf("isFalse: got %#v", v))
	}
}

func assertOK(v any) {
	markAssert()
	if v == nil || v == false {
		panic(fmt.Sprintf("ok: got %#v", v))
	}
}

func assertInclude(hay, needle any) {
	markAssert()
	if !strings.Contains(fmt.Sprint(hay), fmt.Sprint(needle)) {
		panic(fmt.Sprintf("include: %#v does not contain %#v", hay, needle))
	}
}

func assertNotInclude(hay, needle any) {
	markAssert()
	if strings.Contains(fmt.Sprint(hay), fmt.Sprint(needle)) {
		panic(fmt.Sprintf("notInclude: %#v unexpectedly contains %#v", hay, needle))
	}
}

// yamlDump is a js-yaml-compatible block-style dumper (indent 2, plain
// scalars, block mappings/sequences) used to shim js-yaml's dump method.
func yamlDump(v any) string {
	var b strings.Builder
	yamlBlock(&b, v, 0)
	return strings.TrimSuffix(b.String(), "\n")
}

func yamlScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == math.Trunc(x) {
			return fmt.Sprintf("%.0f", x)
		}
		return fmt.Sprintf("%v", x)
	case int:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// yamlBlock writes v at the given indent. For an array at a mapping-key
// position, the first item's mapping keys start inline after "- " and the
// rest align +2 (js-yaml default block style).
func yamlBlock(b *strings.Builder, v any, indent int) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			b.WriteString(strings.Repeat(" ", indent) + "-")
			yamlItem(b, item, indent)
			b.WriteString("\n")
		}
	default:
		if pairs, ok := yamlPairs(v); ok {
			for _, p := range pairs {
				b.WriteString(strings.Repeat(" ", indent) + fmt.Sprint(p[0]) + ":")
				yamlVal(b, p[1], indent)
				b.WriteString("\n")
			}
		} else {
			b.WriteString(strings.Repeat(" ", indent) + yamlScalar(v))
		}
	}
}

// yamlItem writes a sequence item's value after the "-" token already emitted.
func yamlItem(b *strings.Builder, v any, indent int) {
	if pairs, ok := yamlPairs(v); ok {
		first := true
		for _, p := range pairs {
			k := fmt.Sprint(p[0])
			if first {
				b.WriteString(" " + k + ":")
				yamlVal(b, p[1], indent)
				first = false
			} else {
				b.WriteString("\n" + strings.Repeat(" ", indent+2) + k + ":")
				yamlVal(b, p[1], indent+2)
			}
		}
		return
	}
	if arr, ok := v.([]any); ok {
		b.WriteString("\n")
		for _, item := range arr {
			b.WriteString(strings.Repeat(" ", indent+2) + "-")
			yamlItem(b, item, indent+2)
			b.WriteString("\n")
		}
		return
	}
	b.WriteString(" " + yamlScalar(v))
}

// yamlVal writes the value after "key:" already emitted (handles nesting).
func yamlVal(b *strings.Builder, v any, indent int) {
	if pairs, ok := yamlPairs(v); ok {
		b.WriteString("\n")
		for _, p := range pairs {
			b.WriteString(strings.Repeat(" ", indent+2) + fmt.Sprint(p[0]) + ":")
			yamlVal(b, p[1], indent+2)
			b.WriteString("\n")
		}
		return
	}
	if arr, ok := v.([]any); ok {
		b.WriteString("\n")
		for _, item := range arr {
			b.WriteString(strings.Repeat(" ", indent+2) + "-")
			yamlItem(b, item, indent+2)
			b.WriteString("\n")
		}
		return
	}
	b.WriteString(" " + yamlScalar(v))
}

// yamlPairs yields key/value pairs in order: *jsrtObj preserves insertion
// order (matching js-yaml over a JS object literal); a plain map is sorted
// for determinism.
func yamlPairs(v any) ([][2]any, bool) {
	switch x := v.(type) {
	case *jsrtObj:
		out := make([][2]any, 0, len(*x))
		for _, e := range *x {
			out = append(out, [2]any{e.K, e.V})
		}
		return out, true
	case map[string]any:
		ks := make([]string, 0, len(x))
		for k := range x {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		out := make([][2]any, 0, len(ks))
		for _, k := range ks {
			out = append(out, [2]any{k, x[k]})
		}
		return out, true
	}
	return nil, false
}

func main() {
	pass, fail := 0, 0
	for _, c := range oracleCases {
		if c.pass {
			pass++
		} else {
			fail++
			fmt.Printf("  FAIL %s: %s\n", c.name, c.err)
		}
	}
	fmt.Printf("ORACLE GO: %d pass, %d fail\n", pass, fail)
}
`

// driverJS is the Node-side baseline driver: mocha-style describe/it
// globals, runs each collected case, prints the JS parity number.
const driverJS = `global.results = [];
// Mini mocha: describe/it with per-scope hook inheritance (outer beforeEach
// applies to nested its), running beforeEach -> body -> afterEach per case.
const beforeStack = [[]], afterStack = [[]];
global.beforeEach = (fn) => beforeStack[beforeStack.length-1].push(fn);
global.afterEach = (fn) => afterStack[afterStack.length-1].push(fn);
global.describe = (name, fn) => {
  beforeStack.push(beforeStack[beforeStack.length-1].slice());
  afterStack.push(afterStack[afterStack.length-1].slice());
  fn();
  beforeStack.pop(); afterStack.pop();
};
global.it = (name, fn) => global.results.push({
  name, fn,
  before: beforeStack[beforeStack.length-1].slice(),
  after: afterStack[afterStack.length-1].slice()
});
require(process.argv[2]);
let pass = 0, fail = 0;
for (const r of global.results) {
  try {
    for (const h of r.before) h();
    r.fn();
    pass++;
  } catch (e) {
    fail++;
    console.log("  FAIL " + r.name + ": " + e.message);
  } finally {
    for (const h of r.after) { try { h(); } catch (e2) {} }
  }
}
console.log("ORACLE JS: " + pass + " pass, " + fail + " fail");
`

// stubChai is a minimal chai replacement so the untouched ESLint test file
// can load without node_modules. Only the assert methods the oracle maps.
const stubChai = `function fail(op, got, want) {
  throw new Error(op + ": got " + JSON.stringify(got) + " want " + JSON.stringify(want));
}
function deepEq(a, b) { return JSON.stringify(a) === JSON.stringify(b); }
module.exports = {
  assert: {
    strictEqual: (got, want) => { if (got !== want) fail("strictEqual", got, want); },
    equal: (got, want) => { if (got != want) fail("equal", got, want); },
    deepEqual: (got, want) => { if (!deepEq(got, want)) fail("deepEqual", got, want); },
    deepStrictEqual: (got, want) => { if (!deepEq(got, want)) fail("deepStrictEqual", got, want); },
    isTrue: (v) => { if (v !== true) fail("isTrue", v, true); },
    isFalse: (v) => { if (v !== false) fail("isFalse", v, false); },
    ok: (v) => { if (!v) fail("ok", v, true); },
    include: (hay, needle) => { if (!String(hay).includes(String(needle))) fail("include", hay, needle); },
    notInclude: (hay, needle) => { if (String(hay).includes(String(needle))) fail("notInclude", hay, needle); },
  },
};
`
