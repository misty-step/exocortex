package qmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Empty mode must select the deterministic BM25 subcommand: the kernel
// primitive may not depend on LLM-backed expansion availability.
func TestSubcommandFor(t *testing.T) {
	cases := map[string]string{
		"":       "search",
		"bm25":   "search",
		"hybrid": "query",
		"vector": "vsearch",
	}
	for mode, want := range cases {
		got, err := subcommandFor(mode)
		if err != nil || got != want {
			t.Errorf("subcommandFor(%q) = %q, %v; want %q", mode, got, err, want)
		}
	}
	if _, err := subcommandFor("semantic"); err == nil {
		t.Error("unknown mode must error")
	}
}

// TestSearchLargeJSONAndMultiCollection verifies that Search passes multi-collection
// flags correctly, strips CI environment variables, and parses JSON output exceeding
// the 8 KiB OS pipe buffer without truncation.
func TestSearchLargeJSONAndMultiCollection(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	// Generate a script that verifies arguments and outputs ~20 KB of JSON (>8 KiB)
	script := `#!/bin/sh
# Check that CI is stripped from environment
if [ -n "$CI" ] || [ -n "$CI_MODE" ]; then
  echo "error: CI environment variable not sanitized" >&2
  exit 1
fi

# Verify passed arguments
args="$*"
if ! echo "$args" | grep -q -- "-c col1" || ! echo "$args" | grep -q -- "-c col2"; then
  echo "error: missing collection flags: $args" >&2
  exit 1
fi

# Output large JSON array with 30 items (~20KB)
echo "["
for i in $(seq 1 30); do
  comma=","
  if [ "$i" -eq 30 ]; then comma=""; fi
  cat <<ITEM
  {
    "docid": "#doc$i",
    "file": "qmd://col1/notes/note-$i.md",
    "score": 0.95,
    "line": 42,
    "title": "Large Test Note $i",
    "context": "This is a verbose context summary designed to expand JSON payload size past 8192 bytes for testing.",
    "snippet": "@@ -1,10 @@ Verbose snippet content paragraph $i with detailed lines to ensure large payload size."
  }$comma
ITEM
done
echo "]"
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Prepend fakeQMD binDir to PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+origPath)
	t.Setenv("CI", "true")
	t.Setenv("CI_MODE", "1")

	hits, err := Search(context.Background(), "test-query", []string{"col1", "col2"}, "hybrid", 30)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 30 {
		t.Fatalf("got %d hits, want 30", len(hits))
	}
	if hits[0].DocID != "#doc1" || hits[29].DocID != "#doc30" {
		t.Fatalf("unexpected hit IDs: first=%s last=%s", hits[0].DocID, hits[29].DocID)
	}
	if hits[0].Title != "Large Test Note 1" || hits[0].Line != 42 {
		t.Fatalf("unexpected hit fields: %+v", hits[0])
	}
}
