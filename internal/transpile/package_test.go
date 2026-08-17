package transpile

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tsmorph "github.com/jclyons52/ts-go-morph"
)

// transpilePackage runs the multi-file pipeline on in-memory sources named
// by virtual path (e.g. "a.ts" lives at /src/a.ts, so "./b" resolves).
func transpilePackage(t *testing.T, files map[string]string) *PackageResult {
	t.Helper()
	p, err := tsmorph.NewProject(tsmorph.ProjectOptions{UseInMemoryFileSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var sfs []*tsmorph.SourceFile
	for _, name := range names {
		sfs = append(sfs, p.CreateSourceFile("/src/"+name, files[name]))
	}
	pkg := NewPackage(p, sfs, "ts2gotest")
	res, err := pkg.Transpile()
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// fileByName finds a generated file, failing the test if missing.
func fileByName(t *testing.T, res *PackageResult, name string) *FileResult {
	t.Helper()
	for _, f := range res.Files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("generated file %s not found (have: %v)", name, fileNames(res))
	return nil
}

func fileNames(res *PackageResult) []string {
	var names []string
	for _, f := range res.Files {
		names = append(names, f.Name)
	}
	return names
}

// compilePackageGo writes every generated file into a temp module and
// builds it, proving the package is consistent Go.
func compilePackageGo(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module ts2gotest\n\ngo 1.26.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated package does not compile:\n%s", out)
	}
}

func TestPackageTwoFiles(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `import { greet } from "./b";
export const msg: string = greet("bob");`,
		"b.ts": `export function greet(s: string): string {
  return "hi " + s;
}`,
	})

	a := fileByName(t, res, "a.go")
	// The same-package import is dropped; the call survives.
	if strings.Contains(a.Code, `"./b"`) || strings.Contains(a.Code, "import") {
		t.Errorf("same-package import should be dropped:\n%s", a.Code)
	}
	if !strings.Contains(a.Code, `greet("bob")`) {
		t.Errorf("cross-file call missing:\n%s", a.Code)
	}
	// Cross-file symbol must not be flagged as an unknown import.
	if len(a.Report.Items) > 0 {
		t.Errorf("expected no work items for a.go, got %v", a.Report.Items)
	}

	b := fileByName(t, res, "b.go")
	compilePackageGo(t, map[string]string{"a.go": a.Code, "b.go": b.Code})
}

func TestPackageExternalImportGap(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `import fs from "fs";
export const p: string = fs.readFileSync("/x");`,
		"b.ts": `export const n: number = 1;`,
	})

	a := fileByName(t, res, "a.go")
	found := false
	for _, it := range a.Report.Items {
		if it.Severity == SevTodo && it.Category == "import" && strings.Contains(it.Message, "fs") {
			found = true
		}
	}
	if !found {
		t.Errorf("external import should be a work item, got %v", a.Report.Items)
	}
	// No Go import block entry for the external module (the manifest
	// comment legitimately quotes the TS snippet).
	if strings.Contains(a.Code, "	\"fs\"") {
		t.Errorf("import line should not be emitted:\n%s", a.Code)
	}
	// The typed var with a placeholder initializer must still compile
	// (resilience contract: placeholders never break the build).
	compilePackageGo(t, map[string]string{
		"a.go": a.Code,
		"b.go": fileByName(t, res, "b.go").Code,
	})
}

func TestPackageShimEmittedOnce(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `async function greet(name: string): Promise<string> {
  return "hi " + name;
}`,
		"b.ts": `export const n: number = 1;`,
	})

	count := 0
	for _, f := range res.Files {
		if f.Name == "jsrt.go" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("jsrt.go should be emitted exactly once, got %d (%v)", count, fileNames(res))
	}
	a := fileByName(t, res, "a.go")
	if !strings.Contains(a.Code, "jsrtAsync") {
		t.Errorf("async file should reference the shim:\n%s", a.Code)
	}
	if strings.Contains(a.Code, "type jsrtPromise") {
		t.Errorf("shim definitions must not be duplicated into a.go:\n%s", a.Code)
	}
	compilePackageGo(t, map[string]string{
		"a.go":    a.Code,
		"b.go":    fileByName(t, res, "b.go").Code,
		"jsrt.go": fileByName(t, res, "jsrt.go").Code,
	})
}

func TestPackageSameDirModule(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `import { x } from "./util";
export const v: number = x;`,
		"util.ts": `export const x: number = 42;`,
	})
	a := fileByName(t, res, "a.go")
	if len(a.Report.Items) > 0 {
		t.Errorf("same-dir extension-less import should resolve, got %v", a.Report.Items)
	}
	compilePackageGo(t, map[string]string{
		"a.go":    a.Code,
		"util.go": fileByName(t, res, "util.go").Code,
	})
}

func TestPackageSiblingPackageGap(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `import { x } from "./lib";
export const v: number = x;`,
		"lib/index.ts": `export const x: number = 42;`,
	})
	a := fileByName(t, res, "a.go")
	found := false
	for _, it := range a.Report.Items {
		if it.Category == "import" && strings.Contains(it.Message, "wire up") && strings.Contains(it.Message, "package lib") {
			found = true
		}
	}
	if !found {
		t.Errorf("sibling-package import should be a wire-up work item, got %v", a.Report.Items)
	}
	// The subdirectory file becomes its own package.
	idx := fileByName(t, res, "index.go")
	if idx.RelDir != "lib" || !strings.Contains(idx.Code, "package lib") {
		t.Errorf("lib/index.ts should be package lib at lib/index.go (RelDir=%q):\n%s", idx.RelDir, idx.Code)
	}
}

func TestPackageBareExport(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `export const n: number = 1;
export { n };`,
	})
	a := fileByName(t, res, "a.go")
	if len(a.Report.Items) > 0 {
		t.Errorf("bare export should be a no-op, got %v", a.Report.Items)
	}
}

func TestPackageReportAggregation(t *testing.T) {
	res := transpilePackage(t, map[string]string{
		"a.ts": `import fs from "fs";
export const n: number = 1;`,
		"b.ts": `export const m: number = 2;`,
	})
	r := res.Report
	if len(r.Files) != 2 {
		t.Fatalf("aggregate report should hold 2 per-file reports, got %d", len(r.Files))
	}
	if r.Score() != 3 {
		t.Errorf("aggregate score should be 3 (one todo item), got %d", r.Score())
	}
	sev := r.BySeverity()
	if sev[SevTodo] != 1 {
		t.Errorf("aggregate should count 1 todo item, got %v", sev)
	}
	if !strings.Contains(r.String(), "a.ts") {
		t.Errorf("package report should mention the file:\n%s", r.String())
	}
}
