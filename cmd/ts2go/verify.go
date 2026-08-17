package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jclyons52/ts2go/internal/transpile"
)

// verifyBuild closes the driver loop: the generated files are copied into a
// temp module (package main rewritten to a neutral name so library output
// type-checks), `go build ./...` runs, and every compiler failure is
// returned as a CompileError. Nothing is written to the real output; the
// temp module is discarded.
func verifyBuild(files []*transpile.FileResult) []transpile.CompileError {
	dir, err := os.MkdirTemp("", "ts2go-verify")
	if err != nil {
		return []transpile.CompileError{{Message: "verify: " + err.Error()}}
	}
	defer os.RemoveAll(dir)

	for _, f := range files {
		code := rewritePackageMain(f.Code)
		p := filepath.Join(dir, f.RelDir, f.Name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return []transpile.CompileError{{Message: "verify: " + err.Error()}}
		}
		if err := os.WriteFile(p, []byte(code), 0o644); err != nil {
			return []transpile.CompileError{{Message: "verify: " + err.Error()}}
		}
	}
	goVer := goVersion()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module ts2go_verify\n\ngo "+goVer+"\n"), 0o644); err != nil {
		return []transpile.CompileError{{Message: "verify: " + err.Error()}}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, _ := cmd.CombinedOutput()
	return parseGoErrors(string(out))
}

// goVersion returns the installed Go release, e.g. "1.26.5". The generated
// code uses `any` and other go1.18+ features, so the verify module must
// declare a matching language version (a bare go.mod defaults to go1.16).
func goVersion() string {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return "1.18"
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "go")
}

// rewritePackageMain renames `package main` to a library package so
// transpiled output (which has no main()) type-checks in the verify build.
// Multi-package output is untouched: per-directory packages are already
// named after their directory.
func rewritePackageMain(code string) string {
	if !strings.Contains(code, "package main\n") {
		return code
	}
	return strings.Replace(code, "package main\n", "package ts2go_verify\n", 1)
}

// errLineRe matches go build error lines:
//
//	main.go:14:21: undefined: fmtPrice
//	main.go:14: undefined: fmtPrice       (some errors have no column)
//
// Package-header lines (# demo/lib), blank lines, and multi-line error
// continuations are ignored.
var errLineRe = regexp.MustCompile(`^([^:\n]+):(\d+)(?::(\d+))?:\s+(.+)$`)

// parseGoErrors converts `go build ./...` output into CompileErrors. Paths
// are relative to the build root ("./main.go" or "lib/format.go").
func parseGoErrors(buildOut string) []transpile.CompileError {
	var errs []transpile.CompileError
	for _, line := range strings.Split(buildOut, "\n") {
		m := errLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		e := transpile.CompileError{
			File:    strings.TrimPrefix(m[1], "./"),
			Message: m[4],
		}
		if n, err := strconv.Atoi(m[2]); err == nil {
			e.Line = n
		}
		if m[3] != "" {
			if n, err := strconv.Atoi(m[3]); err == nil {
				e.Col = n
			}
		}
		errs = append(errs, e)
	}
	return errs
}

// attachCompileErrors routes build findings to the FileResult they name
// (matched by relative path), appends a findings comment block to the
// generated code so the TODO travels with the file, and returns any errors
// that name no generated file (package-level issues).
func attachCompileErrors(files []*transpile.FileResult, errs []transpile.CompileError) []transpile.CompileError {
	var unmatched []transpile.CompileError
	for _, e := range errs {
		target := findFile(files, e.File)
		if target == nil {
			unmatched = append(unmatched, e)
			continue
		}
		if target.Report != nil {
			target.Report.CompileErrors = append(target.Report.CompileErrors, e)
		}
		appendFindings(&target.Code, e)
	}
	return unmatched
}

// findFile locates the FileResult at rel (relative to the output root).
func findFile(files []*transpile.FileResult, rel string) *transpile.FileResult {
	rel = filepath.ToSlash(filepath.Clean(rel))
	for _, f := range files {
		if filepath.ToSlash(filepath.Join(f.RelDir, f.Name)) == rel {
			return f
		}
	}
	return nil
}

// appendFindings adds one `go build` finding to the end of a generated
// file's source as a comment, so the compiler's own feedback travels with
// the code the LLM is about to fix.
func appendFindings(code *string, e transpile.CompileError) {
	loc := fmt.Sprintf("%s:%d:%d", e.File, e.Line, e.Col)
	if e.Col == 0 {
		loc = fmt.Sprintf("%s:%d", e.File, e.Line)
	}
	*code += fmt.Sprintf("\n// ts2go compile finding: %s: %s\n", loc, e.Message)
}
