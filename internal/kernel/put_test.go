package kernel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// g runs git in dir and fails the test on error.
func g(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mkNote(nType, body string) string {
	return fmt.Sprintf("---\ntype: %s\nstatus: active\ndescription: test note\ntags: [test]\ncreated: 2026-08-21T00:00:00Z\n---\n\n%s\n", nType, body)
}

func mustPayload(t *testing.T, p []byte) string {
	t.Helper()
	return string(p)
}

// fixture is a two-clone cross-host simulation: a bare remote plus two
// working clones, each registered as its own cortex.
type fixture struct {
	t      *testing.T
	origin string
	a, b   string
	cs     []Cortex
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	testConfigEnv(t)
	base := t.TempDir()
	f := &fixture{
		t:      t,
		origin: filepath.Join(base, "origin.git"),
		a:      filepath.Join(base, "cloneA"),
		b:      filepath.Join(base, "cloneB"),
	}
	g(t, base, "init", "--bare", "-b", "master", f.origin)
	g(t, base, "clone", f.origin, f.a)
	g(t, f.a, "config", "user.email", "a@test")
	g(t, f.a, "config", "user.name", "Host A")
	os.WriteFile(filepath.Join(f.a, "README.md"), []byte("# fixture\n"), 0o644)
	g(t, f.a, "add", "README.md")
	g(t, f.a, "commit", "-m", "seed")
	g(t, f.a, "push", "-u", "origin", "master")
	g(t, base, "clone", f.origin, f.b)
	g(t, f.b, "config", "user.email", "b@test")
	g(t, f.b, "config", "user.name", "Host B")

	for _, r := range []struct {
		name, path string
	}{{"hosta", f.a}, {"hostb", f.b}} {
		c, err := Register(r.name, r.path, "daybook", "")
		if err != nil {
			t.Fatalf("register %s: %v", r.name, err)
		}
		f.cs = append(f.cs, *c)
	}
	return f
}

func (f *fixture) put(cortex, path, payload string) (*PutResult, *Conflict) {
	f.t.Helper()
	return Put(nil, f.cs, PutInput{
		CortexName: cortex,
		Path:       path,
		Payload:    []byte(payload),
		Agent:      "test-agent",
		Via:        "cli",
		OwnPayload: true,
	})
}

func (f *fixture) rev(cortex, path string) string {
	f.t.Helper()
	res, conf := Get(f.cs, cortex, path)
	if conf != nil {
		f.t.Fatalf("get %s/%s: %v", cortex, path, conf.Code)
	}
	return res.Revision
}

func (f *fixture) head(dir string) string { return g(f.t, dir, "rev-parse", "HEAD") }

// Proof 1: bare put on any existing path -> exists.
func TestProof1ExistsOnAnyPreexistingDestination(t *testing.T) {
	f := newFixture(t)

	// Tracked clean.
	if _, conf := f.put("hosta", "notes/tracked.md", mkNote("note", "v1")); conf != nil {
		t.Fatalf("seed create failed: %v", conf.Code)
	}
	_, conf := f.put("hosta", "notes/tracked.md", mkNote("note", "other"))
	wantCode(t, conf, "exists")

	// Staged destination.
	stagedPath := "notes/staged.md"
	os.WriteFile(filepath.Join(f.a, stagedPath), []byte(mkNote("note", "someone staged this")), 0o644)
	g(t, f.a, "add", stagedPath)
	_, conf = f.put("hosta", stagedPath, mkNote("note", "intruder"))
	wantCode(t, conf, "exists")

	// Untracked destination.
	untracked := "notes/untracked.md"
	os.WriteFile(filepath.Join(f.a, untracked), []byte(mkNote("note", "fresh")), 0o644)
	_, conf = f.put("hosta", untracked, mkNote("note", "intruder"))
	wantCode(t, conf, "exists")
}

// Proof 2: same-host create race — exactly one creator wins, loser gets exists.
func TestProof2CreateRace(t *testing.T) {
	f := newFixture(t)
	type outcome struct {
		won  bool
		code string
	}
	var mu sync.Mutex
	results := make([]outcome, 2)
	var wg sync.WaitGroup
	for i, payload := range []string{mkNote("note", "from alpha"), mkNote("note", "from beta")} {
		wg.Add(1)
		go func(i int, payload string) {
			defer wg.Done()
			res, conf := f.put("hosta", "notes/race.md", payload)
			mu.Lock()
			defer mu.Unlock()
			if conf == nil {
				results[i].won = true
				_ = res
			} else {
				results[i].code = conf.Code
			}
		}(i, payload)
	}
	wg.Wait()

	wins := 0
	for i, r := range results {
		switch {
		case r.won:
			wins++
		case r.code == "exists":
		default:
			t.Fatalf("goroutine %d unexpected result %+v", i, r)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one creator must win, got %d (%+v)", wins, results)
	}
	commits := g(t, f.a, "log", "--format=%H", "--", "notes/race.md")
	if len(strings.Fields(commits)) != 1 {
		t.Fatalf("expected exactly one commit for raced note, got %q", commits)
	}
}

// Proof 3: expects mismatch -> revision_conflict carrying actual revision.
func TestProof3ExpectsMismatch(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/x.md", mkNote("note", "v1")); conf != nil {
		t.Fatal(conf.Code)
	}
	realRev := f.rev("hosta", "notes/x.md")
	bogus := strings.Repeat("ab", 32)
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/x.md",
		Payload: []byte(mkNote("note", "v2")), Expects: bogus,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	if res != nil || conf == nil || conf.Code != "revision_conflict" {
		t.Fatalf("want revision_conflict, got %v / %v", res, conf)
	}
	if conf.Detail["actual"] != realRev {
		t.Fatalf("actual = %v, want stored revision %s", conf.Detail["actual"], realRev)
	}
}

// Proof 4: concurrent updates with the same expected revision — exactly
// one commits; plus foreign staged abort.
func TestProof4ConcurrentUpdatesAndForeignStaged(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/cas.md", mkNote("note", "base")); conf != nil {
		t.Fatal(conf.Code)
	}
	r1 := f.rev("hosta", "notes/cas.md")

	type outcome struct {
		pushed bool
		code   string
	}
	results := make([]outcome, 2)
	var wg sync.WaitGroup
	for i, body := range []string{"winner body", "loser body"} {
		wg.Add(1)
		go func(i int, body string) {
			defer wg.Done()
			res, conf := Put(nil, f.cs, PutInput{
				CortexName: "hosta", Path: "notes/cas.md",
				Payload: []byte(mkNote("note", body)), Expects: r1,
				Agent: "t", Via: "cli", OwnPayload: true,
			})
			out := outcome{}
			if conf == nil && res != nil {
				out.pushed = res.Pushed
			} else if conf != nil {
				out.code = conf.Code
			}
			results[i] = out
		}(i, body)
	}
	wg.Wait()

	winners, conflicts := 0, 0
	for _, r := range results {
		switch {
		case r.pushed:
			winners++
		case r.code == "revision_conflict":
			conflicts++
		default:
			t.Fatalf("unexpected outcome %+v", r)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("want 1 winner + 1 revision_conflict, got %+v", results)
	}

	// Foreign staged path aborts before any write and stays intact.
	foreign := "notes/foreign.md"
	foreignBody := mkNote("note", "another worker's staged work")
	os.WriteFile(filepath.Join(f.a, foreign), []byte(foreignBody), 0o644)
	g(t, f.a, "add", foreign)
	newRev := f.rev("hosta", "notes/cas.md")
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/cas.md",
		Payload: []byte(mkNote("note", "should not land")), Expects: newRev,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "foreign_staged_state")
	if got, _ := os.ReadFile(filepath.Join(f.a, foreign)); string(got) != foreignBody {
		t.Fatal("foreign staged file bytes changed")
	}
	status := g(t, f.a, "status", "--porcelain", "--", foreign)
	if !strings.HasPrefix(status, "A ") && !strings.HasPrefix(status, "M ") {
		t.Fatalf("foreign file no longer staged: %q", status)
	}
}

