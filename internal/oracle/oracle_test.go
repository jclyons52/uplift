package oracle

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestInterpolateParity(t *testing.T) {
	if testing.Short() {
		t.Skip("oracle run exercises the full pipeline")
	}
	r, err := Run(
		"/tmp/eslint-inspect/lib/linter/interpolate.js",
		"/tmp/eslint-inspect/tests/lib/linter/interpolate.js",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("run: %v", err)
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
	if testing.Short() {
		t.Skip("oracle run exercises the full pipeline")
	}
	// Scales the oracle to a non-basename test binding (`formatter`),
	// exercising the generic module-binding fixUp + the JSON shim + the
	// numeric-aware deep-equal (2 vs JSON.parse's float64 2).
	r, err := Run(
		"/tmp/eslint-inspect/lib/cli-engine/formatters/json.js",
		"/tmp/eslint-inspect/tests/lib/cli-engine/formatters/json.js",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("run: %v", err)
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
	if testing.Short() {
		t.Skip("oracle run exercises the full pipeline")
	}
	base := "/tmp/eslint-inspect/"
	pairs := []struct{ mod, test string }{
		{base + "lib/cli-engine/formatters/compact.js", base + "tests/lib/cli-engine/formatters/compact.js"},
		{base + "lib/cli-engine/formatters/unix.js", base + "tests/lib/cli-engine/formatters/unix.js"},
		{base + "lib/cli-engine/formatters/visualstudio.js", base + "tests/lib/cli-engine/formatters/visualstudio.js"},
		{base + "lib/cli-engine/formatters/json-with-metadata.js", base + "tests/lib/cli-engine/formatters/json-with-metadata.js"},
	}
	for _, p := range pairs {
		r, err := Run(p.mod, p.test, t.TempDir())
		if err != nil {
			t.Fatalf("%s: run: %v", filepath.Base(p.mod), err)
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
