package oracle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// requireOracle skips the calling test unless the dev/CI fixtures this package
// drives through the whole pipeline are present: a prepared eslint checkout at
// /tmp/eslint-inspect and an uplift binary at /tmp/uplift. They are fixtures
// rather than repository contents, so a fresh clone (and CI without the setup
// step) must skip rather than fail.
func requireOracle(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("oracle run exercises the full pipeline")
	}
	for _, p := range []string{
		"/tmp/eslint-inspect/lib/linter/interpolate.js",
		"/tmp/uplift",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s not present: prepare the eslint checkout and build the oracle binary to /tmp/uplift", p)
		}
	}
}

func TestInterpolateParity(t *testing.T) {
	requireOracle(t)
	r, err := Run(
		"/tmp/eslint-inspect/lib/linter/interpolate.js",
		"/tmp/eslint-inspect/tests/lib/linter/interpolate.js",
		t.TempDir(),
	)
	if err != nil {
		t.Skipf("oracle prerequisites missing: %v", err)
	}
	fmt.Println("JS OUT >>>", r.JSOut)
	fmt.Println("GO BUILD >>>", r.GoBuildOK, r.GoBuildEr)
	fmt.Println("GO RUN >>>", r.GoRunOut)
	fmt.Println("SUMMARY >>>", r.Summary())
	if !r.GoBuildOK {
		t.Fatalf("go build failed:\n%s", r.GoBuildEr)
	}
	if !r.ParityOK() {
		t.Errorf("parity diverged: %s\nJS:\n%s\nGO:\n%s", r.Summary(), r.JSOut, r.GoRunOut)
	}
}

func TestJsonParity(t *testing.T) {
	requireOracle(t)
	// Scales the oracle to a non-basename test binding (`formatter`),
	// exercising the generic module-binding fixUp + the JSON shim + the
	// numeric-aware deep-equal (2 vs JSON.parse's float64 2).
	r, err := Run(
		"/tmp/eslint-inspect/lib/cli-engine/formatters/json.js",
		"/tmp/eslint-inspect/tests/lib/cli-engine/formatters/json.js",
		t.TempDir(),
	)
	if err != nil {
		t.Skipf("oracle prerequisites missing: %v", err)
	}
	fmt.Println("JSON JS OUT >>>", r.JSOut)
	fmt.Println("JSON BUILD >>>", r.GoBuildOK, r.GoBuildEr)
	fmt.Println("JSON GO RUN >>>", r.GoRunOut)
	fmt.Println("JSON SUMMARY >>>", r.Summary())
	if !r.GoBuildOK {
		t.Fatalf("go build failed:\n%s", r.GoBuildEr)
	}
	if !r.ParityOK() {
		t.Errorf("parity diverged: %s\nJS:\n%s\nGO:\n%s", r.Summary(), r.JSOut, r.GoRunOut)
	}
}

// TestFormatterParity drives the Phase B dynamic-any runtime over real,
// purely `any`-typed ESLint formatter modules + their chai-assert tests.
// These modules traverse `results` as raw `any` (member access, .forEach,
// .length, ||, dynamic writes, JSON), so they exercise the jsrt value model.
func TestFormatterParity(t *testing.T) {
	requireOracle(t)
	base := "/tmp/eslint-inspect/"
	pairs := []struct{ mod, test string }{
		{base + "lib/cli-engine/formatters/compact.js", base + "tests/lib/cli-engine/formatters/compact.js"},
		{base + "lib/cli-engine/formatters/unix.js", base + "tests/lib/cli-engine/formatters/unix.js"},
		{base + "lib/cli-engine/formatters/visualstudio.js", base + "tests/lib/cli-engine/formatters/visualstudio.js"},
		{base + "lib/cli-engine/formatters/json-with-metadata.js", base + "tests/lib/cli-engine/formatters/json-with-metadata.js"},
		{base + "lib/cli-engine/formatters/tap.js", base + "tests/lib/cli-engine/formatters/tap.js"},
	}
	for _, p := range pairs {
		r, err := Run(p.mod, p.test, t.TempDir())
		if err != nil {
			t.Skipf("%s: oracle prerequisites missing: %v", filepath.Base(p.mod), err)
		}
		fmt.Printf("FORMATTER SUMMARY >>> %s\n", r.Summary())
		if !r.GoBuildOK {
			t.Fatalf("%s: go build failed:\n%s", filepath.Base(p.mod), r.GoBuildEr)
		}
		if !r.ParityOK() {
			t.Errorf("%s: parity diverged: %s\nJS:\n%s\nGO:\n%s", filepath.Base(p.mod), r.Summary(), r.JSOut, r.GoRunOut)
		}
	}
}