// Proof 5: dirty destination aborts; local edits survive byte-for-byte.
func TestProof5DirtyDestination(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/dirty.md", mkNote("note", "committed")); conf != nil {
		t.Fatal(conf.Code)
	}
	rev := f.rev("hosta", "notes/dirty.md")

	// Unstaged edit to destination.
	localEdit := "---\ntype: note\n---\n\nhuman mid-edit\n"
	os.WriteFile(filepath.Join(f.a, "notes/dirty.md"), []byte(localEdit), 0o644)
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/dirty.md",
		Payload: []byte(mkNote("note", "clobber")), Expects: rev,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "dirty_destination")
	if conf.Detail["state"] != "unstaged" {
		t.Fatalf("state = %v, want unstaged", conf.Detail["state"])
	}
	if got, _ := os.ReadFile(filepath.Join(f.a, "notes/dirty.md")); string(got) != localEdit {
		t.Fatal("unstaged edit did not survive byte-for-byte")
	}

	// Staged-only change to destination.
	g(t, f.a, "add", "notes/dirty.md")
	_, conf = Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/dirty.md",
		Payload: []byte(mkNote("note", "clobber")), Expects: rev,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "dirty_destination")
	if conf.Detail["state"] != "staged" {
		t.Fatalf("state = %v, want staged", conf.Detail["state"])
	}
}

