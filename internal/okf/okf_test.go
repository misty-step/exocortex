package okf

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func parse(t *testing.T, raw string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatalf("fixture must parse: %v", err)
	}
	return &root
}

func validate(t *testing.T, raw string) []Violation {
	t.Helper()
	return Validate(parse(t, raw))
}

func hasViolation(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

const richFixture = `---
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

func TestValidateRichFixture(t *testing.T) {
	if vs := validate(t, richFixture); len(vs) != 0 {
		t.Fatalf("valid OKF v0.2 signals must not be flagged: %+v", vs)
	}
}

// OKF v0.2 SPEC §5: "Every timestamp-valued key in OKF is an ISO 8601
// datetime with an explicit UTC offset." Any explicit offset conforms;
// offset zero is one valid spelling, not the only one.
func TestValidateAcceptsNonZeroOffsets(t *testing.T) {
	raw := `---
generated:
  by: human:alice
  at: 2026-06-20T22:53:05+05:30
verified:
  - by: process:nightly
    at: 2026-06-25T09:00:00-04:00
stale_after: 2026-12-31T00:00:00+01:00
provenance:
  at: 2026-06-28T14:00:00+02:00
---
`
	vs := validate(t, raw)
	for _, rule := range []string{
		"generated_at_format", "verified_at_format", "stale_after_format", "provenance_at_format",
	} {
		if hasViolation(vs, rule) {
			t.Fatalf("explicit nonzero offset must satisfy %s: %+v", rule, vs)
		}
	}
}

// The offset must be explicit: RFC3339 datetimes without an offset
// fail the layout parse and violate every timestamp rule.
func TestValidateRejectsMissingOffset(t *testing.T) {
	raw := `---
generated:
  by: human:alice
  at: 2026-06-20T22:53:05
provenance:
  at: 2026-06-28T14:00:00
---
`
	vs := validate(t, raw)
	if !hasViolation(vs, "generated_at_format") || !hasViolation(vs, "provenance_at_format") {
		t.Fatalf("offset-less datetimes must violate: %+v", vs)
	}
}

func TestValidateRejectsMalformedSignals(t *testing.T) {
	raw := `---
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
`
	vs := validate(t, raw)
	for _, rule := range []string{
		"generated_by_format", "generated_at_format",
		"verified_by_format", "verified_at_format",
		"stale_after_format", "provenance_at_format",
	} {
		if !hasViolation(vs, rule) {
			t.Fatalf("missing %s in %+v", rule, vs)
		}
	}
}

func TestValidateVerifiedMapping(t *testing.T) {
	vs := validate(t, "---\nverified: {by: process:nightly, at: 2026-06-25T09:00:00Z}\n---\n")
	if hasViolation(vs, "verified_format") {
		t.Fatalf("bare verified mapping is valid OKF v0.2: %+v", vs)
	}
}

func TestValidateGeneratedAtOptional(t *testing.T) {
	raw := `---
generated:
  by: human:alice
---
`
	if vs := validate(t, raw); len(vs) != 0 {
		t.Fatalf("generated.at is optional when production time is unknown: %+v", vs)
	}
}

func TestValidateMergeKeys(t *testing.T) {
	plain := `---
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
<<: *defaults
---
`
	if vs := validate(t, plain); !hasViolation(vs, "generated_by_format") {
		t.Fatalf("merged generated metadata was skipped: %+v", vs)
	}
	quoted := `---
:"<<":
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
---
`
	if vs := validate(t, quoted); hasViolation(vs, "generated_by_format") {
		t.Fatalf("quoted << key must not act as a merge: %+v", vs)
	}
	longTag := `---
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
!<tag:yaml.org,2002:merge> "<<": *defaults
---
`
	if vs := validate(t, longTag); !hasViolation(vs, "generated_by_format") {
		t.Fatalf("long-form merge tag must act as a merge: %+v", vs)
	}
	aliasKey := `---
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
merge_name: &merge_name "<<"
*merge_name: *defaults
---
`
	if vs := validate(t, aliasKey); hasViolation(vs, "generated_by_format") {
		t.Fatalf("aliased << key must not act as a merge: %+v", vs)
	}
	explicitTag := `---
defaults: &defaults
  generated:
    by: "bad actor"
    at: 2026-06-20T22:53:05Z
! <<: *defaults
---
`
	if vs := validate(t, explicitTag); !hasViolation(vs, "generated_by_format") {
		t.Fatalf("explicit non-specific merge tag must act as a merge: %+v", vs)
	}
}
