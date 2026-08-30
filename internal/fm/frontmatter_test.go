package fm

import (
	"strings"
	"testing"
	"time"
)

func TestDocumentSplit(t *testing.T) {
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
			n := ParseDocument([]byte(tc.raw)).Note
			if n.HasFM != tc.wantFM || n.FMText != tc.wantFMText {
				t.Fatalf("ParseDocument(%q).Note = HasFM=%v FMText=%q", tc.raw, n.HasFM, n.FMText)
			}
		})
	}
}

func validate(profile, raw string) ([]Finding, error) {
	return Validate(profile, ParseDocument([]byte(raw)))
}

func TestValidateDaybook(t *testing.T) {
	fs, err := validate("daybook", "---\ntype: decision\n---\nbody")
	if err != nil {
		t.Fatalf("minimal type-only note must pass daybook profile: %v", err)
	}
	if len(fs) == 0 {
		t.Fatal("expected warnings for missing optional keys")
	}

	if _, err := validate("daybook", "---\nstatus: active\n---\nbody"); err == nil {
		t.Fatal("missing type must fail")
	}
	if _, err := validate("daybook", "plain body"); err == nil {
		t.Fatal("missing frontmatter must fail")
	}
	if _, err := validate("daybook", "---\n: [unclosed\n---\n"); err == nil {
		t.Fatal("unparseable frontmatter must fail")
	}
	// Unknown type vocabulary tolerated; bad created only warns.
	fs, err = validate("daybook", "---\ntype: zettel-exotic\ncreated: not-a-date\n---\n")
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
	dateOnly := "---\ntype: x\ncreated: 2026-08-21\n---\n"
	fs, err = validate("daybook", dateOnly)
	if err != nil {
		t.Fatalf("date-only created must warn, not fail, under daybook: %v", err)
	}
	if !hasRule(fs, "created_format") {
		t.Fatalf("daybook missed date-only created: %+v", fs)
	}
	if _, err := validate("strict", dateOnly); err == nil {
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
	full := "---\ntype: a\nstatus: b\ncreated: 2026-08-21T00:00:00Z\ndescription: d\ntags: [x]\n---\n"
	if _, err := validate("strict", full); err != nil {
		t.Fatalf("complete strict note must pass: %v", err)
	}
	if _, err := validate("strict", "---\ntype: a\nstatus: b\n---\n"); err == nil {
		t.Fatal("strict requires all five keys")
	}
	if _, err := validate("strict", "---\ntype: a\nstatus: b\ncreated: yesterday\ndescription: d\ntags: [x]\n---\n"); err == nil {
		t.Fatal("strict requires RFC3339 created")
	}
	// Date-only created fails strict; unquoted full RFC3339 passes —
	// both resolve to time.Time, so only the lexical check distinguishes.
	dateOnly := "---\ntype: a\nstatus: b\ncreated: 2026-08-21\ndescription: d\ntags: [x]\n---\n"
	if _, err := validate("strict", dateOnly); err == nil {
		t.Fatal("strict must reject date-only created")
	}
	unq := "---\ntype: a\nstatus: b\ncreated: 2019-03-04T05:06:07Z\ndescription: d\ntags: [x]\n---\n"
	if _, err := validate("strict", unq); err != nil {
		t.Fatalf("unquoted RFC3339 must pass strict: %v", err)
	}
}

func TestValidateOKFV02(t *testing.T) {
	raw := `---
type: Attested Computation
title: Revenue
description: Recognized revenue.
resource: https://example.test/revenue
sources:
  - resource: https://example.test/policy
generated:
  by: reference_agent/gemini-2.5-pro
  at: 2026-06-20T22:53:05Z
verified:
  - by: human:ahormati
    at: 2026-06-25T09:00:00Z
  - by: process:finance-nightly
    at: 2026-06-26T02:00:00+00:00
stale_after: 2026-12-31T00:00:00+01:00
runtime: bigquery
parameters:
  - name: year
    type: integer
    required: true
computation: references/revenue.sql
executor:
  resource: references/run.md
attester:
  resource: references/check.py
usage_window:
  from: 2026-06-01
  to: 2026-06-30
provenance:
  agent: kernel
  at: 2026-06-28T14:00:00Z
  via: cli
status: stable
created: 2026-06-20T22:53:05Z
tags: [finance]
---
body
`
	fs, err := validate("daybook", raw)
	if err != nil {
		t.Fatalf("valid OKF v0.2 note must pass daybook: %v", err)
	}
	for _, f := range fs {
		if f.Rule == "unknown_keys" {
			t.Fatalf("OKF v0.2 keys should be known: %+v", fs)
		}
		if f.Rule == "generated_by_format" || f.Rule == "generated_at_format" ||
			f.Rule == "verified_by_format" || f.Rule == "verified_at_format" ||
			f.Rule == "stale_after_format" || f.Rule == "provenance_at_format" {
			t.Fatalf("valid OKF v0.2 signal should not be flagged: %+v", f)
		}
	}
	if _, err := validate("strict", raw); err != nil {
		t.Fatalf("valid OKF v0.2 note must pass strict: %v", err)
	}
}

func TestValidateOKFV02RejectsMalformedSignals(t *testing.T) {
	raw := `---
type: Metric
status: stable
created: 2026-06-20T22:53:05Z
description: Revenue.
tags: [finance]
generated:
  by: "human:"
  at: 2026-06-20
verified:
  - by: agent
    at: 2026-06-25
stale_after: not-a-date
provenance:
  at: yesterday
---
body
`
	fs, err := validate("daybook", raw)
	if err != nil {
		t.Fatalf("malformed optional signals must warn, not fail, under daybook: %v", err)
	}
	for _, rule := range []string{
		"generated_by_format", "generated_at_format",
		"verified_by_format", "verified_at_format",
		"stale_after_format", "provenance_at_format",
	} {
		if !hasRule(fs, rule) {
			t.Fatalf("daybook missing %s in %+v", rule, fs)
		}
	}
	if _, err := validate("strict", raw); err == nil {
		t.Fatal("strict must reject malformed OKF v0.2 signals")
	}
}

func TestValidateOKFV02VerifiedMapping(t *testing.T) {
	raw := `---
type: Metric
verified: {by: process:nightly, at: 2026-06-25T09:00:00Z}
---
body
`
	if fs, err := validate("daybook", raw); err != nil || hasRule(fs, "verified_format") {
		t.Fatalf("bare verified mapping is valid OKF v0.2: findings=%+v err=%v", fs, err)
	}
}

func TestValidateOKFV02GeneratedAtOptional(t *testing.T) {
	raw := `---
type: Metric
status: stable
created: 2026-06-20T22:53:05Z
description: Revenue.
tags: [finance]
generated:
  by: human:alice
---
body
`
	if _, err := validate("strict", raw); err != nil {
		t.Fatalf("generated.at is optional: %v", err)
	}
}

func TestValidateOKFV02MergeKeys(t *testing.T) {
	raw := `---
defaults: &defaults
  type: Metric
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
<<: *defaults
---
body
`
	fs, err := validate("daybook", raw)
	if err != nil {
		t.Fatalf("merged OKF metadata must still validate: %v", err)
	}
	if !hasRule(fs, "generated_by_format") {
		t.Fatalf("merged generated metadata was skipped: %+v", fs)
	}
	quoted := `---
type: Metric
"<<":
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
---
body
`
	qfs, qerr := validate("daybook", quoted)
	if qerr != nil || hasRule(qfs, "generated_by_format") {
		t.Fatalf("quoted << key must not act as a merge: findings=%+v err=%v", qfs, qerr)
	}

	longTag := `---
defaults: &defaults
  type: Metric
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
!<tag:yaml.org,2002:merge> "<<": *defaults
---
body
`
	lfs, lerr := validate("daybook", longTag)
	if lerr != nil || !hasRule(lfs, "generated_by_format") {
		t.Fatalf("long-form merge tag must act as a merge: findings=%+v err=%v", lfs, lerr)
	}
	aliasKey := `---
type: Metric
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
merge_name: &merge_name "<<"
*merge_name: *defaults
---
body
`
	afs, aerr := validate("daybook", aliasKey)
	if aerr != nil || hasRule(afs, "generated_by_format") {
		t.Fatalf("aliased << key must not act as a merge: findings=%+v err=%v", afs, aerr)
	}
	explicitTag := `---
type: Metric
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
! <<: *defaults
---
body
`
	efs, eerr := validate("daybook", explicitTag)
	if eerr != nil || !hasRule(efs, "generated_by_format") {
		t.Fatalf("explicit non-specific merge tag must act as a merge: findings=%+v err=%v", efs, eerr)
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