// Proof 6: daybook driver commits exactly once, pushes; identical retry is a no-op.
func TestProof6SingleCommitPushedThenNoop(t *testing.T) {
	f := newFixture(t)
	payload := mkNote("decision", "the pinned contract")
	if _, conf := f.put("hosta", "misty-step/pinned.md", payload); conf != nil {
		t.Fatalf("create failed: %s detail=%v", conf.Code, conf.Detail)
	}
	logA := g(t, f.a, "log", "--format=%H", "--", "misty-step/pinned.md")
	if len(strings.Fields(logA)) != 1 {
		t.Fatalf("want one commit, got %q", logA)
	}
	remoteHead := g(t, f.origin, "rev-parse", "master")
	if remoteHead != f.head(f.a) {
		t.Fatal("push did not advance remote to local HEAD")
	}
	// Commit touches only the target path.
	stat := g(t, f.a, "show", "--name-only", "--format=", "HEAD")
	if lines := nonEmpty(stat); len(lines) != 1 || lines[0] != "misty-step/pinned.md" {
		t.Fatalf("commit touched %v", lines)
	}

	// Byte-identical retry with current revision: successful no-op.
	rev := f.rev("hosta", "misty-step/pinned.md")
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "misty-step/pinned.md",
		Payload: []byte(payload), Expects: rev,
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("retry failed: %s detail=%v", conf.Code, conf.Detail)
	}
	if g(t, f.a, "log", "--format=%H", "--", "misty-step/pinned.md") != logA {
		t.Fatal("no-op created a commit")
	}

	// Second no-op form: get-content retry — the payload carries the
	// stored provenance stamp back verbatim (pinned byte-equality).
	disk, _ := os.ReadFile(filepath.Join(f.a, "misty-step/pinned.md"))
	res2, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "misty-step/pinned.md",
		Payload: disk, Expects: rev,
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("get-content retry failed: %s detail=%v", conf.Code, conf.Detail)
	}
	if !res2.Noop {
		t.Fatal("get-content retry must be a no-op")
	}
}

// Proof 8: profile conformance — type-only frontmatter passes; created immutable.
func TestProof8ProfileConformance(t *testing.T) {
	f := newFixture(t)
	minimal := "---\ntype: fleeting\ncreated: 2019-03-04T05:06:07Z\n---\n\ntiny\n"
	if _, conf := f.put("hosta", "notes/minimal.md", minimal); conf != nil {
		t.Fatalf("type-only note rejected: %v", conf.Code)
	}
	lintRes, conf := Lint(f.cs, "hosta", "notes/minimal.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if lintRes.Errors != 0 {
		t.Fatalf("lint errors on conforming note: %+v", lintRes.Findings)
	}
	if lintRes.Warnings == 0 {
		t.Fatal("expected warnings for missing optional keys")
	}

	// created survives an update untouched.
	rev := f.rev("hosta", "notes/minimal.md")
	updated := "---\ntype: fleeting\ncreated: 2019-03-04T05:06:07Z\n---\n\nchanged body\n"
	res, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/minimal.md",
		Payload: []byte(updated), Expects: rev,
		Agent: "u", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatal(conf.Code)
	}
	disk, _ := os.ReadFile(filepath.Join(f.a, "notes/minimal.md"))
	if !strings.Contains(string(disk), "created: 2019-03-04T05:06:07Z") {
		t.Fatal("created was modified")
	}
	if !strings.Contains(res.Revision, "") || res.Revision == "" {
		t.Fatal("missing new revision")
	}
}

