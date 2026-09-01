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

	"github.com/misty-step/exocortex/internal/okf"
	"go.yaml.in/yaml/v3"
)

// Note is a raw file split into frontmatter and body.
type Note struct {
	Raw    []byte
	FMText string // between the --- delimiters, delimiters excluded
	Body   string // after the closing delimiter ("" if none)
	HasFM  bool
}

// Document is one parse of a note's frontmatter. It owns the original
// split, the decoded map, and the YAML node tree. Validation and
// lexical scalar access reuse it. Provenance splice and strip stay
// line-based over the original bytes and never round-trip YAML.
type Document struct {
	Note Note
	Map  map[string]any // nil if frontmatter is missing or unparseable
	err  error
	root yaml.Node
}

func (d Document) Err() error { return d.err }

var (
	openDelim  = "---\n"
	closerRe   = regexp.MustCompile(`(?m)^---[ \t]*(\r?\n|$)`)
	topLevelRe = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):`)
)

// split divides raw into frontmatter and body. Frontmatter is present
// only when the file starts with an opening --- delimiter line and a
// closing --- line exists at column 0 later in the file.
func split(raw []byte) Note {
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

// ParseDocument splits raw once and decodes the frontmatter mapping and
// YAML node tree from that split. Missing frontmatter is not an error;
// unparseable YAML is.
func ParseDocument(raw []byte) Document {
	n := split(raw)
	d := Document{Note: n}
	if !n.HasFM {
		return d
	}
	if err := yaml.Unmarshal([]byte(n.FMText), &d.root); err != nil {
		d.err = err
		return d
	}
	var m map[string]any
	if err := d.root.Decode(&m); err != nil {
		d.err = err
		return d
	}
	if m == nil {
		d.err = errors.New("frontmatter is not a mapping")
		return d
	}
	d.Map = m
	return d
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
	// OKF v0.2's optional provenance, lifecycle, and computation
	// families are part of the frontmatter vocabulary even when a
	// particular concept does not use them.
	"title": true, "resource": true, "sources": true,
	"generated": true, "verified": true, "stale_after": true,
	"runtime": true, "parameters": true, "computation": true,
	"executor": true, "attester": true, "usage_window": true,
}

func validateStrict(d Document) ([]Finding, error) {
	if err := requireFrontmatter(d); err != nil {
		return nil, err
	}
	var fs []Finding
	for _, k := range []string{"type", "status", "created", "description", "tags"} {
		if empty(d.Map[k]) {
			fs = append(fs, errf("key_missing", "strict profile requires non-empty %q", k))
		}
	}
	if c, ok := d.Scalar("created"); ok && strings.TrimSpace(c) != "" {
		if _, perr := time.Parse(time.RFC3339, c); perr != nil {
			fs = append(fs, errf("created_format", "created %q is not RFC3339", c))
		}
	}
	fs = append(fs, validateOKF(d, true)...)
	for _, f := range fs {
		if f.Level == "error" {
			return fs, contract(f)
		}
	}
	return fs, nil
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

// Validate applies a named profile to an already-parsed document.
// Errors are contract failures; warnings are advisory and never
// blocking under any profile.
func Validate(profile string, d Document) ([]Finding, error) {
	switch profile {
	case "daybook":
		return validateDaybook(d)
	case "strict":
		return validateStrict(d)
	default:
		return nil, fmt.Errorf("unknown profile %q", profile)
	}
}

func requireFrontmatter(d Document) error {
	if !d.Note.HasFM {
		return contract(errf("fm_missing", "frontmatter missing"))
	}
	if d.err != nil {
		return contract(errf("fm_unparseable", "frontmatter is not parseable YAML: %v", d.err))
	}
	return nil
}

func validateDaybook(d Document) ([]Finding, error) {
	if err := requireFrontmatter(d); err != nil {
		return nil, err
	}
	if t, ok := d.Map["type"].(string); !ok || strings.TrimSpace(t) == "" {
		return nil, contract(errf("type_missing", "frontmatter has no non-empty \"type\""))
	}
	return daybookWarnings(d), nil
}

func daybookWarnings(d Document) []Finding {
	// Journal micro-notes (type: memo) are quiet under the daybook
	// profile: they are not wiki notes, and constant key warnings would
	// erode trust in one-line memories.
	if t, _ := d.Map["type"].(string); t == "memo" {
		return nil
	}
	var fs []Finding
	for _, k := range []string{"status", "description", "tags", "created"} {
		if empty(d.Map[k]) {
			fs = append(fs, warnf("key_missing", "%q missing or empty", k))
		}
	}
	if c, ok := d.Scalar("created"); ok && c != "" {
		if _, perr := time.Parse(time.RFC3339, c); perr != nil {
			fs = append(fs, warnf("created_format", "created %q is not RFC3339", c))
		}
	}
	fs = append(fs, validateOKF(d, false)...)
	var unknown []string
	for k := range d.Map {
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

// validateOKF applies the structural checks shared by the OKF v0.2
// provenance and lifecycle fields. Daybook reports malformed optional
// fields as warnings; strict promotes the same findings to errors.
func validateOKF(d Document, strict bool) []Finding {
	var fs []Finding
	for _, v := range okf.Validate(&d.root) {
		if strict {
			fs = append(fs, errf(v.Rule, "%s", v.Message))
		} else {
			fs = append(fs, warnf(v.Rule, "%s", v.Message))
		}
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
	n := split(raw)
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

// Scalar returns the lexical text of a top-level scalar key from the
// already-parsed YAML node. ok is false when the key is absent or its
// value is not a scalar.
func (d Document) Scalar(key string) (string, bool) {
	if !d.Note.HasFM {
		return "", false
	}
	if len(d.root.Content) == 0 || d.root.Content[0].Kind != yaml.MappingNode {
		return "", false
	}
	mapping := d.root.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			v := mapping.Content[i+1]
			if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
				return "", false
			}
			return v.Value, true
		}
	}
	return "", false
}

// StripProvenance removes the kernel-owned provenance block from a
// note's frontmatter, leaving all other bytes untouched. It is the
// identity for notes without frontmatter or without a provenance block.
func StripProvenance(raw []byte) []byte {
	n := split(raw)
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
