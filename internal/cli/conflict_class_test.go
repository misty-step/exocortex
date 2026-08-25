package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/misty-step/exocortex/internal/kernel"
)

// Every documented input-class code and representative operation/internal
// codes must keep their SPEC exit mapping. emit is the CLI face: it may
// only consult Conflict.Class, never a second code-name table.
func TestEmitMapsConflictClassToExit(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"invalid_input", 2},
		{"unknown_command", 2},
		{"registration_failed", 2},
		{"payload_unreadable", 2},
		{"internal_error", 2},
		{"exists", 1},
		{"revision_conflict", 1},
		{"created_immutable", 1},
		{"dirty_destination", 1},
		{"foreign_staged_state", 1},
		{"foreign_unstaged_state", 1},
		{"journal_immutable", 1},
		{"not_found", 1},
		{"duplicate_cortex", 1},
		{"search_unavailable", 1},
		{"writer_unavailable", 1},
		{"cortex_unavailable", 1},
		{"empty_note", 1},
		{"novel_operation_code", 1},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			var out bytes.Buffer
			conf := &kernel.Conflict{
				Code:      tc.code,
				Operation: "probe",
				Path:      "notes/x.md",
				Hint:      "hint",
				Detail:    map[string]any{"expected": "e", "actual": "a"},
			}
			got := emit(&out, nil, conf)
			if got != tc.want {
				t.Fatalf("exit=%d want %d", got, tc.want)
			}
			var body map[string]any
			if err := json.Unmarshal(out.Bytes(), &body); err != nil {
				t.Fatalf("stdout not JSON: %v\n%s", err, out.String())
			}
			if body["error"] != tc.code {
				t.Fatalf("error=%v want %s", body["error"], tc.code)
			}
			if body["operation"] != "probe" || body["path"] != "notes/x.md" || body["hint"] != "hint" {
				t.Fatalf("pinned fields missing: %v", body)
			}
			if body["expected"] != "e" || body["actual"] != "a" {
				t.Fatalf("detail keys not flattened: %v", body)
			}
			wantBody := conf.Body()
			if !mapsEqual(body, wantBody) {
				t.Fatalf("CLI body drifted from Conflict.Body:\ncli=%v\nbody=%v", body, wantBody)
			}
		})
	}
}

func TestEmitOmitsEmptyOperationAndPath(t *testing.T) {
	var out bytes.Buffer
	conf := &kernel.Conflict{Code: "invalid_input", Hint: "run help"}
	if code := emit(&out, nil, conf); code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["operation"]; ok {
		t.Fatalf("empty operation should be omitted: %v", body)
	}
	if _, ok := body["path"]; ok {
		t.Fatalf("empty path should be omitted: %v", body)
	}
	if !mapsEqual(body, conf.Body()) {
		t.Fatalf("CLI body drifted from Conflict.Body:\ncli=%v\nbody=%v", body, conf.Body())
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		as, aok := av.(string)
		bs, bok := bv.(string)
		if aok && bok {
			if as != bs {
				return false
			}
			continue
		}
		if fmtValue(av) != fmtValue(bv) {
			return false
		}
	}
	return true
}

func fmtValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
