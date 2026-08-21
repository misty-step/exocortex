package cli

import (
	"bytes"
	"encoding/json"
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
		t.Fatalf("stdout is not JSON (exit %d):\n%s\nstderr: %s", code, raw, errb.String())
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

	// Malformed flag on register -> JSON invalid_input, exit 2.
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
