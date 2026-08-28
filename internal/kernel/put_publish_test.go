package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPushError(t *testing.T) {
	cases := []struct {
		stderr string
		want   pushOutcome
	}{
		{" ! [rejected] master -> master (non-fast-forward)\nerror: failed to push some refs", pushMoved},
		{"hint: Updates were rejected because the tip of your current branch is behind", pushMoved},
		{" ! [rejected] master -> master (non-fast-forward)\nhint: Updates were rejected because the remote contains work that you do", pushMoved},
		{" ! [remote rejected] master -> master (pre-receive hook declined)", pushRefused},
		{"remote: error: cannot lock ref 'refs/heads/master'\n ! [remote rejected] master -> master (failed to update ref)", pushMoved},
		{" ! [remote rejected] main -> main (push declined due to repository rule violations)", pushRefused},
		{"remote: pre-receive hook declined\nerror: failed to push some refs", pushRefused},
		{"fatal: Authentication failed for 'https://example.test/repo.git'", pushRefused},
		{"Permission denied (publickey).", pushRefused},
		{"fatal: could not read Username for 'https://example.test': terminal prompts disabled", pushRefused},
		{"fatal: the remote end hung up unexpectedly", pushUnknown},
		{"fatal: unable to access 'https://example.invalid/': Could not resolve host", pushUnknown},
		{"error: RPC failed; curl 28 Timeout was reached", pushUnknown},
		{"", pushUnknown},
	}
	for _, tc := range cases {
		got := classifyPushError(gitErr(tc.stderr))
		if got != tc.want {
			t.Fatalf("stderr %q: got %v want %v", tc.stderr, got, tc.want)
		}
	}
}

func TestDifferentPathCreatesBothLand(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	beforePushHook = func() {
		if _, conf := f.put("hosta", "notes/peer.md", mkNote("note", "peer landed first")); conf != nil {
			t.Errorf("peer create failed: %v", conf.Code)
		}
	}
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/mine.md",
		Payload: []byte(mkNote("note", "unrelated create")),
		Agent:   "agent-b", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("unrelated create must land, got %s: %+v", conf.Code, conf.Detail)
	}
	if res == nil || !res.Pushed {
		t.Fatalf("want pushed create, got %+v", res)
	}
	rootB := mustEffectiveRoot(&f.cs[1])
	if !strings.Contains(string(mustRead(filepath.Join(rootB, "notes/mine.md"))), "unrelated create") {
		t.Fatal("loser's own path missing after replay")
	}
	if !strings.Contains(string(mustRead(filepath.Join(rootB, "notes/peer.md"))), "peer landed first") {
		t.Fatal("peer's path missing after converge")
	}
}

func TestReplayFailurePreservesStdinPayload(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil; afterConvergeHook = nil })
	beforePushHook = func() {
		if _, conf := f.put("hosta", "notes/peer.md", mkNote("note", "peer landed first")); conf != nil {
			t.Errorf("peer create failed: %v", conf.Code)
		}
	}
	afterConvergeHook = func() {
		writer := mustEffectiveRoot(&f.cs[1])
		notes := filepath.Join(writer, "notes")
		if err := os.Chmod(notes, 0o555); err != nil {
			t.Errorf("chmod notes: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(notes, 0o755) })
	}
	payload := []byte(mkNote("note", "stdin payload must survive replay write_failed"))
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/mine.md",
		Payload: payload, Agent: "agent-b", Via: "mcp", OwnPayload: false,
	})
	wantCode(t, conf, "write_failed")
	saved, _ := conf.Detail["payload_saved"].(string)
	if saved == "" {
		t.Fatalf("payload not preserved: %v", conf.Detail)
	}
	raw, err := os.ReadFile(saved)
	if err != nil || !strings.Contains(string(raw), "stdin payload must survive replay write_failed") {
		t.Fatalf("preserved payload wrong: %v %s", err, raw)
	}
}