// Proof 9: cross-host update race — loser converges onto the winner.
func TestProof9CrossHostUpdateRace(t *testing.T) {
	f := newFixture(t)
	// Seed note at R1 before clone B snapshots it... B already cloned, so
	// seed through A and pull into B.
	if _, conf := f.put("hosta", "notes/shared.md", mkNote("note", "R1 content")); conf != nil {
		t.Fatal(conf.Code)
	}
	g(t, f.b, "pull", "--ff-only")
	r1 := f.rev("hosta", "notes/shared.md")
	if f.rev("hostb", "notes/shared.md") != r1 {
		t.Fatal("hosts diverged before race")
	}
	// Host B races with a stale view. The hook fires after B's local
	// commit, landing A's winning put on the remote — so B's push is
	// genuinely rejected (non-fast-forward), not caught by B's refresh.
	aPayload := mkNote("note", "A wins with this content")
	bPayload := mkNote("note", "B loses this content")
	beforePushHook = func() {
		if _, conf := Put(nil, f.cs, PutInput{
			CortexName: "hosta", Path: "notes/shared.md",
			Payload: []byte(aPayload), Expects: r1,
			Agent: "agent-a", Via: "cli", OwnPayload: true,
		}); conf != nil {
			t.Errorf("peer put failed inside hook: %v", conf.Code)
		}
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/shared.md",
		Payload: []byte(bPayload), Expects: r1,
		Agent: "agent-b", Via: "cli", OwnPayload: true,
	})
	beforePushHook = nil
	wantCode(t, conf, "revision_conflict")
	if conf.Detail["push_stderr"] == "" {
		t.Fatal("conflict must carry push diagnostics (evidence of real rejection)")
	}

	// Zero trace of B's commit; branch equals remote tip; bytes equal A's.
	if bBranch, remote := f.head(f.b), g(t, f.origin, "rev-parse", "master"); bBranch != remote {
		t.Fatalf("B branch %s != remote %s", short(bBranch), short(remote))
	}
	disk, _ := os.ReadFile(filepath.Join(f.b, "notes/shared.md"))
	// Winner's committed bytes carry the winner's provenance stamp.
	if !strings.Contains(string(disk), "A wins with this content") || !strings.Contains(string(disk), "agent: agent-a") {
		t.Fatalf("B disk = %q, want A's stamped payload", disk)
	}
	bLog := g(t, f.b, "log", "--format=%s", "--", "notes/shared.md")
	if strings.Contains(bLog, "exocortex put notes/shared.md via agent-b") {
		t.Fatalf("B's losing commit survived: %q", bLog)
	}
	if merges := g(t, f.b, "log", "--merges", "--format=%H"); merges != "" {
		t.Fatalf("merge commit appeared: %q", merges)
	}
}

// Proof 10: unwind safety with a foreign unstaged edit constructed after
// pre-flight would have run.
func TestProof10UnwindSparesForeignUnstaged(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/tail.md", mkNote("note", "R1")); conf != nil {
		t.Fatal(conf.Code)
	}
	g(t, f.b, "pull", "--ff-only")

	// Remote advances (winner lands).
	r1 := f.rev("hostb", "notes/tail.md")
	aPayload := mkNote("note", "remote winner")
	if _, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/tail.md",
		Payload: []byte(aPayload), Expects: r1,
		Agent: "a", Via: "cli", OwnPayload: true,
	}); conf != nil {
		t.Fatal(conf.Code)
	}

	// In B, simulate the rejected put's local state directly: our commit
	// on top of base, unrelated unstaged edit present (appeared after
	// pre-flight).
	base := f.head(f.b)
	os.WriteFile(filepath.Join(f.b, "notes/tail.md"), []byte(mkNote("note", "B loses")), 0o644)
	g(t, f.b, "add", "notes/tail.md")
	g(t, f.b, "commit", "-m", "vault(test): exocortex put notes/tail.md via agent-b")
	foreignRel := "notes/foreign-wip.md"
	foreignAbs := filepath.Join(f.b, foreignRel)
	foreignBody := "---\ntype: scratch\n---\n\nunrelated WIP that must survive\n"
	os.WriteFile(foreignAbs, []byte("---\ntype: old\n---\n\nold\n"), 0o644)
	g(t, f.b, "add", foreignRel)
	g(t, f.b, "commit", "-m", "seed foreign tracked file")
	os.WriteFile(foreignAbs, []byte(foreignBody), 0o644) // now unstaged-dirty
	// A foreign path staged AFTER pre-flight (the race window): the
	// soft-reset unwind must leave its index entry and bytes intact.
	stagedRel := "notes/staged-in-race.md"
	stagedBody := "---\ntype: note\n---\n\nstaged mid-race, must stay staged\n"
	os.WriteFile(filepath.Join(f.b, stagedRel), []byte(stagedBody), 0o644)
	g(t, f.b, "add", stagedRel)

	errs := unwindAndConverge(f.b, base, "notes/tail.md")
	if len(errs) != 0 {
		t.Fatalf("unwind reported errors: %v", errs)
	}
	if got, _ := os.ReadFile(foreignAbs); string(got) != foreignBody {
		t.Fatal("foreign unstaged edit destroyed by unwind")
	}
	if status := g(t, f.b, "status", "--porcelain", "--", stagedRel); !strings.HasPrefix(status, "A ") && !strings.HasPrefix(status, "M ") {
		t.Fatalf("mid-race staged file lost its index entry: %q", status)
	}
	if got, _ := os.ReadFile(filepath.Join(f.b, stagedRel)); string(got) != stagedBody {
		t.Fatal("mid-race staged file bytes changed")
	}
	if f.head(f.b) != g(t, f.origin, "rev-parse", "master") {
		t.Fatal("unwind did not converge onto remote tip")
	}
	disk, _ := os.ReadFile(filepath.Join(f.b, "notes/tail.md"))
	// Winner's committed bytes carry the winner's provenance stamp.
	if !strings.Contains(string(disk), "remote winner") || !strings.Contains(string(disk), "agent: a") {
		t.Fatalf("tail file = %q, want winner's stamped payload", disk)
	}

	// Pre-flight alone: an unrelated unstaged tracked edit present up
	// front aborts cleanly. Run on a pristine slate — the race leftovers
	// above are staged, which is a different (also correct) conflict.
	g(t, f.b, "reset", "--hard", "HEAD")
	scratchRel := "notes/scratch.md"
	scratchAbs := filepath.Join(f.b, scratchRel)
	os.WriteFile(scratchAbs, []byte("---\ntype: x\n---\nseed\n"), 0o644)
	g(t, f.b, "add", scratchRel)
	g(t, f.b, "commit", "-m", "vault(test): seed scratch")
	os.WriteFile(scratchAbs, []byte("---\ntype: x\n---\ndirty\n"), 0o644) // unstaged

	r2 := f.rev("hostb", "notes/tail.md")
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/tail.md",
		Payload: []byte(mkNote("note", "blocked")), Expects: r2,
		Agent: "b", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "foreign_unstaged_state")
	if paths, ok := conf.Detail["paths"].([]string); ok && len(paths) == 1 && paths[0] == scratchRel {
		// expected
	} else {
		t.Fatalf("foreign_unstaged_state paths = %#v", conf.Detail["paths"])
	}
}

