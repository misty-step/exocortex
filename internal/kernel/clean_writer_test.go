package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noOriginClone adds a clone WITHOUT a remote: writes there can never
// fall back to a clean-writer, so abort-semantics stay testable.
func (f *fixture) noOriginClone(t *testing.T, name string) string {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, name+"-clone")
	g(t, parent, "clone", f.origin, dir) // run from the EXISTING parent
	g(t, dir, "remote", "remove", "origin")
	c, err := Register(name, dir, "daybook", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f.cs = append(f.cs, *c)
	return dir
}

// provisionWriter dirties a tracked heartbeat-style file so the next
// put falls back to (and thereby provisions) the persistent writer,
// then restores the heartbeat to its committed state.
func provisionWriter(t *testing.T, f *fixture, shared string) {
	t.Helper()
	hb := filepath.Join(shared, "meta/loops-heartbeat.md")
	os.MkdirAll(filepath.Dir(hb), 0o755)
	os.WriteFile(hb, []byte("---\ntype: x\n---\nseed\n"), 0o644)
	g(t, shared, "add", "meta/loops-heartbeat.md")
	g(t, shared, "commit", "-m", "seed heartbeat")
	g(t, shared, "push")
	os.WriteFile(hb, []byte("---\ntype: x\n---\nheartbeat 05:00 append\n"), 0o644)
}

// Proof 15 (clean writer): when the shared checkout carries unrelated
// foreign UNSTAGED dirt (the heartbeat pattern), writes fall back to
// the cortex's persistent writer clone — landing, committing, pushing
// from there — while the heartbeat stays untouched and the post-push
// ff keeps the qmd-indexed shared tree current. Sticky selection keeps
// later writes and reads on the writer even once the heartbeat cleans.
func TestProof15CleanWriterFallsBackOnDirtyHeartbeat(t *testing.T) {
	f := newFixture(t)
	provisionWriter(t, f, f.a)
	if _, conf := f.put("hosta", "notes/real.md", mkNote("note", "v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	rev := f.rev("hosta", "notes/real.md")

	// Heartbeat append: unrelated foreign UNSTAGED dirt.
	hb := filepath.Join(f.a, "meta/loops-heartbeat.md")
	dirtyBytes, _ := os.ReadFile(hb)
	os.WriteFile(hb, []byte("---\ntype: x\n---\nheartbeat 05:00 append\n"), 0o644)

	// Update a DIFFERENT path: must succeed via the persistent writer.
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/real.md",
		Payload: []byte(mkNote("note", "v2 via clean writer")),
		Expects: rev,
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("clean-writer fallback failed: %s detail=%v", conf.Code, conf.Detail)
	}
	if !res.Pushed {
		t.Fatal("expected push from writer clone")
	}

	cfgDir, _ := ConfigDir()
	writer := filepath.Join(cfgDir, "writers", "hosta")
	wDisk, err := os.ReadFile(filepath.Join(writer, "notes/real.md"))
	if err != nil || !strings.Contains(string(wDisk), "v2 via clean writer") {
		t.Fatalf("writer clone missing the update: %v", err)
	}

	// Shared checkout untouched by the write: heartbeat byte-identical.
	after, _ := os.ReadFile(hb)
	if string(after) != string(dirtyBytes) {
		t.Fatal("foreign dirty file was touched")
	}

	// Post-push ff synced the REGISTERED checkout along unrelated
	// paths — this keeps the qmd-indexed tree current for search.
	diskSynced, _ := os.ReadFile(filepath.Join(f.a, "notes/real.md"))
	if !strings.Contains(string(diskSynced), "v2 via clean writer") {
		t.Fatalf("registered checkout not synced after push: %s", diskSynced)
	}

	// Reads use the sticky writer root: get returns the new revision
	// immediately, so the normal get -> put retry cycle works.
	got, conf := Get(f.cs, "hosta", "notes/real.md")
	if conf != nil || got.Revision != res.Revision {
		t.Fatalf("get stale after writer push: rev=%s want %s", got.Revision, res.Revision)
	}

	// Destination dirtied on the shared checkout AFTER provisioning:
	// still terminal dirty_destination — the registered checkout's own
	// destination state is never silently bypassed.
	os.WriteFile(filepath.Join(f.a, "notes/real.md"), []byte(mkNote("note", "human mid-edit on shared")), 0o644)
	resW, confW := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/real.md",
		Payload: []byte(mkNote("note", "should not land")),
		Expects: res.Revision,
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if confW == nil || confW.Code != "dirty_destination" {
		t.Fatalf("want dirty_destination post-provisioning, got %#v / %#v", confW, resW)
	}
	g(t, f.a, "checkout", "--", "notes/real.md")           // restore shared dest
	g(t, f.a, "checkout", "--", "meta/loops-heartbeat.md") // owner cleans heartbeat

	// Heartbeat cleaned: sticky writer root serves the next cycle.
	res2, conf2 := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/real.md",
		Payload: []byte(mkNote("note", "v3 incremental")),
		Expects: res.Revision,
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if conf2 != nil || !res2.Pushed {
		t.Fatalf("incremental writer write failed: %v / %v", conf2, res2)
	}
	got2, _ := Get(f.cs, "hosta", "notes/real.md")
	if !strings.Contains(got2.Content, "v3 incremental") {
		t.Fatal("get does not read the writer after provisioning")
	}
}

// A LEFTOVER-DIRTY WRITER clone must abort the write at the selected-
// root scan — never be stash-cycled or merged before the guard.
func TestDirtyExistingWriterAborts(t *testing.T) {
	f := newFixture(t)
	// Dirty heartbeat + a real different-path put: this is what
	// actually provisions the persistent writer clone.
	provisionWriter(t, f, f.a)
	if _, conf := f.put("hosta", "notes/provision.md", mkNote("note", "provisions writer")); conf != nil {
		t.Fatal(conf.Code)
	}
	if writerDir(&f.cs[0]) == "" {
		t.Fatal("writer clone was not provisioned")
	}
	g(t, f.a, "checkout", "--", "meta/loops-heartbeat.md")

	// Leftover dirt on a TRACKED file inside the writer itself.
	warm := filepath.Join(writerDir(&f.cs[0]), "README.md")
	os.WriteFile(warm, []byte("# fixture\nleftover mid-edit\n"), 0o644)

	_, conf := f.put("hosta", "notes/warm.md", mkNote("note", "second write"))
	wantCode(t, conf, "foreign_unstaged_state")
	paths, ok := conf.Detail["paths"].([]string)
	if !ok || len(paths) != 1 || paths[0] != "README.md" {
		t.Fatalf("paths = %#v", conf.Detail["paths"])
	}
	after, _ := os.ReadFile(warm)
	if !strings.Contains(string(after), "leftover mid-edit") {
		t.Fatal("writer leftover bytes changed")
	}
}
