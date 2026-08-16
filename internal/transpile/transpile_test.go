package transpile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// transpileFile runs the full pipeline on a TS file.
func transpileFile(t *testing.T, path string) string {
	t.Helper()
	code, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	sf := p.CreateSourceFile("/"+path, string(code))
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// compileGo verifies generated code compiles by writing it into a temp module.
func compileGo(t *testing.T, name, src string) {
	t.Helper()
	dir := t.TempDir()
	// Compile as a library package: `package main` requires a main() func
	// that a transpiled module won't have.
	src = strings.Replace(src, "package main", "package ts2gotest", 1)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	gomod := "module ts2gotest\n\ngo 1.26.5\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go does not compile:\n%s\n--- generated source ---\n%s", out, src)
	}
}

func TestBankEndToEnd(t *testing.T) {
	out := transpileFile(t, "../../testdata/bank.ts")

	for _, want := range []string{
		"type Money = int64",
		"type Account struct {",
		"Balance int64",
		"type AccountKind int",
		"AccountKindChecking",
		"func NewBank(name string) *Bank {",
		"r.Name = name",
		"func (r *Bank) total() int64 {",
		"for _, a := range r.Accounts {",
		"sum += a.Balance",
		"if a > b {",
		"func max(a float64, b float64) float64 {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// The append statement must be an assignment.
	if !strings.Contains(out, "= append(r.Accounts, acc)") {
		t.Errorf("push should emit append assignment, got:\n%s", out)
	}
	// Builtin aliases must NOT be re-declared (recursive in Go).
	if strings.Contains(out, "type uint8") || strings.Contains(out, "type int64") {
		t.Errorf("builtin aliases must be skipped, got:\n%s", out)
	}
	compileGo(t, "bank.go", out)
}

func TestAliasConvention(t *testing.T) {
	out := transpileFile(t, "../../testdata/bank.ts")
	// The core trick: `type uint8 = number` makes `uint8` resolve to Go's
	// builtin uint8 — visible via field types (Balance: int64 from the
	// `type Money = int64` alias) and via the absence of a float64 mapping.
	if strings.Contains(out, "type uint8 = float64") || strings.Contains(out, "type int64 = float64") {
		t.Errorf("builtin alias fell back to float64:\n%s", out)
	}
	if !strings.Contains(out, "type Money = int64") {
		t.Errorf("Money alias should resolve through the convention:\n%s", out)
	}
}

func TestBanList(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/banned.ts", `
class Foo {
  method() {
    Object.setPrototypeOf(this, Foo.prototype);
    return eval("1+1");
  }
}
`)
	tp := NewTranspiler(p, sf)
	_, err := tp.Transpile()
	if err == nil {
		t.Fatal("expected ban error for prototype manipulation")
	}
	if !strings.Contains(err.Error(), "banned") {
		t.Fatalf("expected banned-construct error, got: %v", err)
	}
}

func TestSimpleFunction(t *testing.T) {
	p, _ := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	sf := p.CreateSourceFile("/f.ts", `
function add(a: number, b: number): number {
  return a + b;
}
`)
	tp := NewTranspiler(p, sf)
	out, err := tp.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "func add(a float64, b float64) float64 {") {
		t.Errorf("unexpected function signature:\n%s", out)
	}
	if !strings.Contains(out, "return a + b") {
		t.Errorf("unexpected body:\n%s", out)
	}
	compileGo(t, "f.go", out)
}