func TestDifferentPathUpdatesBothLand(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/peer.md", mkNote("note", "peer v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	if _, conf := f.put("hostb", "notes/mine.md", mkNote("note", "mine v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	g(t, f.a, "pull", "--ff-only")
	g(t, f.b, "pull", "--ff-only")
	peerRev := f.rev("hosta", "notes/peer.md")
	mineRev := f.rev("hostb", "notes/mine.md")
	beforePushHook = func() {
		if _, conf := Put(nil, f.cs, PutInput{
			CortexName: "hosta", Path: "notes/peer.md",
			Payload: []byte(mkNote("note", "peer v2")), Expects: peerRev,
			Agent: "agent-a", Via: "cli", OwnPayload: true,
		}); conf != nil {
			t.Errorf("peer update failed: %v", conf.Code)
		}
	}
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/mine.md",
		Payload: []byte(mkNote("note", "mine v2")), Expects: mineRev,
		Agent: "agent-b", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("unrelated update must land, got %s expected=%v actual=%v", conf.Code, conf.Detail["expected"], conf.Detail["actual"])
	}
	if res == nil || !res.Pushed {
		t.Fatalf("want pushed update, got %+v", res)
	}
	rootB := mustEffectiveRoot(&f.cs[1])
	if !strings.Contains(string(mustRead(filepath.Join(rootB, "notes/mine.md"))), "mine v2") {
		t.Fatal("update of unchanged path lost")
	}
	if !strings.Contains(string(mustRead(filepath.Join(rootB, "notes/peer.md"))), "peer v2") {
		t.Fatal("peer update missing after converge")
	}
}

func TestPublishRejectedHook(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if err := writeRejectHook(f.origin); err != nil {
		t.Fatal(err)
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/hooked.md",
		Payload: []byte(mkNote("note", "must not pretend this exists")),
		Agent:   "agent-a", Via: "cli", OwnPayload: false,
	})
	wantCode(t, conf, "publish_rejected")
	if conf.Class() != ClassUnavailable {
		t.Fatalf("class=%v want Unavailable", conf.Class())
	}
	if conf.Detail["push_stderr"] == "" {
		t.Fatal("publish_rejected must carry push diagnostics")
	}
	saved, _ := conf.Detail["payload_saved"].(string)
	if saved == "" {
		t.Fatalf("payload not preserved: %v", conf.Detail)
	}
	rootA := mustEffectiveRoot(&f.cs[0])
	if _, err := os.Stat(filepath.Join(rootA, "notes/hooked.md")); !os.IsNotExist(err) {
		t.Fatalf("refused create must unwind; stat=%v", err)
	}
	remote := g(t, f.origin, "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(remote, "notes/hooked.md") {
		t.Fatal("refused push must not land on origin")
	}
}

func TestPublishUnknownRecoversWhenLanded(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	pushOverride = func() error {
		writer := mustEffectiveRoot(&f.cs[0])
		if _, err := git(writer, "push"); err != nil {
			return err
		}
		return gitErr("fatal: the remote end hung up unexpectedly")
	}
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/lost-ack.md",
		Payload: []byte(mkNote("note", "landed but ack lost")),
		Agent:   "agent-a", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("matching remote bytes must be success, got %s %+v", conf.Code, conf.Detail)
	}
	if res == nil || !res.Pushed {
		t.Fatalf("want proved push, got %+v", res)
	}
	rootA := mustEffectiveRoot(&f.cs[0])
	if !strings.Contains(string(mustRead(filepath.Join(rootA, "notes/lost-ack.md"))), "landed but ack lost") {
		t.Fatal("proved success lost local bytes")
	}
}

func TestLostAckDoesNotSucceedWhenWriterMissesTip(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/foreign.md", mkNote("note", "foreign v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := mustEffectiveRoot(&f.cs[0])
	beforePushHook = func() {
		if err := os.WriteFile(filepath.Join(writer, "notes/foreign.md"), []byte(mkNote("note", "local dirty foreign")), 0o644); err != nil {
			t.Errorf("dirty foreign: %v", err)
		}
	}
	pushOverride = func() error {
		if _, err := git(writer, "push"); err != nil {
			return err
		}
		g(t, f.b, "pull", "--ff-only")
		if err := os.WriteFile(filepath.Join(f.b, "notes/foreign.md"), []byte(mkNote("note", "peer foreign v2")), 0o644); err != nil {
			return err
		}
		g(t, f.b, "add", "notes/foreign.md")
		g(t, f.b, "commit", "-m", "peer advances foreign")
		g(t, f.b, "push")
		return gitErr("fatal: the remote end hung up unexpectedly")
	}
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/lost-ack.md",
		Payload: []byte(mkNote("note", "landed but writer blocked")),
		Agent:   "agent-a", Via: "cli", OwnPayload: true,
	})
	if res != nil && res.Pushed {
		t.Fatal("must not report pushed when writer missed the fetched tip")
	}
	wantCode(t, conf, "writer_unavailable")
	if conf.Class() != ClassUnavailable {
		t.Fatalf("class=%v want Unavailable", conf.Class())
	}
	if conf.Detail["remote"] != "landed" {
		t.Fatalf("remote=%v want landed", conf.Detail["remote"])
	}
	if strings.Contains(conf.Hint, "re-read with get") {
		t.Fatalf("landed hint must not send callers to get: %q", conf.Hint)
	}
	if !strings.Contains(conf.Hint, "proved_commit") || !strings.Contains(conf.Hint, "do not retry") {
		t.Fatalf("landed hint must name proved identity and forbid retry: %q", conf.Hint)
	}
	if conf.Detail["proved_commit"] == nil || conf.Detail["proved_revision"] == nil {
		t.Fatalf("proved remote state missing: %v", conf.Detail)
	}
	remote := g(t, f.origin, "ls-tree", "-r", "--name-only", "HEAD")
	if !strings.Contains(remote, "notes/lost-ack.md") {
		t.Fatal("candidate must remain on origin")
	}
	if _, err := os.Stat(filepath.Join(writer, "notes/lost-ack.md")); !os.IsNotExist(err) {
		t.Fatalf("writer get must not show the candidate after failed converge; stat=%v", err)
	}
}

