package qmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestSearchLargeJSONScaleWithoutTempFiles(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	// Generate a script that outputs 500 items (>500 KB) of JSON
	script := `#!/bin/sh
echo "["
for i in $(seq 1 500); do
  comma=","
  if [ "$i" -eq 500 ]; then comma=""; fi
  cat <<ITEM
  {
    "docid": "#scale$i",
    "file": "qmd://col1/notes/scale-$i.md",
    "score": 0.88,
    "line": $i,
    "title": "Scale Test Note $i with extensive title payload",
    "context": "Extended context field for item $i designed to ensure high memory throughput and stream buffer drainage.",
    "snippet": "@@ -10,20 @@ Very large snippet content section $i containing multiple lines of text, symbols, and structured Markdown formatting for stream verification."
  }$comma
ITEM
done
echo "]"
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	// Assert no lingering qmd-search temp files before query
	matchesBefore, err := filepath.Glob(filepath.Join(os.TempDir(), "qmd-search-*.json"))
	if err != nil {
		t.Fatal(err)
	}

	hits, err := Search(context.Background(), "scale-query", []string{"col1"}, "bm25", 500)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 500 {
		t.Fatalf("got %d hits, want 500", len(hits))
	}
	if hits[0].DocID != "#scale1" || hits[499].DocID != "#scale500" {
		t.Fatalf("unexpected bounds: first=%s, last=%s", hits[0].DocID, hits[499].DocID)
	}

	// Assert no qmd-search temp files were created on disk
	matchesAfter, err := filepath.Glob(filepath.Join(os.TempDir(), "qmd-search-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matchesAfter) > len(matchesBefore) {
		t.Fatalf("found %d temporary files created by Search: %v", len(matchesAfter)-len(matchesBefore), matchesAfter)
	}
}

func TestSearchHandlesLeadingNonJSONPreamble(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	script := `#!/bin/sh
echo "[info] model loaded in 12ms"
echo "[warning] collection cache cold, loading metadata..."
cat <<EOF
[
  {
    "docid": "#doc-preamble",
    "file": "qmd://daybook/notes/preamble.md",
    "score": 0.92,
    "line": 10,
    "title": "Preamble Test",
    "context": "Testing preamble skip",
    "snippet": "@@ -1,5 @@ snippet"
  }
]
EOF
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	hits, err := Search(context.Background(), "preamble-test", []string{"daybook"}, "bm25", 10)
	if err != nil {
		t.Fatalf("Search failed with preamble: %v", err)
	}
	if len(hits) != 1 || hits[0].DocID != "#doc-preamble" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}

func TestSearchNonZeroExitIncludesStderr(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	script := `#!/bin/sh
echo "error: GPU memory exhausted (failed to allocate 2048MB)" >&2
exit 1
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	_, err := Search(context.Background(), "fail-query", []string{"daybook"}, "bm25", 10)
	if err == nil {
		t.Fatal("expected Search to fail on non-zero exit")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "GPU memory exhausted") {
		t.Fatalf("error should contain stderr diagnostics: %q", errStr)
	}
}

func TestSearchHybridFallbackToBM25(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	// Fail when cmdName is query, succeed when cmdName is search
	script := `#!/bin/sh
while [ "$1" = "--index" ]; do shift 2; done
cmd="$1"
if [ "$cmd" = "query" ]; then
  echo "error: query expansion failed (Ollama connection refused)" >&2
  exit 1
fi
if [ "$cmd" = "search" ]; then
  cat <<EOF
[
  {
    "docid": "#bm25-fallback",
    "file": "qmd://daybook/notes/bm25.md",
    "score": 0.85,
    "line": 4,
    "title": "Fallback BM25 Note",
    "context": "Context for fallback",
    "snippet": "@@ -1,4 @@ fallback snippet"
  }
]
EOF
  exit 0
fi
exit 1
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	hits, err := Search(context.Background(), "fallback-query", []string{"daybook"}, "hybrid", 10)
	if err != nil {
		t.Fatalf("hybrid fallback failed: %v", err)
	}
	if len(hits) != 1 || hits[0].DocID != "#bm25-fallback" {
		t.Fatalf("unexpected fallback hits: %+v", hits)
	}
}

func TestSearchInvalidJSONOutput(t *testing.T) {
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	script := `#!/bin/sh
echo "fatal: database corrupted at offset 0x40"
exit 0
`
	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	_, err := Search(context.Background(), "invalid-json", []string{"daybook"}, "bm25", 10)
	if err == nil {
		t.Fatal("expected error on non-JSON output")
	}
	if !strings.Contains(err.Error(), "non-JSON output") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCollectionPathParsesShowOutput(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "qmd")
	script := `#!/bin/sh
while [ "$1" = "--index" ]; do shift 2; done
if [ "$1" != "collection" ] || [ "$2" != "show" ] || [ "$3" != "daybook" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
printf 'Collection: daybook\n  Path:     /tmp/writer/daybook\n  Pattern:  **/*.md\n'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	got, err := CollectionPath(context.Background(), "daybook")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/writer/daybook" {
		t.Fatalf("path = %q", got)
	}
}

func TestCollectionPathFailsClosed(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "qmd")
	script := `#!/bin/sh
echo "Collection not found: missing" >&2
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	if _, err := CollectionPath(context.Background(), "missing"); err == nil {
		t.Fatal("missing collection must fail")
	}
	if _, err := CollectionPath(context.Background(), ""); err == nil {
		t.Fatal("empty name must fail")
	}
}

func TestEmbedRejectsIncompleteBanners(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "qmd")
	script := `#!/bin/sh
echo "Done!"
echo "⚠ 3 chunks still failed after retries"
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	if err := Embed(context.Background(), "daybook"); err == nil {
		t.Fatal("incomplete embed must fail")
	}
}

func TestEmbedRequiresCompletionBanner(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "qmd")
	script := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	if err := Embed(context.Background(), "daybook"); err == nil {
		t.Fatal("missing banner must fail")
	}
}
