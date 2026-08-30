package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runMain(t *testing.T, stdin string, args ...string) (int, map[string]any, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, strings.NewReader(stdin), &out, &errb)
	var body map[string]any
	raw := out.String()
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		var arr []any
		if aerr := json.Unmarshal([]byte(raw), &arr); aerr != nil {
			t.Fatalf("stdout is not JSON (exit %d):\n%s\nstderr: %s", code, raw, errb.String())
		}
	}
	return code, body, raw
}

func setupCortex(t *testing.T) string {
	t.Helper()
	// One shared config dir for the whole test so registrations persist.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := t.TempDir()
	if err := os.WriteFile(filepath.Join(r, "seed.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

// The pinned --json contract holds on EVERY exit path: success,
// operation conflict, and invalid input all emit a JSON document.
func TestUniformJSONContract(t *testing.T) {
	repo := setupCortex(t)

	// register with trailing --json parses and succeeds.
	code, body, _ := runMain(t, "", "register", "smoke", repo, "--json")
	if code != 0 || body["registered"].(map[string]any)["name"] != "smoke" {
		t.Fatalf("register: exit=%d body=%v", code, body)
	}

	// Duplicate registration -> JSON conflict, nonzero exit.
	code, body, _ = runMain(t, "", "register", "smoke", repo)
	if code == 0 || body["error"] != "duplicate_cortex" {
		t.Fatalf("duplicate register: exit=%d body=%v", code, body)
	}

	// Bare invocation emits JSON invalid_input on stdout.
	code, body, _ = runMain(t, "")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("no command: exit=%d body=%v", code, body)
	}

	// help is the documented human exception: usage text, exit 0, never
	// JSON — probed through the raw Main path, bypassing runMain's
	// strict JSON unmarshal.
	for _, form := range []string{"help", "-h", "--help"} {
		var out, errb bytes.Buffer
		code := Main([]string{form}, strings.NewReader(""), &out, &errb)
		if code != 0 || !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("%s: exit=%d out=%q", form, code, out.String())
		}
	}
	code, body, _ = runMain(t, "", "register", "smoke", repo, "--vcs")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("flag error: exit=%d body=%v", code, body)
	}

	// Missing positional -> JSON invalid_input.
	code, body, _ = runMain(t, "", "register", "onlyname")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("missing arg: exit=%d body=%v", code, body)
	}

	// Unknown command -> JSON, exit 2.
	code, body, _ = runMain(t, "", "frobnicate")
	if code != 2 || body["error"] != "unknown_command" {
		t.Fatalf("unknown command: exit=%d body=%v", code, body)
	}

	// put without --from -> JSON invalid_input.
	code, body, _ = runMain(t, "", "put", "notes/x.md", "--cortex", "smoke")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("put without from: exit=%d body=%v", code, body)
	}

	// put with unreadable payload file -> JSON payload_unreadable.
	code, body, _ = runMain(t, "", "put", "notes/x.md", "--from", "/nonexistent/draft.md", "--cortex", "smoke")
	if code == 0 || body["error"] != "payload_unreadable" {
		t.Fatalf("bad payload path: exit=%d body=%v", code, body)
	}

	// Happy put then get round trip still zero-exit JSON.
	payload := "---\ntype: note\nstatus: active\ncreated: 2026-08-21T00:00:00Z\n---\n\nbody\n"
	code, body, _ = runMain(t, payload, "put", "notes/x.md", "--from", "-", "--cortex", "smoke")
	if code != 0 || body["operation"] != "create" {
		t.Fatalf("create: exit=%d body=%v", code, body)
	}
	code, body, _ = runMain(t, "", "get", "notes/x.md", "--cortex", "smoke")
	if code != 0 || body["frontmatter"].(map[string]any)["provenance"] == nil {
		t.Fatalf("get: exit=%d body=%v", code, body)
	}

	// Bare put onto existing -> operation conflict exits 1 (not 2).
	code, body, _ = runMain(t, payload, "put", "notes/x.md", "--from", "-", "--cortex", "smoke")
	if code != 1 || body["error"] != "exists" {
		t.Fatalf("exists conflict: exit=%d body=%v", code, body)
	}

	// created immutability surfaces through the CLI as data.
	rev := bodyRevision(t, repo, "notes/x.md")
	tampered := "---\ntype: note\ncreated: 1999-01-01T00:00:00Z\n---\n\nbody\n"
	code, body, _ = runMain(t, tampered, "put", "notes/x.md", "--from", "-", "--expects", rev, "--cortex", "smoke")
	if code != 1 || body["error"] != "created_immutable" {
		t.Fatalf("created_immutable: exit=%d body=%v", code, body)
	}
	if body["stored"] != "2026-08-21T00:00:00Z" {
		t.Fatalf("stored value missing from body: %v", body)
	}
}