func TestMovedDoesNotClaimLandedWhenWriterMissesTip(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/foreign.md", mkNote("note", "foreign v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := mustEffectiveRoot(&f.cs[0])
	beforePushHook = func() {
		if err := os.WriteFile(filepath.Join(writer, "notes/foreign.md"), []byte(mkNote("note", "local dirty foreign")), 0o644); err != nil {
			t.Errorf("dirty foreign: %v", err)
		}
		g(t, f.b, "pull", "--ff-only")
		if err := os.WriteFile(filepath.Join(f.b, "notes/foreign.md"), []byte(mkNote("note", "peer foreign v2")), 0o644); err != nil {
			t.Errorf("peer foreign: %v", err)
		}
		g(t, f.b, "add", "notes/foreign.md")
		g(t, f.b, "commit", "-m", "peer advances foreign")
		g(t, f.b, "push")
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/mine.md",
		Payload: []byte(mkNote("note", "unrelated create")),
		Agent:   "agent-a", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "writer_unavailable")
	if conf.Detail["remote"] != "rejected" {
		t.Fatalf("remote=%v want rejected", conf.Detail["remote"])
	}
	remote := g(t, f.origin, "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(remote, "notes/mine.md") {
		t.Fatal("rejected nff must not land the candidate")
	}
}

func TestMovedPathChangeDoesNotHashStaleLocalWhenConvergeFails(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/shared.md", mkNote("note", "R1")); conf != nil {
		t.Fatal(conf.Code)
	}
	if _, conf := f.put("hosta", "notes/foreign.md", mkNote("note", "foreign v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	g(t, f.b, "pull", "--ff-only")
	r1 := f.rev("hosta", "notes/shared.md")
	writer := mustEffectiveRoot(&f.cs[0])
	beforePushHook = func() {
		if err := os.WriteFile(filepath.Join(writer, "notes/foreign.md"), []byte(mkNote("note", "local dirty foreign")), 0o644); err != nil {
			t.Errorf("dirty foreign: %v", err)
		}
		if _, conf := Put(nil, f.cs, PutInput{
			CortexName: "hostb", Path: "notes/shared.md",
			Payload: []byte(mkNote("note", "peer wins shared")), Expects: r1,
			Agent: "agent-b", Via: "cli", OwnPayload: true,
		}); conf != nil {
			t.Errorf("peer shared put: %v", conf.Code)
		}
		if _, conf := Put(nil, f.cs, PutInput{
			CortexName: "hostb", Path: "notes/foreign.md",
			Payload: []byte(mkNote("note", "peer foreign v2")), Expects: f.rev("hostb", "notes/foreign.md"),
			Agent: "agent-b", Via: "cli", OwnPayload: true,
		}); conf != nil {
			t.Errorf("peer foreign put: %v", conf.Code)
		}
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/shared.md",
		Payload: []byte(mkNote("note", "loser shared")), Expects: r1,
		Agent: "agent-a", Via: "cli", OwnPayload: true,
	})
	if conf != nil && conf.Code == "revision_conflict" && conf.Detail["actual"] == conf.Detail["expected"] {
		t.Fatal("stale local actual==expected is a false publication conflict")
	}
	wantCode(t, conf, "writer_unavailable")
	if conf.Detail["remote"] != "rejected" {
		t.Fatalf("remote=%v want rejected", conf.Detail["remote"])
	}
}

