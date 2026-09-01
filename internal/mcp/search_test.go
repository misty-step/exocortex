package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/exocortex/internal/kernel"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTypedSearchFailsClosedWhenCortexUnavailable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "decision.md"), []byte("---\ntype: decision\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-b", "master", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"-C", root, "add", "decision.md"},
		{"-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "fixture"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := kernel.Register("broken", root, "daybook", "daybook", "journal"); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	qmd := "#!/bin/sh\necho '[{\"docid\":\"#d\",\"file\":\"qmd://broken/decision.md\",\"score\":0.9}]'\n"
	if err := os.WriteFile(filepath.Join(binDir, "qmd"), []byte(qmd), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	res, _, err := search(context.Background(), nil, searchArgs{Query: "decision", Type: "decision", Mode: "bm25"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("unavailable typed search must be a tool error: %#v", res)
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content %T", res.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "cortex_unavailable" {
		t.Fatalf("body=%v", body)
	}
}

func TestSearchReportsInvalidQMDOutput(t *testing.T) {
	binDir := t.TempDir()
	qmd := "#!/bin/sh\necho '[{\"score\": 0. 89}]'\n"
	if err := os.WriteFile(filepath.Join(binDir, "qmd"), []byte(qmd), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	res, _, err := search(context.Background(), nil, searchArgs{Query: "September", Mode: "bm25"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("invalid output must be a tool error: %#v", res)
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content %T", res.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
		t.Fatal(err)
	}
	hint, _ := body["hint"].(string)
	if body["error"] != "search_unavailable" ||
		!strings.Contains(hint, "raw qmd --format json") ||
		strings.Contains(hint, "indexed qmd collection") {
		t.Fatalf("misclassified decode body: %v", body)
	}
}
