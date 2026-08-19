// Package bench benchmarks a pure function in both JS (node) and Go (a
// ported repo) across the same inputs, reporting wall time, memory, and
// ops/sec so the "performance / resource consumption" uplift goal is
// measurable. It targets the common leaf-lib shape: a single-argument
// (string or int) pure function. Cross-language value equality is the parity
// gate's job, not this tool's — here the result is just kept alive to defeat
// dead-code elimination.
package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options describe a benchmark run.
type Options struct {
	Name       string
	Iterations int
	Inputs     []string
	ArgType    string // "string" | "int"
	JSModule   string // abs path to a JS module exporting Func
	JSFunc     string
	GoImport   string // import path, e.g. github.com/jclyons52/esutils-go
	GoDir      string // abs dir of the Go module
	GoFunc     string // exported function name
}

// Result is one side's measurement.
type Result struct {
	WallMs   float64 `json:"wallMs"`
	MemMB    float64 `json:"memMB"`
	OpsPerS  float64 `json:"opsPerSec"`
	Desc     string  `json:"desc"`
	Iterated int64   `json:"iterations"`
}

type benchResult struct {
	WallMs float64 `json:"wallMs"`
	MemMB  float64 `json:"memMB"`
}

func (o Options) total() int64 { return int64(o.Iterations) * int64(len(o.Inputs)) }

func callJS(o Options) string {
	arg := "x"
	if o.ArgType == "int" {
		arg = "parseInt(x, 10)"
	}
	return fmt.Sprintf("sink = m[%q](%s);", o.JSFunc, arg)
}

// runJS measures node execution of the function.
func (o Options) runJS(dir string) (Result, error) {
	inputs, _ := json.Marshal(o.Inputs)
	driver := fmt.Sprintf(`'use strict';
const m = require(%q);
const inputs = %s;
const iters = %d;
let sink;
function call(x) { %s }
const t0 = process.hrtime.bigint();
for (let i = 0; i < iters; i++) {
  for (const x of inputs) { call(x); }
}
const wall = Number(process.hrtime.bigint() - t0) / 1e6;
const rss = process.memoryUsage().rss / 1048576;
process.stdout.write(JSON.stringify({wallMs: wall, memMB: rss}));
`, o.JSModule, string(inputs), o.Iterations, callJS(o))
	if err := os.WriteFile(filepath.Join(dir, "bench.js"), []byte(driver), 0o644); err != nil {
		return Result{}, err
	}
	cmd := exec.Command("node", "bench.js")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("node: %w\n%s", err, out)
	}
	var br benchResult
	json.Unmarshal(out, &br)
	tot := o.total()
	ops := 0.0
	if br.WallMs > 0 {
		ops = float64(tot) / (br.WallMs / 1e3)
	}
	return Result{WallMs: br.WallMs, MemMB: br.MemMB, Desc: "node " + o.JSFunc, Iterated: tot, OpsPerS: ops}, nil
}

// runGo measures Go by generating a temp module that imports the ported repo
// (via replace) and runs the same loop.
func (o Options) runGo(dir string) (Result, error) {
	quoted := make([]string, len(o.Inputs))
	for i, x := range o.Inputs {
		quoted[i] = fmt.Sprintf("%q", x)
	}
	goInputs := "{" + strings.Join(quoted, ",") + "}"
	// Emit ONLY the matching argument branch (Go compiles both even though
	// only one runs, so a dead mismatched-signature branch won't compile).
	body := fmt.Sprintf("sink = ported.%s(x)", o.GoFunc)
	if o.ArgType == "int" {
		body = "vi, _ := strconv.ParseInt(x, 10, 64)\n				sink = ported." + o.GoFunc + "(int(vi))"
	}
	alias := "ported"
	main := fmt.Sprintf(`package main

import (
	"runtime"
	"strconv"
	"time"

	%s "%s"
)

func main() {
	inputs := []string%s
	iters := %d
	var sink interface{}
	t0 := time.Now()
	for i := 0; i < iters; i++ {
		for _, x := range inputs {
			%s
		}
	}
	wallMs := time.Since(t0).Milliseconds()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	println("WALL=" + strconv.FormatInt(wallMs, 10))
	println("MEM=" + strconv.FormatFloat(float64(m.TotalAlloc)/1048576, 'f', 1, 64))
	_ = sink
}
`,
		alias, o.GoImport, goInputs, o.Iterations, body)

	goMod := fmt.Sprintf("module bench\n\ngo 1.26\n\nrequire %s v0.0.0\n\nreplace %s => %s\n", o.GoImport, o.GoImport, o.GoDir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		return Result{}, err
	}
	cmd := exec.Command("go", "run", "-mod=mod", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("go: %w\n%s", err, out)
	}
	wallMs, memMB := 0.0, 0.0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "WALL=") {
			fmt.Sscanf(strings.TrimPrefix(line, "WALL="), "%f", &wallMs)
		}
		if strings.HasPrefix(line, "MEM=") {
			fmt.Sscanf(strings.TrimPrefix(line, "MEM="), "%f", &memMB)
		}
	}
	tot := o.total()
	ops := 0.0
	if wallMs > 0 {
		ops = float64(tot) / (wallMs / 1e3)
	}
	return Result{WallMs: wallMs, MemMB: memMB, Desc: "go " + o.GoFunc, Iterated: tot, OpsPerS: ops}, nil
}

// Run executes both sides and returns the two results.
func Run(o Options) (js, gores Result, err error) {
	dir, err := os.MkdirTemp("", "bench-")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)
	js, err = o.runJS(dir)
	if err != nil {
		return
	}
	gores, err = o.runGo(dir)
	return
}

// Render formats the comparison.
func Render(o Options, js, gores Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Bench: %s (%d iters x %d inputs)  arg:%s\n",
		o.Name, o.Iterations, len(o.Inputs), o.ArgType)
	fmt.Fprintf(&b, "  %-22s %12s %10s %14s\n", "side", "wall(ms)", "mem(MB)", "ops/sec")
	for _, r := range []Result{js, gores} {
		fmt.Fprintf(&b, "  %-22s %12.1f %10.1f %14.0f\n", r.Desc, r.WallMs, r.MemMB, r.OpsPerS)
	}
	if js.OpsPerS > 0 {
		fmt.Fprintf(&b, "  Go speedup: %.2fx\n", gores.OpsPerS/js.OpsPerS)
	}
	return b.String()
}