func TestPublishUnknownWhenNotLanded(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	pushOverride = func() error {
		return gitErr("fatal: unable to access 'https://example.invalid/': Could not resolve host")
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/ghost.md",
		Payload: []byte(mkNote("note", "never reached the remote")),
		Agent:   "agent-a", Via: "mcp", OwnPayload: false,
	})
	wantCode(t, conf, "publish_unknown")
	if conf.Class() != ClassUnavailable {
		t.Fatalf("class=%v want Unavailable", conf.Class())
	}
	if _, ok := conf.Detail["payload_saved"].(string); !ok {
		t.Fatalf("payload not preserved: %v", conf.Detail)
	}
	rootA := mustEffectiveRoot(&f.cs[0])
	if _, err := os.Stat(filepath.Join(rootA, "notes/ghost.md")); !os.IsNotExist(err) {
		t.Fatalf("unlanded unknown must unwind when fetch works; stat=%v", err)
	}
}

func TestNoteDoesNotMintSecondPathOnPublishUnknown(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	var pushes int
	pushOverride = func() error {
		pushes++
		pushOverride = func() error {
			pushes++
			return gitErr("fatal: the remote end hung up unexpectedly")
		}
		return gitErr("fatal: the remote end hung up unexpectedly")
	}
	_, conf := Note(nil, f.cs, NoteInput{CortexName: "hosta", Text: "do not duplicate me", Agent: "agent-a", Via: "cli"})
	wantCode(t, conf, "publish_unknown")
	if pushes != 1 {
		t.Fatalf("note minted another path: push attempts=%d", pushes)
	}
}

func TestUnknownFetchFailureDoesNotRebaseOnNextPut(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/seed.md", mkNote("note", "seed")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := mustEffectiveRoot(&f.cs[0])
	originURL := g(t, writer, "remote", "get-url", "origin")
	pushOverride = func() error {
		gone := filepath.Join(t.TempDir(), "gone.git")
		if _, err := git(writer, "remote", "set-url", "origin", gone); err != nil {
			return err
		}
		return gitErr("fatal: unable to access 'https://example.invalid/': Could not resolve host")
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/ghost.md",
		Payload: []byte(mkNote("note", "unpublished candidate")),
		Agent:   "agent-a", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "publish_unknown")
	if unwound, _ := conf.Detail["unwound"].(bool); unwound {
		t.Fatal("fetch failure must keep the local candidate")
	}
	g(t, writer, "remote", "set-url", "origin", originURL)
	if _, conf := f.put("hostb", "notes/peer.md", mkNote("note", "origin advanced")); conf != nil {
		t.Fatal(conf.Code)
	}
	_, conf = f.put("hosta", "notes/second.md", mkNote("note", "must not rebase ghost"))
	wantCode(t, conf, "refresh_failed")
	remote := g(t, f.origin, "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(remote, "notes/ghost.md") {
		t.Fatal("unpublished candidate was published by a later put")
	}
	if strings.Contains(remote, "notes/second.md") {
		t.Fatal("diverged writer wrote a second path")
	}
	if !strings.Contains(string(mustRead(filepath.Join(writer, "notes/ghost.md"))), "unpublished candidate") {
		t.Fatal("local candidate was rebased or dropped")
	}
}

func TestUnknownFetchFailureAheadDoesNotPublishOnNextPut(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() { beforePushHook = nil; pushOverride = nil })
	if _, conf := f.put("hosta", "notes/seed.md", mkNote("note", "seed")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := mustEffectiveRoot(&f.cs[0])
	originURL := g(t, writer, "remote", "get-url", "origin")
	pushOverride = func() error {
		gone := filepath.Join(t.TempDir(), "gone.git")
		if _, err := git(writer, "remote", "set-url", "origin", gone); err != nil {
			return err
		}
		return gitErr("fatal: unable to access 'https://example.invalid/': Could not resolve host")
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/ghost.md",
		Payload: []byte(mkNote("note", "unpublished candidate")),
		Agent:   "agent-a", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "publish_unknown")
	g(t, writer, "remote", "set-url", "origin", originURL)
	_, conf = f.put("hosta", "notes/second.md", mkNote("note", "must not publish ghost"))
	wantCode(t, conf, "refresh_failed")
	remote := g(t, f.origin, "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(remote, "notes/ghost.md") || strings.Contains(remote, "notes/second.md") {
		t.Fatalf("ahead writer published on next put:\n%s", remote)
	}
}

func gitErr(stderr string) error {
	return &GitError{Args: []string{"push"}, Stderr: stderr, Err: errExit1{}}
}

type errExit1 struct{}

func (errExit1) Error() string { return "exit 1" }

func writeRejectHook(origin string) error {
	hook := filepath.Join(origin, "hooks", "pre-receive")
	body := "#!/bin/sh\necho 'pre-receive hook declined' >&2\nexit 1\n"
	return os.WriteFile(hook, []byte(body), 0o755)
}
