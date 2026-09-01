package kernel

import (
	"context"
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

func TestProof15SolePublisherIsolatesFromHumanCheckout(t *testing.T) {
	f := newFixture(t)

	// Dirty the human checkout with heartbeat, human edits, and staged work.
	hb := filepath.Join(f.a, "meta/loops-heartbeat.md")
	os.MkdirAll(filepath.Dir(hb), 0o755)
	os.WriteFile(hb, []byte("---\ntype: x\n---\nheartbeat 05:00 append\n"), 0o644)
	dirtyHB, _ := os.ReadFile(hb)

	humanEdit := filepath.Join(f.a, "notes/real.md")
	os.MkdirAll(filepath.Dir(humanEdit), 0o755)
	os.WriteFile(humanEdit, []byte("---\ntype: note\n---\n\nhuman working bytes\n"), 0o644)
	dirtyHuman, _ := os.ReadFile(humanEdit)

	stagedPeer := filepath.Join(f.a, "notes/staged-peer.md")
	os.WriteFile(stagedPeer, []byte("---\ntype: note\n---\n\nstaged peer\n"), 0o644)
	g(t, f.a, "add", "notes/staged-peer.md")

	// Put succeeds independently through the publisher clone.
	res, conf := Put(context.TODO(), f.cs, PutInput{
		CortexName: "hosta", Path: "notes/real.md",
		Payload: []byte(mkNote("note", "v1 via sole publisher")),
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("sole publisher put failed: %s detail=%v", conf.Code, conf.Detail)
	}
	if !res.Pushed {
		t.Fatal("expected push from publisher clone")
	}

	// Publisher clone holds the committed update.
	cfgDir, _ := ConfigDir()
	writer := filepath.Join(cfgDir, "writers", "hosta")
	wDisk, err := os.ReadFile(filepath.Join(writer, "notes/real.md"))
	if err != nil || !strings.Contains(string(wDisk), "v1 via sole publisher") {
		t.Fatalf("publisher clone missing update: %v", err)
	}

	// Human checkout is completely untouched byte-for-byte.
	afterHB, _ := os.ReadFile(hb)
	if string(afterHB) != string(dirtyHB) {
		t.Fatal("human heartbeat was mutated")
	}
	afterHuman, _ := os.ReadFile(humanEdit)
	if string(afterHuman) != string(dirtyHuman) {
		t.Fatal("human working file was overwritten")
	}

	// Reads resolve from the publisher clone.
	got, conf := Get(f.cs, "hosta", "notes/real.md")
	if conf != nil || got.Revision != res.Revision {
		t.Fatalf("get failed or stale: rev=%s want %s", got.Revision, res.Revision)
	}
	if !strings.Contains(got.Content, "v1 via sole publisher") {
		t.Fatalf("get content = %q, want publisher content", got.Content)
	}
}

func TestDirtyPublisherAborts(t *testing.T) {
	f := newFixture(t)

	// Initial put provisions the publisher clone.
	if _, conf := f.put("hosta", "notes/provision.md", mkNote("note", "provisions writer")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := writerDir(&f.cs[0])
	if writer == "" {
		t.Fatal("publisher was not provisioned")
	}

	// Dirty a tracked file inside the publisher clone.
	warm := filepath.Join(writer, "README.md")
	os.WriteFile(warm, []byte("# fixture\nleftover mid-edit in publisher\n"), 0o644)

	_, conf := f.put("hosta", "notes/warm.md", mkNote("note", "second write"))
	wantCode(t, conf, "foreign_unstaged_state")
	paths, ok := conf.Detail["paths"].([]string)
	if !ok || len(paths) != 1 || paths[0] != "README.md" {
		t.Fatalf("paths = %#v", conf.Detail["paths"])
	}
	after, _ := os.ReadFile(warm)
	if !strings.Contains(string(after), "leftover mid-edit in publisher") {
		t.Fatal("publisher leftover bytes changed")
	}
}

func TestPublisherPinsNonDefaultTrackedBranch(t *testing.T) {
	f := newFixture(t)
	// Create and checkout non-default branch on hosta
	g(t, f.a, "checkout", "-b", "feature-vault")
	g(t, f.a, "push", "-u", "origin", "feature-vault")

	res, conf := Put(context.TODO(), f.cs, PutInput{
		CortexName: "hosta", Path: "notes/branch-test.md",
		Payload: []byte(mkNote("note", "landed on feature-vault")),
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if conf != nil || !res.Pushed {
		t.Fatalf("put failed on non-default branch: %v / %v", conf, res)
	}

	cfgDir, _ := ConfigDir()
	writer := filepath.Join(cfgDir, "writers", "hosta")
	branch := g(t, writer, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "feature-vault" {
		t.Fatalf("publisher branch = %q, want feature-vault", branch)
	}

	// Verify remote received the commit on feature-vault, not master
	remoteCommit := g(t, f.origin, "rev-parse", "feature-vault")
	if res.Commit != remoteCommit {
		t.Fatalf("remote feature-vault = %s, want %s", remoteCommit, res.Commit)
	}
}

func TestProof17FreshRegistrationReadsCommittedStateNotHumanDirt(t *testing.T) {
	testConfigEnv(t)
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	human := filepath.Join(base, "human")

	g(t, base, "init", "--bare", "-b", "master", origin)
	g(t, base, "clone", origin, human)
	g(t, human, "config", "user.email", "ci@exocortex.test")
	g(t, human, "config", "user.name", "exocortex-test")
	os.MkdirAll(filepath.Join(human, "notes"), 0o755)
	os.WriteFile(filepath.Join(human, "notes/committed.md"), []byte(mkNote("note", "committed seed")), 0o644)
	g(t, human, "add", "notes/committed.md")
	g(t, human, "commit", "-m", "seed")

	g(t, human, "push", "-u", "origin", "master")

	// Human now edits the file locally without committing.
	os.WriteFile(filepath.Join(human, "notes/committed.md"), []byte(mkNote("note", "uncommitted human dirt")), 0o644)

	// Register fresh cortex.
	c, err := Register("fresh", human, "daybook", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Get without any prior put must return the clean committed state.
	got, conf := Get([]Cortex{*c}, "fresh", "notes/committed.md")
	if conf != nil {
		t.Fatalf("get failed on fresh registration: %v", conf)
	}
	if !strings.Contains(got.Content, "committed seed") {
		t.Fatalf("got content = %q, want committed seed", got.Content)
	}
	if strings.Contains(got.Content, "uncommitted human dirt") {
		t.Fatal("get read uncommitted human dirt on fresh registration")
	}

	// effectiveRoot returns the provisioned publisher clone, not human.
	root, err := effectiveRoot(c)
	if err != nil {
		t.Fatal(err)
	}
	if root == human {
		t.Fatal("effectiveRoot returned human checkout instead of publisher clone")
	}
}

func TestProof18ProvisioningFailureFailsClosed(t *testing.T) {
	testConfigEnv(t)
	base := t.TempDir()
	human := filepath.Join(base, "human")
	os.MkdirAll(filepath.Join(human, "notes"), 0o755)
	os.WriteFile(filepath.Join(human, "notes/secret.md"), []byte("human uncommitted secret"), 0o644)
	g(t, base, "init", "-b", "master", human)
	g(t, human, "remote", "add", "origin", "file:///nonexistent/broken.git")

	c, err := Register("broken", human, "daybook", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Get must fail closed with cortex_unavailable, never returning the human file.
	_, conf := Get([]Cortex{*c}, "broken", "notes/secret.md")
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on get, got %#v", conf)
	}

	// Log and Lint must also fail closed.
	_, conf = Log([]Cortex{*c}, "broken", "notes/secret.md", 10)
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on log, got %#v", conf)
	}

	_, conf = Lint([]Cortex{*c}, "broken", "notes/secret.md")
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on lint, got %#v", conf)
	}
}

func TestProof19MissingOriginFailsClosedAcrossReadsAndWrites(t *testing.T) {
	testConfigEnv(t)
	base := t.TempDir()
	human := filepath.Join(base, "human-no-origin")
	os.MkdirAll(filepath.Join(human, "notes"), 0o755)
	os.WriteFile(filepath.Join(human, "notes/secret.md"), []byte("human uncommitted secret"), 0o644)
	g(t, base, "init", "-b", "master", human)

	c, err := Register("no-origin", human, "daybook", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Reads must fail closed.
	_, conf := Get([]Cortex{*c}, "no-origin", "notes/secret.md")
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on get with missing origin, got %#v", conf)
	}

	_, conf = Log([]Cortex{*c}, "no-origin", "notes/secret.md", 10)
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on log with missing origin, got %#v", conf)
	}

	_, conf = Lint([]Cortex{*c}, "no-origin", "notes/secret.md")
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("want cortex_unavailable on lint with missing origin, got %#v", conf)
	}

	// Writes must fail closed.
	_, pconf := Put(context.TODO(), []Cortex{*c}, PutInput{
		CortexName: "no-origin", Path: "notes/secret.md",
		Payload: []byte(mkNote("note", "must not write")),
		Agent:   "a", Via: "cli", OwnPayload: true,
	})
	if pconf == nil || pconf.Code != "writer_unavailable" {
		t.Fatalf("want writer_unavailable on put with missing origin, got %#v", pconf)
	}

	// Verify the local file in human checkout was never modified.
	disk, _ := os.ReadFile(filepath.Join(human, "notes/secret.md"))
	if string(disk) != "human uncommitted secret" {
		t.Fatal("local file in human checkout was mutated")
	}
}
