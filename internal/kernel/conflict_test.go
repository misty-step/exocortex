package kernel

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConflictClassTable(t *testing.T) {
	cases := []struct {
		code string
		want Class
	}{
		{"invalid_input", ClassInput},
		{"unknown_command", ClassInput},
		{"registration_failed", ClassInput},
		{"payload_unreadable", ClassInput},
		{"internal_error", ClassInternal},
		{"exists", ClassOperation},
		{"revision_conflict", ClassOperation},
		{"created_immutable", ClassOperation},
		{"dirty_destination", ClassOperation},
		{"foreign_staged_state", ClassOperation},
		{"foreign_unstaged_state", ClassOperation},
		{"journal_immutable", ClassOperation},
		{"not_found", ClassOperation},
		{"duplicate_cortex", ClassOperation},
		{"duplicate_path", ClassOperation},
		{"search_unavailable", ClassUnavailable},
		{"writer_unavailable", ClassUnavailable},
		{"cortex_unavailable", ClassUnavailable},
		{"log_unavailable", ClassUnavailable},
		{"publish_rejected", ClassUnavailable},
		{"publish_unknown", ClassUnavailable},
		{"empty_note", ClassOperation},
		{"resolve_failed", ClassOperation},
		{"lock_failed", ClassOperation},
		{"novel_code", ClassOperation},
	}
	for _, tc := range cases {
		got := (&Conflict{Code: tc.code}).Class()
		if got != tc.want {
			t.Fatalf("%s: Class()=%v want %v", tc.code, got, tc.want)
		}
	}
}

func TestConflictBodyGolden(t *testing.T) {
	cases := []struct {
		name string
		in   Conflict
		want map[string]any
	}{
		{
			name: "exists",
			in: Conflict{
				Code:      "exists",
				Operation: "create",
				Path:      "notes/x.md",
				Hint:      "bare put creates only; read the note with get and update it with --expects",
			},
			want: map[string]any{
				"error":     "exists",
				"operation": "create",
				"path":      "notes/x.md",
				"hint":      "bare put creates only; read the note with get and update it with --expects",
			},
		},
		{
			name: "revision_conflict",
			in: Conflict{
				Code:      "revision_conflict",
				Operation: "update",
				Path:      "notes/x.md",
				Hint:      "remote moved; re-read with get and retry",
				Detail:    map[string]any{"expected": "aaa", "actual": "bbb"},
			},
			want: map[string]any{
				"error":     "revision_conflict",
				"operation": "update",
				"path":      "notes/x.md",
				"hint":      "remote moved; re-read with get and retry",
				"expected":  "aaa",
				"actual":    "bbb",
			},
		},
		{
			name: "created_immutable",
			in: Conflict{
				Code:      "created_immutable",
				Operation: "update",
				Path:      "notes/x.md",
				Hint:      "created never changes; resubmit with created: 2026-08-21T00:00:00Z",
				Detail:    map[string]any{"stored": "2026-08-21T00:00:00Z", "submitted": "1999-01-01T00:00:00Z"},
			},
			want: map[string]any{
				"error":     "created_immutable",
				"operation": "update",
				"path":      "notes/x.md",
				"hint":      "created never changes; resubmit with created: 2026-08-21T00:00:00Z",
				"stored":    "2026-08-21T00:00:00Z",
				"submitted": "1999-01-01T00:00:00Z",
			},
		},
		{
			name: "foreign_staged_state",
			in: Conflict{
				Code:      "foreign_staged_state",
				Operation: "put",
				Path:      "notes/x.md",
				Hint:      "another worker staged paths in this cortex; commit or unstage your own staged work first; never stash or discard theirs",
				Detail:    map[string]any{"paths": []string{"other.md"}},
			},
			want: map[string]any{
				"error":     "foreign_staged_state",
				"operation": "put",
				"path":      "notes/x.md",
				"hint":      "another worker staged paths in this cortex; commit or unstage your own staged work first; never stash or discard theirs",
				"paths":     []any{"other.md"},
			},
		},
		{
			name: "invalid_input omits empty path",
			in: Conflict{
				Code:      "invalid_input",
				Operation: "put",
				Hint:      "pass --from",
				Detail:    map[string]any{"detail": "missing --from"},
			},
			want: map[string]any{
				"error":     "invalid_input",
				"operation": "put",
				"hint":      "pass --from",
				"detail":    "missing --from",
			},
		},
		{
			name: "unknown_command",
			in: Conflict{
				Code:      "unknown_command",
				Operation: "frobnicate",
				Hint:      "run `exocortex help` for the command list",
			},
			want: map[string]any{
				"error":     "unknown_command",
				"operation": "frobnicate",
				"hint":      "run `exocortex help` for the command list",
			},
		},
		{
			name: "internal_error",
			in: Conflict{
				Code:      "internal_error",
				Operation: "put",
				Hint:      "retry; if persistent, inspect the exocortex installation",
				Detail:    map[string]any{"detail": "boom"},
			},
			want: map[string]any{
				"error":     "internal_error",
				"operation": "put",
				"hint":      "retry; if persistent, inspect the exocortex installation",
				"detail":    "boom",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Body()
			gotNorm := jsonRoundTrip(t, got)
			wantNorm := jsonRoundTrip(t, tc.want)
			if !reflect.DeepEqual(gotNorm, wantNorm) {
				t.Fatalf("Body mismatch\ngot  %#v\nwant %#v", gotNorm, wantNorm)
			}
		})
	}
}
func TestPublicationConflictCleanUnwindIsJSONArray(t *testing.T) {
	conf := publicationConflict("publish_rejected", "create", "notes/x.md", PutInput{}, nil, nil, "retry")
	body := jsonRoundTrip(t, conf.Body())
	unwind, ok := body["unwind"].([]any)
	if !ok || len(unwind) != 0 {
		t.Fatalf("unwind=%#v want empty JSON array", body["unwind"])
	}
}

func jsonRoundTrip(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
