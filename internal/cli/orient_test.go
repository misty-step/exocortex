package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeNote(t *testing.T, root, rel, body string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installFakeQMD(t *testing.T, hits string) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
cat <<'EOF'
` + hits + `
EOF
`
	if err := os.WriteFile(filepath.Join(binDir, "qmd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
}

func decodeHits(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var hits []map[string]any
	if err := json.Unmarshal([]byte(raw), &hits); err != nil {
		t.Fatalf("search JSON: %v\n%s", err, raw)
	}
	return hits
}

func hitPaths(hits []map[string]any) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		if p, ok := h["path"].(string); ok {
			out = append(out, p)
		}
	}
	return out
}

func hasPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

func TestSearchTypeAndBriefUseCortexPolicy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vault := t.TempDir()
	emma := t.TempDir()

	writeNote(t, vault, "misty-step/kernel.md", "---\ntype: note\nstatus: active\ndescription: live kernel\n---\n# Kernel\n- one owner\n")
	writeNote(t, vault, "projects/old.md", "---\ntype: note\nstatus: superseded\n---\n# Old plan\n")
	writeNote(t, vault, "meta/agents-board/memo/2026-08-25/a.md", "---\ntype: memo\n---\nmemo body\n")
	writeNote(t, vault, "meta/conversations/2026-08-25.md", "---\ntype: conversation\n---\nchat\n")
	writeNote(t, vault, "Clippings/book.md", "---\ntype: clipping\n---\nbook\n")
	writeNote(t, vault, "docs/adr/0001.md", "---\ntype: note\nstatus: active\n---\n# ADR\n")
	writeNote(t, emma, "daily/2026-08-25/n.md", "---\ntype: memo\n---\nemma memo\n")
	writeNote(t, emma, "keep.md", "---\ntype: decision\nstatus: active\ndescription: keep this\n---\n# Keep\n- second cortex\n")
	writeNote(t, emma, "dead.md", "---\ntype: decision\nstatus: archived\n---\n# Dead\n")
	writeNote(t, vault, "notes/shared.md", "---\ntype: decision\nstatus: active\n---\n# Vault shared\n")
	writeNote(t, emma, "notes/shared.md", "---\ntype: decision\nstatus: active\n---\n# Emma shared\n")

	code, body, raw := runMain(t, "", "register", "vault", vault, "--vcs", "none", "--journal-prefix", "meta/agents-board/memo")
	if code != 0 {
		t.Fatalf("register vault: %v %s", body, raw)
	}
	code, body, raw = runMain(t, "", "register", "emma", emma, "--vcs", "none", "--journal-prefix", "daily")
	if code != 0 {
		t.Fatalf("register emma: %v %s", body, raw)
	}

	installFakeQMD(t, `[
  {"docid":"#1","file":"qmd://vault/misty-step/kernel.md","score":0.9,"line":1,"title":"Kernel","context":"","snippet":"one owner"},
  {"docid":"#2","file":"qmd://vault/projects/old.md","score":0.8,"line":1,"title":"Old plan","context":"","snippet":"old"},
  {"docid":"#3","file":"qmd://vault/meta/agents-board/memo/2026-08-25/a.md","score":0.7,"line":1,"title":"memo","context":"","snippet":"memo"},
  {"docid":"#4","file":"qmd://vault/meta/conversations/2026-08-25.md","score":0.6,"line":1,"title":"chat","context":"","snippet":"chat"},
  {"docid":"#5","file":"qmd://vault/Clippings/book.md","score":0.5,"line":1,"title":"book","context":"","snippet":"book"},
  {"docid":"#6","file":"qmd://vault/docs/adr/0001.md","score":0.4,"line":1,"title":"ADR","context":"","snippet":"adr"},
  {"docid":"#7","file":"qmd://emma/daily/2026-08-25/n.md","score":0.3,"line":1,"title":"emma memo","context":"","snippet":"emma"},
  {"docid":"#8","file":"qmd://emma/keep.md","score":0.2,"line":1,"title":"Keep","context":"","snippet":"keep"},
  {"docid":"#9","file":"qmd://emma/dead.md","score":0.1,"line":1,"title":"Dead","context":"","snippet":"dead"},
  {"docid":"#10","file":"qmd://omp-sessions/2026/x.jsonl","score":0.05,"line":1,"title":"sess","context":"","snippet":"sess"},
  {"docid":"#11","file":"qmd://vault/notes/shared.md","score":0.04,"line":1,"title":"Vault shared","context":"","snippet":"shared"},
  {"docid":"#12","file":"qmd://emma/notes/shared.md","score":0.03,"line":1,"title":"Emma shared","context":"","snippet":"shared"}
]`)

	code, _, raw = runMain(t, "", "search", "topic", "--type", "memo", "--mode", "bm25", "--limit", "20")
	if code != 0 {
		t.Fatalf("search memo exit=%d %s", code, raw)
	}
	memoPaths := hitPaths(decodeHits(t, raw))
	if !hasPath(memoPaths, "meta/agents-board/memo/2026-08-25/a.md") || !hasPath(memoPaths, "daily/2026-08-25/n.md") {
		t.Fatalf("memo filter missed journal prefixes: %v", memoPaths)
	}
	if hasPath(memoPaths, "keep.md") || hasPath(memoPaths, "misty-step/kernel.md") {
		t.Fatalf("memo filter leaked non-memos: %v", memoPaths)
	}

	code, _, raw = runMain(t, "", "search", "topic", "--type", "decision", "--mode", "bm25", "--limit", "20")
	if code != 0 {
		t.Fatalf("search decision exit=%d %s", code, raw)
	}
	decPaths := hitPaths(decodeHits(t, raw))
	if !hasPath(decPaths, "misty-step/kernel.md") || !hasPath(decPaths, "keep.md") || !hasPath(decPaths, "docs/adr/0001.md") {
		t.Fatalf("decision filter missed live notes: %v", decPaths)
	}
	if hasPath(decPaths, "meta/agents-board/memo/2026-08-25/a.md") || hasPath(decPaths, "daily/2026-08-25/n.md") ||
		hasPath(decPaths, "Clippings/book.md") || hasPath(decPaths, "meta/conversations/2026-08-25.md") ||
		hasPath(decPaths, "projects/old.md") {
		t.Fatalf("decision filter leaked noise or dead notes: %v", decPaths)
	}

	code, _, raw = runMain(t, "", "search", "topic", "--type", "session", "--mode", "bm25", "--limit", "20")
	if code != 0 {
		t.Fatalf("search session exit=%d %s", code, raw)
	}
	sessPaths := hitPaths(decodeHits(t, raw))
	if !hasPath(sessPaths, "meta/conversations/2026-08-25.md") || !hasPath(sessPaths, "2026/x.jsonl") {
		t.Fatalf("session filter missed transcripts: %v", sessPaths)
	}

	code, body, raw = runMain(t, "", "brief", "topic", "--limit", "10")
	if code != 0 {
		t.Fatalf("brief exit=%d %s", code, raw)
	}
	notes, _ := body["canonical_notes"].([]any)
	var briefPaths []string
	sharedCortices := map[string]bool{}
	for _, n := range notes {
		m, _ := n.(map[string]any)
		briefPaths = append(briefPaths, fmtString(m["path"]))
		if fmtString(m["path"]) == "misty-step/kernel.md" && fmtString(m["cortex"]) != "vault" {
			t.Fatalf("brief cortex not canonical: %v", m)
		}
		if fmtString(m["path"]) == "notes/shared.md" {
			sharedCortices[fmtString(m["cortex"])] = true
		}
	}
	if !hasPath(briefPaths, "misty-step/kernel.md") || !hasPath(briefPaths, "keep.md") {
		t.Fatalf("brief missed live decisions: %v", briefPaths)
	}
	if !sharedCortices["vault"] || !sharedCortices["emma"] {
		t.Fatalf("brief same-rel must keep both cortices: %v notes=%v", sharedCortices, briefPaths)
	}
	for _, banned := range []string{
		"meta/agents-board/memo/2026-08-25/a.md",
		"daily/2026-08-25/n.md",
		"Clippings/book.md",
		"meta/conversations/2026-08-25.md",
		"projects/old.md",
		"dead.md",
	} {
		if hasPath(briefPaths, banned) {
			t.Fatalf("brief leaked %s: %v", banned, briefPaths)
		}
	}
}

func TestTypedSearchFailsClosedOnBadRegistry(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	exo := filepath.Join(cfg, "exocortex")
	if err := os.MkdirAll(exo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exo, "cortices.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	installFakeQMD(t, `[{"docid":"#1","file":"qmd://vault/daily/x.md","score":0.9,"line":1,"title":"m","context":"","snippet":"m"}]`)
	code, body, raw := runMain(t, "", "search", "topic", "--type", "memo", "--mode", "bm25")
	if code == 0 {
		t.Fatalf("typed search must fail closed on a bad registry: %s", raw)
	}
	if body["error"] == nil || body["error"] == "" {
		t.Fatalf("want an error payload, got %v", body)
	}
}
