package fm

import (
	"strings"
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		name, raw, wantFMText string
		wantFM                bool
	}{
		{"full", "---\ntype: note\n---\nbody\n", "type: note", true},
		{"no open", "just text\n", "", false},
		{"no close", "---\ntype: note\nnope\n", "", false},
		{"empty body", "---\ntype: x\n---\n", "type: x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := Split([]byte(tc.raw))
			if n.HasFM != tc.wantFM || n.FMText != tc.wantFMText {
				t.Fatalf("Split(%q) = HasFM=%v FMText=%q", tc.raw, n.HasFM, n.FMText)
			}
		})
	}
}

func TestValidateDaybook(t *testing.T) {
	ok := Split([]byte("---\ntype: decision\n---\nbody"))
	fs, err := Validate("daybook", ok)
	if err != nil {
		t.Fatalf("minimal type-only note must pass daybook profile: %v", err)
	}
	if len(fs) == 0 {
		t.Fatal("expected warnings for missing optional keys")
	}

	if _, err := Validate("daybook", Split([]byte("---\nstatus: active\n---\nbody"))); err == nil {
		t.Fatal("missing type must fail")
	}
	if _, err := Validate("daybook", Split([]byte("plain body"))); err == nil {
		t.Fatal("missing frontmatter must fail")
	}
	if _, err := Validate("daybook", Split([]byte("---\n: [unclosed\n---\n"))); err == nil {
		t.Fatal("unparseable frontmatter must fail")
	}
	// Unknown type vocabulary tolerated; bad created only warns.
	fs, err = Validate("daybook", Split([]byte("---\ntype: zettel-exotic\ncreated: not-a-date\n---\n")))
	if err != nil {
		t.Fatalf("unknown type value must only warn: %v", err)
	}
	var sawCreatedFormat bool
	for _, f := range fs {
		if f.Rule == "created_format" {
			sawCreatedFormat = true
		}
		if f.Level != "warning" {
			t.Fatalf("daybook profile must warn, never error, on %v", f)
		}
	}
	if !sawCreatedFormat {
		t.Fatal("non-RFC3339 created should warn")
	}

	// Date-only created is non-empty but NOT RFC3339: the lexical read
	// must catch it in both profiles despite yaml.v3 decoding it to
	// time.Time.
	dateOnly := Split([]byte("---\ntype: x\ncreated: 2026-08-21\n---\n"))
	fs, err = Validate("daybook", dateOnly)
	if err != nil {
		t.Fatalf("date-only created must warn, not fail, under daybook: %v", err)
	}
	if !hasRule(fs, "created_format") {
		t.Fatalf("daybook missed date-only created: %+v", fs)
	}
	if _, err := Validate("strict", dateOnly); err == nil {
		t.Fatal("strict must reject date-only created")
	}
}

func hasRule(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestValidateStrict(t *testing.T) {
	full := Split([]byte("---\ntype: a\nstatus: b\ncreated: 2026-08-21T00:00:00Z\ndescription: d\ntags: [x]\n---\n"))
	if _, err := Validate("strict", full); err != nil {
		t.Fatalf("complete strict note must pass: %v", err)
	}
	partial := Split([]byte("---\ntype: a\nstatus: b\n---\n"))
	if _, err := Validate("strict", partial); err == nil {
		t.Fatal("strict requires all five keys")
	}
	badDate := Split([]byte("---\ntype: a\nstatus: b\ncreated: yesterday\ndescription: d\ntags: [x]\n---\n"))
	if _, err := Validate("strict", badDate); err == nil {
		t.Fatal("strict requires RFC3339 created")
	}
	// Date-only created fails strict; unquoted full RFC3339 passes —
	// both resolve to time.Time, so only the lexical check distinguishes.
	dateOnly := Split([]byte("---\ntype: a\nstatus: b\ncreated: 2026-08-21\ndescription: d\ntags: [x]\n---\n"))
	if _, err := Validate("strict", dateOnly); err == nil {
		t.Fatal("strict must reject date-only created")
	}
	unq := Split([]byte("---\ntype: a\nstatus: b\ncreated: 2019-03-04T05:06:07Z\ndescription: d\ntags: [x]\n---\n"))
	if _, err := Validate("strict", unq); err != nil {
		t.Fatalf("unquoted RFC3339 must pass strict: %v", err)
	}
}

const richNote = `---
type: decision
status: active # keep this comment
created: 2020-01-01T00:00:00Z
description: >
  folded
  description
tags: [a, b]
custom:
  nested: true
  list: [1, 2]
---

Body text with ---
fences and blank lines

`

// nonProvenanceLines strips the provenance block so two splices can be
// compared everywhere the kernel does not own.
func nonProvenanceLines(s string) string {
	var keep []string
	inProv := false
	for _, ln := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(ln, "provenance:"):
			inProv = true
			continue
		case inProv && strings.HasPrefix(ln, "  "):
			continue
		case inProv:
			inProv = false
		}
		keep = append(keep, ln)
	}
	return strings.Join(keep, "\n")
}

func TestSplicePreservesBytes(t *testing.T) {
	at := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// Insert into a note without provenance: everything else byte-identical.
	out := string(SpliceProvenance([]byte(richNote), Provenance{Agent: "ox-alpha", At: at, Via: "mcp"}))
	for _, want := range []string{
		"status: active # keep this comment",
		"created: 2020-01-01T00:00:00Z",
		"  nested: true",
		"Body text with ---",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("splice lost %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "agent: ox-alpha") || !strings.Contains(out, "via: mcp") {
		t.Fatalf("provenance missing:\n%s", out)
	}

	// Replace an existing provenance block: rest unchanged, old block gone.
	withProv := strings.Replace(richNote, "type: decision",
		"type: decision\nprovenance:\n  agent: old-agent\n  at: 1999-01-01T00:00:00Z\n  via: cli", 1)
	out2 := string(SpliceProvenance([]byte(withProv), Provenance{Agent: "new-agent", At: at, Via: "cli"}))
	if strings.Contains(out2, "old-agent") {
		t.Fatalf("stale provenance survived:\n%s", out2)
	}
	if !strings.Contains(out2, "agent: new-agent") {
		t.Fatalf("fresh provenance missing:\n%s", out2)
	}
	if nonProvenanceLines(out) != nonProvenanceLines(out2) {
		t.Fatalf("non-provenance bytes differ between insert and replace")
	}

	// Idempotence: re-stamping with identical values changes nothing.
	out3 := string(SpliceProvenance([]byte(out), Provenance{Agent: "ox-alpha", At: at, Via: "mcp"}))
	if out3 != out {
		t.Fatalf("re-stamp with identical values changed bytes:\n%s", out3)
	}
}

func TestSpliceWithoutFrontmatter(t *testing.T) {
	raw := []byte("no frontmatter here\n")
	if got := SpliceProvenance(raw, Provenance{Agent: "x", At: time.Now(), Via: "cli"}); string(got) != string(raw) {
		t.Fatal("note without frontmatter must be returned unchanged")
	}
}

func TestYamlScalar(t *testing.T) {
	cases := map[string]string{
		"ox-alpha":    "ox-alpha",
		"claude-code": "claude-code",
		"a b":         `"a b"`,
		"a:b":         `"a:b"`,
		"":            `""`,
		"true":        `"true"`,
		"123":         `"123"`,
		"-dash":       `"-dash"`,
		"harness#tag": `"harness#tag"`,
	}
	for in, want := range cases {
		if got := yamlScalar(in); got != want {
			t.Errorf("yamlScalar(%q) = %q, want %q", in, got, want)
		}
	}
}
