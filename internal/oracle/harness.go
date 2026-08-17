package oracle

// harnessGo is the Go-side test harness written next to the transpiled
// module+test. It provides the describe/it/assert scaffolding the transpiled
// test references (after the fix-up pass) and prints the parity number.
const harnessGo = `package main

import (
	"fmt"
	"reflect"
)

type oracleCase struct {
	name string
	pass bool
	err  string
}

var oracleCases []oracleCase

func oracleDescribe(_ string, fn func()) { fn() }

func oracleIt(name string, fn func()) {
	c := oracleCase{name: name}
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.pass, c.err = false, fmt.Sprint(r)
			}
		}()
		fn()
		c.pass = true
	}()
	oracleCases = append(oracleCases, c)
}

func assertStrictEqual(got, want any) {
	if !reflect.DeepEqual(got, want) {
		panic(fmt.Sprintf("strictEqual: got %#v want %#v", got, want))
	}
}

func assertEqual(got, want any) { assertStrictEqual(got, want) }

func assertDeepEqual(got, want any) { assertStrictEqual(got, want) }

func assertIsTrue(v any) {
	if b, ok := v.(bool); !ok || !b {
		panic(fmt.Sprintf("isTrue: got %#v", v))
	}
}

func assertIsFalse(v any) {
	if b, ok := v.(bool); !ok || b {
		panic(fmt.Sprintf("isFalse: got %#v", v))
	}
}

func assertOK(v any) {
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
    isTrue: (v) => { if (v !== true) fail("isTrue", v, true); },
    isFalse: (v) => { if (v !== false) fail("isFalse", v, false); },
    ok: (v) => { if (!v) fail("ok", v, true); },
  },
};
`
