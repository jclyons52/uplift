package oracle

import (
	"fmt"
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