func bodyRevision(t *testing.T, repo, rel string) string {
	t.Helper()
	_, body, _ := runMain(t, "", "get", rel, "--cortex", "smoke")
	rev, _ := body["revision"].(string)
	if rev == "" {
		t.Fatalf("no revision for %s in %s", rel, repo)
	}
	return rev
}

func TestSearchAndBriefCLI(t *testing.T) {
	repo := setupCortex(t)
	code, body, _ := runMain(t, "", "register", "smoke", repo, "--json")
	if code != 0 {
		t.Fatalf("register failed: %v", body)
	}

	// Missing query/topic -> JSON invalid_input exit 2.
	code, body, _ = runMain(t, "", "search")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("search missing query: exit=%d body=%v", code, body)
	}

	code, body, _ = runMain(t, "", "brief")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("brief missing topic: exit=%d body=%v", code, body)
	}

	// Invalid flag -> JSON invalid_input exit 2.
	code, body, _ = runMain(t, "", "search", "topic", "--invalid-flag")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("search invalid flag: exit=%d body=%v", code, body)
	}
}

func TestSyncAndStatusCLI(t *testing.T) {
	repo := setupCortex(t)
	code, body, _ := runMain(t, "", "register", "smoke", repo, "--json")
	if code != 0 {
		t.Fatalf("register failed: %v", body)
	}

	code, body, _ = runMain(t, "", "sync", "daybok")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("sync positional: exit=%d body=%v", code, body)
	}
	code, body, _ = runMain(t, "", "status", "daybok")
	if code != 2 || body["error"] != "invalid_input" {
		t.Fatalf("status positional: exit=%d body=%v", code, body)
	}

	note := "---\ntype: note\ncreated: 2026-08-21T00:00:00Z\n---\n\ncli dirty marker\n"
	code, body, _ = runMain(t, note, "put", "notes/x.md", "--from", "-", "--cortex", "smoke")
	if code != 0 {
		t.Fatalf("put failed: exit=%d body=%v", code, body)
	}
	rev, _ := body["revision"].(string)
	if rev == "" {
		t.Fatal("put missing revision")
	}

	code, _, raw := runMain(t, "", "status", "--cortex", "smoke")
	if code != 0 {
		t.Fatalf("status failed: exit=%d %s", code, raw)
	}
	var st []map[string]any
	if err := json.Unmarshal([]byte(raw), &st); err != nil || len(st) != 1 {
		t.Fatalf("status json: %v %s", err, raw)
	}
	if st[0]["dirty"] != true || st[0]["dirty_commit"] != rev {
		t.Fatalf("status after none-vcs put = %v, want dirty %s", st[0], rev)
	}

	binDir := t.TempDir()
	fake := filepath.Join(binDir, "qmd")
	script := `#!/bin/sh
while [ "$1" = "--index" ]; do shift 2; done
if [ "$1" = "collection" ]; then
  printf 'Collection: %s\n  Path:     %s\n' "$3" "$EXOCORTEX_TEST_QMD_ROOT"
  exit 0
fi
if [ "$1" = "embed" ]; then
  echo "Done!"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	t.Setenv("EXOCORTEX_TEST_QMD_ROOT", repo)

	code, _, raw = runMain(t, "", "sync", "--cortex", "smoke")
	if code != 0 {
		t.Fatalf("sync failed: exit=%d %s", code, raw)
	}
	code, _, raw = runMain(t, "", "status", "--cortex", "smoke")
	if code != 0 {
		t.Fatalf("status after sync: exit=%d %s", code, raw)
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil || len(st) != 1 {
		t.Fatalf("status after sync json: %v %s", err, raw)
	}
	if st[0]["dirty"] != false || st[0]["synced_commit"] != rev {
		t.Fatalf("status after sync = %v, want clean %s", st[0], rev)
	}
}

func TestCLIAbsolutePathAndRegisterConflicts(t *testing.T) {
	repo := setupCortex(t)
	code, _, raw := runMain(t, "", "register", "box", repo, "--vcs", "none")
	if code != 0 {
		t.Fatalf("register: exit=%d %s", code, raw)
	}

	payload := "---\ntype: note\nstatus: active\ncreated: 2026-08-21T00:00:00Z\n---\n\nabs\n"
	abs := filepath.Join(repo, "notes", "abs.md")
	code, body, raw := runMain(t, payload, "put", abs, "--from", "-", "--cortex", "box")
	if code != 0 {
		t.Fatalf("explicit abs put: exit=%d %s", code, raw)
	}
	if body["path"] != "notes/abs.md" {
		t.Fatalf("explicit abs path=%v", body["path"])
	}
	code, body, _ = runMain(t, "", "get", abs, "--cortex", "box")
	if code != 0 || !strings.Contains(fmtString(body["content"]), "abs") {
		t.Fatalf("explicit abs get: exit=%d %v", code, body)
	}
	code, body, _ = runMain(t, "", "get", abs)
	if code != 0 || body["path"] != "notes/abs.md" {
		t.Fatalf("implicit abs get: exit=%d %v", code, body)
	}

	other := t.TempDir()
	code, body, _ = runMain(t, "", "register", "box", other, "--vcs", "none")
	if code == 0 || body["error"] != "duplicate_cortex" {
		t.Fatalf("name dup: exit=%d body=%v", code, body)
	}
	code, body, _ = runMain(t, "", "register", "other", repo, "--vcs", "none")
	if code == 0 || body["error"] != "duplicate_path" {
		t.Fatalf("path dup: exit=%d body=%v", code, body)
	}
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}

func TestCLISearchModeTable(t *testing.T) {
	setupCortex(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "cmd.log")
	script := `#!/bin/sh
