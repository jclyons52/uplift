package scaffold

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Write materializes each repo plan under opts.Out. It returns the list of
// repo directories actually created.
func Write(repos []Repo, opts Options) ([]string, error) {
	var created []string
	for _, r := range repos {
		dir := filepath.Join(opts.Out, r.Dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return created, err
		}
		if err := writeFile(filepath.Join(dir, "go.mod"), goMod(r)); err != nil {
			return created, err
		}
		if err := writeFile(filepath.Join(dir, r.Package+".go"), pkgStub(r)); err != nil {
			return created, err
		}
		if err := writeFile(filepath.Join(dir, "parity_test.go"), parityHarness(r)); err != nil {
			return created, err
		}
		if err := writeFile(filepath.Join(dir, "README.md"), r.Describe()); err != nil {
			return created, err
		}
		if opts.Vendor && r.SourceDir != "" {
			if st, err := os.Stat(r.SourceDir); err == nil && st.IsDir() {
				if err := copyTree(r.SourceDir, filepath.Join(dir, "original")); err != nil {
					return created, err
				}
			}
		}
		created = append(created, dir)
	}

	// A backlog index tying the repos together.
	if len(repos) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "# Uplift port backlog\n\nRepos scaffolded from the dependency/leaf-node analysis. Each is a leaf\nlibrary in its own repo — transpile `original/` with ts2go, then clean.\n\n")
		for _, r := range repos {
			fmt.Fprintf(&sb, "- **%s** (`%s`) — %d LOC, API: %s\n",
				r.Name, r.Module, r.Loc, apiSummary(r.Exports))
		}
		if err := writeFile(filepath.Join(opts.Out, "PORTS.md"), sb.String()); err != nil {
			return created, err
		}
	}
	return created, nil
}

func goMod(r Repo) string {
	return fmt.Sprintf("module %s\n\ngo 1.26.5\n", r.Module)
}

func pkgStub(r Repo) string {
	return fmt.Sprintf(`// Package %s is the Go port of the npm package %q.
// Scaffolded by the uplift port toolchain — transpile original/ with ts2go,
// then hand-clean and grow the API surface.
package %s

// Placeholder marks the start of the port.
const Placeholder = "TODO: transpile original via ts2go"
`, r.Package, r.Name, r.Package)
}

func apiSummary(exports []string) string {
	if len(exports) == 0 {
		return "(function/other entry — inspect original/)"
	}
	return strings.Join(exports, ", ")
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// copyTree copies a node_modules package source into dst, skipping any
// nested node_modules and heavy noise (package-lock).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if d.Name() == "package-lock.json" {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, in)
		oerr := out.Close()
		if cerr != nil {
			return cerr
		}
		return oerr
	})
}
