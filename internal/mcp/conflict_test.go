package mcp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/misty-step/exocortex/internal/kernel"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolResultConflictParity(t *testing.T) {
	cases := []*kernel.Conflict{
		{
			Code:      "exists",
			Operation: "create",
			Path:      "notes/x.md",
			Hint:      "bare put creates only",
		},
		{
			Code:      "revision_conflict",
			Operation: "update",
			Path:      "notes/x.md",
			Hint:      "remote moved",
			Detail:    map[string]any{"expected": "aaa", "actual": "bbb"},
		},
		{
			Code:   "invalid_input",
			Hint:   "run help",
			Detail: map[string]any{"detail": "no command given"},
		},
		{
			Code:      "internal_error",
			Operation: "put",
			Hint:      "retry",
			Detail:    map[string]any{"detail": "boom"},
		},
		{
			Code:      "search_unavailable",
			Operation: "search",
			Path:      "powder",
			Hint:      "check that qmd is installed",
			Detail:    map[string]any{"detail": "qmd: not found"},
		},
	}
	for _, conf := range cases {
		t.Run(conf.Code, func(t *testing.T) {
			res, structured, err := toolResult(nil, conf)
			if err != nil {
				t.Fatalf("toolResult SDK error: %v", err)
			}
			if structured != nil {
				t.Fatalf("structured=%v want nil", structured)
			}
			if res == nil || !res.IsError {
				t.Fatalf("want tool error result, got %#v", res)
			}
			if len(res.Content) != 1 {
				t.Fatalf("content=%d", len(res.Content))
			}
			tc, ok := res.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content %T", res.Content[0])
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
				t.Fatalf("body not JSON: %v\n%s", err, tc.Text)
			}
			want := jsonRoundTrip(t, conf.Body())
			if !reflect.DeepEqual(body, want) {
				t.Fatalf("MCP body drifted from Conflict.Body\nmcp=%v\nbody=%v", body, want)
			}
		})
	}
}

func TestToolResultSuccessStaysNonError(t *testing.T) {
	res, structured, err := toolResult(map[string]any{"ok": true}, nil)
	if err != nil || structured != nil || res == nil || res.IsError {
		t.Fatalf("success: res=%#v structured=%v err=%v", res, structured, err)
	}
}

func TestToolResultMarshalFaultIsSDKError(t *testing.T) {
	res, structured, err := toolResult(func() {}, nil)
	if err == nil {
		t.Fatalf("want SDK marshal error, got res=%#v structured=%v", res, structured)
	}
	if res != nil || structured != nil {
		t.Fatalf("SDK fault must not produce a tool result: res=%#v structured=%v", res, structured)
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