while [ "$1" = "--index" ]; do shift 2; done
echo "$1" >> ` + logPath + `
echo '[]'
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "qmd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	cases := []struct {
		args []string
		cmd  string
	}{
		{[]string{"search", "q"}, "query"},
		{[]string{"search", "q", "--mode", "hybrid"}, "query"},
		{[]string{"search", "q", "--mode", "bm25"}, "search"},
		{[]string{"search", "q", "--mode", "vector"}, "vsearch"},
	}
	for _, tc := range cases {
		os.Remove(logPath)
		code, _, raw := runMain(t, "", tc.args...)
		if code != 0 {
			t.Fatalf("args %v exit=%d %s", tc.args, code, raw)
		}
		got, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("args %v log: %v", tc.args, err)
		}
		if strings.TrimSpace(string(got)) != tc.cmd {
			t.Errorf("args %v invoked %q, want %q", tc.args, strings.TrimSpace(string(got)), tc.cmd)
		}
	}
	code, body, _ := runMain(t, "", "search", "q", "--mode", "semantic")
	if code == 0 || body["error"] != "search_unavailable" {
		t.Fatalf("unknown mode: exit=%d body=%v", code, body)
	}
}

func TestSplitArgsUsesFlagSet(t *testing.T) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.String("from", "", "")
	fs.String("expects", "", "")
	fs.Bool("json", true, "")
	fs.Int("limit", 0, "")
	check := func(args, wantFlags, wantPos string) {
		t.Helper()
		flags, pos := splitArgs(strings.Fields(args), fs)
		gotFlags, gotPos := strings.Join(flags, " "), strings.Join(pos, " ")
		if gotFlags != wantFlags || gotPos != wantPos {
			t.Fatalf("%q → flags=%q pos=%q", args, gotFlags, gotPos)
		}
	}
	check("path.md --from -", "--from -", "path.md")
	check("--from - path.md", "--from -", "path.md")
	check("path.md --json extra", "--json", "path.md extra")
	check("--from=- path.md", "--from=-", "path.md")
	check("topic --limit 3 --json", "--limit 3 --json", "topic")
	check("--bogus taken path.md", "--bogus", "taken path.md")
	check("--from - -- -sneaky.md", "--from -", "-sneaky.md")
	check("path.md --expects abc", "--expects abc", "path.md")
}
