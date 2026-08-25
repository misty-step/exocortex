package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// binPath is the built exocortex binary used for the MCP stdio
// round-trip; TestMain builds it once for the package.
var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "exocortex-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tempdir: %v\n", err)
		os.Exit(1)
	}
	binPath = filepath.Join(tmp, "exocortex")
	build := exec.Command("go", "build", "-o", binPath, "github.com/misty-step/exocortex/cmd/exocortex")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build exocortex: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// Proof 7: MCP face round-trip — get → put(expectedRevision) → get
// bumps the revision and preserves the payload apart from the stamp.
func TestProof7MCPRoundTrip(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "kernel-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(binPath, "mcp")}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	call := func(name string, args map[string]any) (map[string]any, bool) {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("%s: empty content", name)
		}
		tc, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("%s: non-text content %T", name, res.Content[0])
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
			t.Fatalf("%s: body not JSON: %v\n%s", name, err, tc.Text)
		}
		return body, res.IsError
	}

	payload := mkNote("note", "mcp roundtrip body")
	body, isErr := call("exocortex_put", map[string]any{
		"path": "notes/mcp.md", "content": payload, "cortex": "hosta", "agent": "mcp-agent",
	})
	if isErr {
		t.Fatalf("create failed: %v", body)
	}
	rev1, _ := body["revision"].(string)
	if rev1 == "" {
		t.Fatalf("missing revision in %v", body)
	}

	got, isErr := call("exocortex_get", map[string]any{"path": "notes/mcp.md", "cortex": "hosta"})
	if isErr || got["revision"] != rev1 {
		t.Fatalf("get mismatch: isErr=%v body=%v", isErr, got)
	}

	updated := mkNote("note", "mcp roundtrip v2")
	body, isErr = call("exocortex_put", map[string]any{
		"path": "notes/mcp.md", "content": updated,
		"expectedRevision": rev1, "cortex": "hosta", "agent": "mcp-agent",
	})
	if isErr {
		t.Fatalf("update failed: %v", body)
	}
	rev2, _ := body["revision"].(string)
	if rev2 == "" || rev2 == rev1 {
		t.Fatalf("revision must bump: %q -> %q", rev1, rev2)
	}

	disk, _ := os.ReadFile(filepath.Join(mustEffectiveRoot(&f.cs[0]), "notes/mcp.md"))
	if !strings.Contains(string(disk), "mcp roundtrip v2") {
		t.Fatal("payload bytes lost")
	}
	if !strings.Contains(string(disk), "agent: mcp-agent") || !strings.Contains(string(disk), "via: mcp") {
		t.Fatalf("provenance stamp missing:\n%s", disk)
	}

	// Stale expectedRevision surfaces the pinned conflict as an error result.
	body, isErr = call("exocortex_put", map[string]any{
		"path": "notes/mcp.md", "content": updated,
		"expectedRevision": rev1, "cortex": "hosta",
	})
	if !isErr || body["error"] != "revision_conflict" {
		t.Fatalf("want revision_conflict error result, got isErr=%v body=%v", isErr, body)
	}
	if body["actual"] != rev2 {
		t.Fatalf("actual = %v, want %q", body["actual"], rev2)
	}
}

func TestMCPSyncAndStatus(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	res, conf := f.put("hosta", "notes/mcp-sync.md", mkNote("note", "mcp sync proof"))
	if conf != nil {
		t.Fatal(conf)
	}
	alignMockCollection(t, f.cs[0])

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "kernel-test", Version: "v0"}, nil)
	cmd := exec.Command(binPath, "mcp")
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	callSlice := func(name string, args map[string]any) ([]map[string]any, bool) {
		t.Helper()
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(out.Content) == 0 {
			t.Fatalf("%s: empty content", name)
		}
		tc, ok := out.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("%s: non-text %T", name, out.Content[0])
		}
		var body []map[string]any
		if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
			t.Fatalf("%s: want JSON array: %v\n%s", name, err, tc.Text)
		}
		return body, out.IsError
	}

	st, isErr := callSlice("exocortex_status", map[string]any{"cortex": "hosta"})
	if isErr || len(st) != 1 || st[0]["dirty"] != true || st[0]["dirty_commit"] != res.Commit {
		t.Fatalf("status before sync: isErr=%v body=%v want dirty %s", isErr, st, res.Commit)
	}

	syncRes, isErr := callSlice("exocortex_sync", map[string]any{"cortex": "hosta"})
	if isErr || len(syncRes) != 1 || syncRes[0]["updated"] != true || syncRes[0]["dirty_cleared"] != true {
		t.Fatalf("sync: isErr=%v body=%v", isErr, syncRes)
	}

	st, isErr = callSlice("exocortex_status", map[string]any{"cortex": "hosta"})
	if isErr || len(st) != 1 || st[0]["dirty"] != false || st[0]["synced_commit"] != res.Commit {
		t.Fatalf("status after sync: isErr=%v body=%v", isErr, st)
	}
}

func TestMCPRegisterDuplicateIsConflict(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "kernel-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(binPath, "mcp")}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "exocortex_register",
		Arguments: map[string]any{"name": "hosta", "path": f.a},
	})
	if err != nil {
		t.Fatalf("register call: %v", err)
	}
	if !res.IsError {
		t.Fatal("duplicate register must be a tool error result")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content %T", res.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &body); err != nil {
		t.Fatalf("body: %v\n%s", err, tc.Text)
	}
	if body["error"] != "duplicate_cortex" {
		t.Fatalf("body=%v", body)
	}
}
