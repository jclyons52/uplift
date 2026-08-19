package scaffold

import (
	"encoding/json"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// PortQueue is a machine-readable port work queue for the uplift harness.
// Persisted as JSON (schema port-queue/v1) so a plugin can consume it as
// clean structured input instead of parsing prose.
type PortQueue struct {
	Schema string       `json:"schema"`
	Repos  []QueueEntry `json:"repos"`
}

// QueueEntry is one library to port.
type QueueEntry struct {
	Name     string   `json:"name"`
	GoModule string   `json:"goModule"`
	Dir      string   `json:"dir"`
	Loc      int      `json:"loc"`
	Exports  []string `json:"exports"`
	Verdict  string   `json:"verdict"`
	Modules  []string `json:"modules"` // relative source files to port
	Units    []Unit   `json:"units"`   // per-export work items (resumable)
}

// Unit is one work item.
type Unit struct {
	Name   string `json:"name"`
	File   string `json:"file,omitempty"`
	Status string `json:"status"` // pending | in_progress | done
}

// PortQueueOf builds the queue from the repo plan.
func PortQueueOf(repos []Repo, opts Options) *PortQueue {
	pq := &PortQueue{Schema: "port-queue/v1"}
	for _, r := range repos {
		entry := QueueEntry{
			Name:     r.Name,
			GoModule: r.Module,
			Dir:      r.Dir,
			Loc:      r.Loc,
			Exports:  r.Exports,
			Modules:  listSourceModules(r.SourceDir),
		}
		entry.Verdict = "port"
		for _, name := range r.Exports {
			entry.Units = append(entry.Units, Unit{Name: name, Status: "pending"})
		}
		if len(entry.Units) == 0 {
			entry.Units = append(entry.Units, Unit{Name: "$entry", Status: "pending"})
		}
		pq.Repos = append(pq.Repos, entry)
	}
	return pq
}

// listSourceModules lists the JS/TS source files of a package, relative.
func listSourceModules(dir string) []string {
	if dir == "" {
		return nil
	}
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") || strings.Contains(base, ".min.") {
			return nil
		}
		switch filepath.Ext(p) {
		case ".js", ".mjs", ".cjs", ".ts", ".tsx":
			if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// WriteQueueJSON writes the port queue.
func WriteQueueJSON(w io.Writer, pq *PortQueue) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(pq)
}

// LoadQueueJSON reads a persisted queue (for resume).
func LoadQueueJSON(b []byte) (*PortQueue, error) {
	var pq PortQueue
	if err := json.Unmarshal(b, &pq); err != nil {
		return nil, err
	}
	return &pq, nil
}
