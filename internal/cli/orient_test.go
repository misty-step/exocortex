package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestSearchReportsInvalidQMDOutput(t *testing.T) {
	installFakeQMD(t, `[{"score": 0. 89}]`)
	code, body, _ := runMain(t, "", "search", "September", "--mode", "bm25")
	if code != 1 || body["error"] != "search_unavailable" {
		t.Fatalf("exit=%d body=%v", code, body)
	}
	hint, _ := body["hint"].(string)
	if !strings.Contains(hint, "raw qmd --format json") || strings.Contains(hint, "indexed qmd collection") {
		t.Fatalf("misclassified decode hint: %q", hint)
	}
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
		t.Fatalf("decision filter leaked noise, dead notes, or unread paths: %v", decPaths)
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

type cliNote struct {
	path string
	body string
}

func cliGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runMainRaw(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb strings.Builder
	code := Main(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func setupCLIDaybook(t *testing.T, notes ...cliNote) (string, string) {
	t.Helper()
	if len(notes) == 0 {
		t.Fatal("setupCLIDaybook requires at least one note")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	human := filepath.Join(base, "human")
	cliGit(t, base, "init", "--bare", "-b", "master", origin)
	cliGit(t, base, "clone", origin, human)
	cliGit(t, human, "config", "user.email", "cli@test")
	cliGit(t, human, "config", "user.name", "CLI Test")
	if err := os.WriteFile(filepath.Join(human, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		writeNote(t, human, note.path, note.body)
	}
	cliGit(t, human, "add", ".")
	cliGit(t, human, "commit", "-m", "seed")
	cliGit(t, human, "push", "-u", "origin", "master")

	code, body, raw := runMain(t, "", "register", "vault", human, "--vcs", "daybook")
	if code != 0 {
		t.Fatalf("register daybook: exit=%d body=%v raw=%s", code, body, raw)
	}
	code, body, raw = runMain(t, "", "get", notes[0].path, "--cortex", "vault")
	if code != 0 {
		t.Fatalf("warm daybook publisher: exit=%d body=%v raw=%s", code, body, raw)
	}

	writers := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "exocortex", "writers")
	entries, err := os.ReadDir(writers)
	if err != nil {
		t.Fatal(err)
	}
	var writer string
	for _, entry := range entries {
		if entry.IsDir() {
			if writer != "" {
				t.Fatalf("multiple publisher writers in %s", writers)
			}
			writer = filepath.Join(writers, entry.Name())
		}
	}
	if writer == "" {
		t.Fatalf("publisher writer missing in %s", writers)
	}
	code, body, _ = runMain(t, "", "get", "notes/missing.md", "--cortex", "vault")
	if code != 1 || body["error"] != "not_found" {
		t.Fatalf("missing get: exit=%d body=%v", code, body)
	}
	return human, writer
}

func installCountingGit(t *testing.T, logPath string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "fetch" ]; then
  printf 'fetch\n' >> "$EXOCORTEX_CLI_FETCH_LOG"
fi
exec "$EXOCORTEX_CLI_REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXOCORTEX_CLI_FETCH_LOG", logPath)
	t.Setenv("EXOCORTEX_CLI_REAL_GIT", realGit)
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
}

func TestCLISearchAndBriefRejectDirtyPublisher(t *testing.T) {
	dirtCases := []struct {
		name   string
		marker string
		apply  func(*testing.T, string)
	}{
		{
			name:   "tracked",
			marker: "DIRTY_TRACKED",
			apply: func(t *testing.T, writer string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(writer, "README.md"), []byte("DIRTY_TRACKED"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "staged",
			marker: "DIRTY_STAGED",
			apply: func(t *testing.T, writer string) {
				t.Helper()
				const rel = "notes/staged-wip.md"
				if err := os.WriteFile(filepath.Join(writer, rel), []byte("DIRTY_STAGED"), 0o644); err != nil {
					t.Fatal(err)
				}
				cliGit(t, writer, "add", rel)
			},
		},
		{
			name:   "untracked",
			marker: "DIRTY_UNTRACKED",
			apply: func(t *testing.T, writer string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(writer, "notes/untracked-wip.md"), []byte("DIRTY_UNTRACKED"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	projections := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "topic", "--cortex", "vault", "--mode", "bm25"}},
		{name: "brief", args: []string{"brief", "topic", "--cortex", "vault"}},
	}
	hits := `[{"docid":"#1","file":"qmd://vault/notes/x.md","score":0.9,"line":1,"title":"x","context":"","snippet":"clean"}]`

	for _, dirt := range dirtCases {
		dirt := dirt
		for _, projection := range projections {
			projection := projection
			t.Run(dirt.name+"/"+projection.name, func(t *testing.T) {
				_, writer := setupCLIDaybook(t, cliNote{
					path: "notes/x.md",
					body: "---\ntype: note\nstatus: active\ndescription: clean\ncreated: 2026-08-21T00:00:00Z\n---\n\nclean committed bytes\n",
				})
				dirt.apply(t, writer)
				installFakeQMD(t, hits)

				code, raw, _ := runMainRaw(t, "", projection.args...)
				if code != 1 {
					t.Fatalf("%s exit=%d raw=%s", projection.name, code, raw)
				}
				var body map[string]any
				if err := json.Unmarshal([]byte(raw), &body); err != nil {
					t.Fatalf("%s conflict JSON: %v\n%s", projection.name, err, raw)
				}
				if body["error"] != "cortex_unavailable" {
					t.Fatalf("%s error=%v, want cortex_unavailable; body=%v", projection.name, body["error"], body)
				}
				if strings.Contains(raw, dirt.marker) {
					t.Fatalf("%s returned publisher dirt bytes: %s", projection.name, raw)
				}
			})
		}
	}
}

func TestCLISearchAndBriefFetchEachCortexOnce(t *testing.T) {
	projections := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "topic", "--cortex", "vault", "--mode", "bm25"}},
		{name: "brief", args: []string{"brief", "topic", "--cortex", "vault"}},
	}
	hits := `[
{"docid":"#1","file":"qmd://vault/notes/a.md","score":0.9,"line":1,"title":"a","context":"","snippet":"a"},
{"docid":"#2","file":"qmd://vault/notes/b.md","score":0.8,"line":1,"title":"b","context":"","snippet":"b"}
]`

	for _, projection := range projections {
		projection := projection
		t.Run(projection.name, func(t *testing.T) {
			_, _ = setupCLIDaybook(t,
				cliNote{path: "notes/a.md", body: "---\ntype: decision\nstatus: active\ndescription: a\ncreated: 2026-08-21T00:00:00Z\n---\n\n# A\n- a\n"},
				cliNote{path: "notes/b.md", body: "---\ntype: decision\nstatus: active\ndescription: b\ncreated: 2026-08-21T00:00:00Z\n---\n\n# B\n- b\n"},
			)
			installFakeQMD(t, hits)
			fetchLog := filepath.Join(t.TempDir(), "fetch.log")
			installCountingGit(t, fetchLog)

			code, raw, _ := runMainRaw(t, "", projection.args...)
			if code != 0 {
				t.Fatalf("%s exit=%d raw=%s", projection.name, code, raw)
			}
			if projection.name == "search" {
				if got := decodeHits(t, raw); len(got) != 2 {
					t.Fatalf("search hits=%d, want 2", len(got))
				}
			} else {
				var body map[string]any
				if err := json.Unmarshal([]byte(raw), &body); err != nil {
					t.Fatalf("brief JSON: %v\n%s", err, raw)
				}
				notes, ok := body["canonical_notes"].([]any)
				if !ok || len(notes) != 2 {
					t.Fatalf("brief canonical_notes=%v, want 2", body["canonical_notes"])
				}
			}
			logged, err := os.ReadFile(fetchLog)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(logged), "fetch\n"); got != 1 {
				t.Fatalf("%s fetch count=%d, want 1; log=%q", projection.name, got, logged)
			}
		})
	}
}
