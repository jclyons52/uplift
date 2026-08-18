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
	"reflect"
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

// jsrtDeepEqual is reflect.DeepEqual with JS numeric + deep-map/slice
// coercion: 2 == 2.0, and numbers nested in maps/slices compare numerically.
func jsrtDeepEqual(a, b any) bool {
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

func jsrtStringify(v any) string {
	b, err := json.Marshal(v)
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
global.describe = (name, fn) => fn();
global.it = (name, fn) => global.results.push({ name, fn });
require(process.argv[2]);
let pass = 0, fail = 0;
for (const r of global.results) {
  try { r.fn(); pass++; } catch (e) { fail++; console.log("  FAIL " + r.name + ": " + e.message); }
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
  },
};
`