// Proof 11: cross-host CREATE race — losing creator exits exists and
// converges onto the winner's note.
func TestProof11CrossHostCreateRace(t *testing.T) {
	f := newFixture(t)
	// B creates the path blind. The hook fires after B's local commit,
	// landing A's create on the remote — B's push is then genuinely
	// rejected, and the tail must remove B's created file (absent from
	// base) before the ff-only restore.
	aPayload := mkNote("note", "A created first")
	bPayload := mkNote("note", "B created blind")
	beforePushHook = func() {
		if _, conf := f.put("hosta", "notes/new-hotness.md", aPayload); conf != nil {
			t.Errorf("peer put failed inside hook: %v", conf.Code)
		}
	}
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hostb", Path: "notes/new-hotness.md",
		Payload: []byte(bPayload),
		Agent:   "agent-b", Via: "mcp", OwnPayload: false, // stdin/MCP-style payload
	})
	beforePushHook = nil
	wantCode(t, conf, "exists")
	if conf.Detail["push_stderr"] == "" {
		t.Fatal("conflict must carry push diagnostics (evidence of real rejection)")
	}

	if f.head(f.b) != g(t, f.origin, "rev-parse", "master") {
		t.Fatal("B did not converge onto remote tip")
	}
	disk, err := os.ReadFile(filepath.Join(f.b, "notes/new-hotness.md"))
	if err != nil {
		t.Fatalf("winner's note missing after restore-to-remote: %v", err)
	}
	if !strings.Contains(string(disk), "A created first") || !strings.Contains(string(disk), "agent: test-agent") {
		t.Fatalf("disk = %q, want A's stamped payload", disk)
	}
	saved, ok := conf.Detail["payload_saved"].(string)
	if !ok || saved == "" {
		t.Fatalf("MCP payload not preserved: %v", conf.Detail)
	}
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("payload_saved missing: %v", err)
	}
}

// ---- helpers ----

func wantCode(t *testing.T, conf *Conflict, code string) {
	t.Helper()
	if conf == nil || conf.Code != code {
		t.Fatalf("want conflict %s, got %#v", code, conf)
	}
	if conf.Hint == "" {
		t.Fatalf("conflict %s must carry a hint", code)
	}
}

func nonEmpty(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
