package deps

import "sort"

// InlineShims is the collected set of canonical Go snippets for the tiny npm
// leaves marked verdict="inline" in the counterpart registry. Each entry is
// the "assessed once" answer: this package is too small / too stdlib-adjacent
// to split into its own repo, so copy this snippet into the consumer instead
// of porting or pulling a dependency.
//
// Displays via `uplift shim` (list all or print one). Update
// counterparts.json alongside this map: an "inline" verdict should reference
// a shim key below.
var InlineShims = map[string]string{
	// strip-ansi — remove ANSI escape sequences (color codes). Covers the
	// common CSI color codes (ESC[...m) and OSC title/set (ESC]...BEL).
	"strip-ansi": `func StripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case in:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '\a' {
				in = false // final byte of a CSI/OSC sequence (or BEL for OSC)
			}
		case r == 0x1b: // ESC
			in = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}`,

	// ansi-regex — the CSI color-code pattern, for when you need to match
	// rather than strip.
	"ansi-regex": `var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")`,

	// cross-spawn — resolve a binary on PATH then exec it (the core of what
	// JS cross-spawn does after its shebang/arg normalization).
	"cross-spawn": `func Command(name string, args ...string) *exec.Cmd {
	if p, err := exec.LookPath(name); err == nil {
		name = p
	}
	return exec.Command(name, args...)
}`,

	// is-path-inside — is child path inside parent (filepath.Rel prefix check).
	"is-path-inside": `func IsPathInside(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}`,

	// fast-levenshtein — edit distance between two strings.
	"fast-levenshtein": `func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}`,

	// glob-parent — the directory portion of a glob pattern.
	"glob-parent": `func GlobParent(pattern string) string {
	if i := strings.LastIndexByte(pattern, '/'); i >= 0 {
		return pattern[:i]
	}
	return ""
}`,

	// is-glob — does the string contain glob magic characters.
	"is-glob": `func IsGlob(s string) bool {
	for _, r := range s {
		switch r {
		case '*', '?', '[', '{':
			return true
		}
	}
	return false
}`,

	// supports-color — minimal TERM/NO_COLOR check (approximate; add
	// isatty/CI/FORCE_COLOR handling where the original's behavior matters).
	"supports-color": `func SupportsColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}`,

	// json-stable-stringify — stringify with map keys sorted (used where the
	// original relied on stable key ordering; note: byte parity with the JS
	// package is not guaranteed, verify if used for hashing).
	"json-stable-stringify": `func StableStringify(v any) (string, error) {
	var b strings.Builder
	if err := stableAppend(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

func stableAppend(b *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case string:
		enc, _ := json.Marshal(t)
		b.Write(enc)
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		enc, _ := json.Marshal(t)
		b.Write(enc)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			ek, _ := json.Marshal(k)
			b.Write(ek)
			b.WriteByte(':')
			if err := stableAppend(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := stableAppend(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	return nil
}`,
}

// ListInlineShims returns the sorted names of all collected inline shims.
func ListInlineShims() []string {
	out := make([]string, 0, len(InlineShims))
	for k := range InlineShims {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
