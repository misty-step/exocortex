// Package frontmatter parses, validates, and surgically edits
// markdown+YAML frontmatter notes. The splice functions preserve every
// byte outside the region they own: a provenance stamp must never
// reformat a note's existing frontmatter.
package fm

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Note is a raw file split into frontmatter and body.
type Note struct {
	Raw    []byte
	FMText string // between the --- delimiters, delimiters excluded
	Body   string // after the closing delimiter ("" if none)
	HasFM  bool
}

var (
	openDelim  = "---\n"
	closerRe   = regexp.MustCompile(`(?m)^---[ \t]*(\r?\n|$)`)
	topLevelRe = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):`)
)

// Split divides raw into frontmatter and body. Frontmatter is present
// only when the file starts with an opening --- delimiter line and a
// closing --- line exists at column 0 later in the file.
func Split(raw []byte) Note {
	n := Note{Raw: raw}
	s := string(raw)
	if !strings.HasPrefix(s, openDelim) {
		return n
	}
	rest := s[len(openDelim):]
	loc := closerRe.FindStringIndex(rest)
	if loc == nil {
		return n
	}
	n.HasFM = true
	n.FMText = strings.TrimSuffix(rest[:loc[0]], "\n")
	n.Body = rest[loc[1]:]
	return n
}

// Parse decodes the frontmatter text into a generic map. It returns an
// error when the text is not a YAML mapping.
func Parse(n Note) (map[string]any, error) {
	if !n.HasFM {
		return nil, errors.New("no frontmatter")
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(n.FMText), &m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("frontmatter is not a mapping")
	}
	return m, nil
}

// Finding is one lint observation.
type Finding struct {
	Level   string `json:"level"` // "error" | "warning"
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

func errf(rule, format string, a ...any) Finding {
	return Finding{Level: "error", Rule: rule, Message: fmt.Sprintf(format, a...)}
}
func warnf(rule, format string, a ...any) Finding {
	return Finding{Level: "warning", Rule: rule, Message: fmt.Sprintf(format, a...)}
}

// knownKeys are the /wiki floor keys; anything else warns under the
// daybook profile.
var knownKeys = map[string]bool{
	"type": true, "status": true, "created": true,
	"description": true, "tags": true, "provenance": true,
}

// Validate applies a named profile to a note. Errors are contract
// failures; warnings are advisory and never blocking under any profile.
func Validate(profile string, n Note) ([]Finding, error) {
	fmMap, perr := Parse(n)
	switch profile {
	case "daybook":
		if !n.HasFM {
			return nil, contract(errf("fm_missing", "frontmatter missing"))
		}
		if perr != nil {
			return nil, contract(errf("fm_unparseable", "frontmatter is not parseable YAML: %v", perr))
		}
		if t, ok := fmMap["type"].(string); !ok || strings.TrimSpace(t) == "" {
			return nil, contract(errf("type_missing", "frontmatter has no non-empty \"type\""))
		}
		return daybookWarnings(fmMap), nil
	case "strict":
		if !n.HasFM {
			return nil, contract(errf("fm_missing", "frontmatter missing"))
		}
		if perr != nil {
			return nil, contract(errf("fm_unparseable", "frontmatter is not parseable YAML: %v", perr))
		}
		var fs []Finding
		for _, k := range []string{"type", "status", "created", "description", "tags"} {
			if empty(fmMap[k]) {
				fs = append(fs, errf("key_missing", "strict profile requires non-empty %q", k))
			}
		}
		if c, ok := fmMap["created"].(string); ok && c != "" {
			if _, perr := time.Parse(time.RFC3339, c); perr != nil {
				fs = append(fs, errf("created_format", "created %q is not RFC3339", c))
			}
		}
		for _, f := range fs {
			if f.Level == "error" {
				return fs, contract(f)
			}
		}
		return fs, nil
	default:
		return nil, fmt.Errorf("unknown profile %q", profile)
	}
}

// contract wraps a finding as a validation error.
type contractError struct{ f Finding }

func contract(f Finding) error { return contractError{f} }

func (e contractError) Error() string { return e.f.Rule + ": " + e.f.Message }

// ContractFinding exposes the finding behind a validation error.
func ContractFinding(err error) (Finding, bool) {
	ce, ok := err.(contractError)
	return ce.f, ok
}

func daybookWarnings(m map[string]any) []Finding {
	var fs []Finding
	for _, k := range []string{"status", "description", "tags", "created"} {
		if empty(m[k]) {
			fs = append(fs, warnf("key_missing", "%q missing or empty", k))
		}
	}
	if c, ok := m["created"].(string); ok && c != "" {
		if _, perr := time.Parse(time.RFC3339, c); perr != nil {
			fs = append(fs, warnf("created_format", "created %q is not RFC3339", c))
		}
	}
	var unknown []string
	for k := range m {
		if !knownKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		fs = append(fs, warnf("unknown_keys", "unknown frontmatter keys: %s", strings.Join(unknown, ", ")))
	}
	return fs
}

func empty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	default:
		return false
	}
}

// Provenance is the kernel's write stamp.
type Provenance struct {
	Agent string
	At    time.Time
	Via   string // "cli" | "mcp"
}

// SpliceProvenance inserts or replaces the top-level provenance block in
// the note's frontmatter, leaving every other byte of the file
// untouched. It returns the new file bytes.
func SpliceProvenance(raw []byte, p Provenance) []byte {
	n := Split(raw)
	if !n.HasFM {
		// Caller validated before stamping; a note without frontmatter
		// cannot carry provenance, so it is returned unchanged.
		return raw
	}
	block := provenanceBlock(p)
	spanStart, spanEnd, spanOK := findKeySpan(n.FMText, "provenance")
	var fmText string
	if spanOK {
		fmText = n.FMText[:spanStart] + block + n.FMText[spanEnd:]
	} else {
		fmText = n.FMText + "\n" + block
	}
	// Split trims the newline before the closer; recompose with exactly
	// one so the closing delimiter never fuses with the last value line.
	head := openDelim + strings.TrimRight(fmText, "\n") + "\n---\n"
	return append([]byte(head), n.Body...)
}

// StripProvenance removes the kernel-owned provenance block from a
// note's frontmatter, leaving all other bytes untouched. It is the
// identity for notes without frontmatter or without a provenance block.
func StripProvenance(raw []byte) []byte {
	n := Split(raw)
	if !n.HasFM {
		return raw
	}
	spanStart, spanEnd, spanOK := findKeySpan(n.FMText, "provenance")
	if !spanOK {
		return raw
	}
	fmText := n.FMText[:spanStart] + n.FMText[spanEnd:]
	head := openDelim + strings.TrimRight(fmText, "\n") + "\n---\n"
	return append([]byte(head), n.Body...)
}

// findKeySpan locates the byte span of a top-level key's value block:
// from the key line to just before the next top-level key line (or the
// end of the frontmatter text).
func findKeySpan(fm, key string) (int, int, bool) {
	start := -1 // plain local: named returns would zero-initialize to a
	// bogus match-at-0 and corrupt the splice.
	offset := 0
	for _, ln := range strings.SplitAfter(fm, "\n") {
		if start >= 0 {
			if topKeyName(ln) != "" {
				return start, offset, true
			}
		} else if topKeyName(ln) == key {
			start = offset
		}
		offset += len(ln)
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, len(fm), true
}

func topKeyName(line string) string {
	m := topLevelRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[1]
}

func provenanceBlock(p Provenance) string {
	var b strings.Builder
	b.WriteString("provenance:\n")
	b.WriteString("  agent: " + yamlScalar(p.Agent) + "\n")
	b.WriteString("  at: " + p.At.UTC().Format(time.RFC3339) + "\n")
	b.WriteString("  via: " + yamlScalar(p.Via) + "\n")
	return b.String()
}

// yamlScalar renders s as a plain YAML scalar, double-quoted only when
// the value would otherwise parse as something else.
func yamlScalar(s string) string {
	if s == "" || strings.ContainsAny(s, ":#{}[]&*!|>'\"%@` \t") ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "?") ||
		strings.EqualFold(s, "true") || strings.EqualFold(s, "false") ||
		strings.EqualFold(s, "null") || strings.EqualFold(s, "~") ||
		isNumeric(s) {
		return quote(s)
	}
	return s
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	dot := false
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case (r == '-' || r == '+') && i == 0:
		case r == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return true
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
