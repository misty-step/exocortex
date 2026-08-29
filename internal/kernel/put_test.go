package kernel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
		c, err := Register(r.name, r.path, "git", "", "")
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

	// Second create on an existing committed path returns exists.
	if _, conf := f.put("hosta", "notes/second.md", mkNote("note", "v1")); conf != nil {
		t.Fatalf("seed create failed: %v", conf.Code)
	}
	_, conf = f.put("hosta", "notes/second.md", mkNote("note", "intruder"))
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
	rootA := mustEffectiveRoot(&f.cs[0])
	commits := g(t, rootA, "log", "--format=%H", "--", "notes/race.md")
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

	// Foreign staged path in publisher tree aborts before any write and stays intact.
	rootA := mustEffectiveRoot(&f.cs[0])
	foreign := "notes/foreign.md"
	foreignBody := mkNote("note", "another worker's staged work")
	os.WriteFile(filepath.Join(rootA, foreign), []byte(foreignBody), 0o644)
	g(t, rootA, "add", foreign)
	newRev := f.rev("hosta", "notes/cas.md")
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/cas.md",
		Payload: []byte(mkNote("note", "should not land")), Expects: newRev,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "foreign_staged_state")
	if got, _ := os.ReadFile(filepath.Join(rootA, foreign)); string(got) != foreignBody {
		t.Fatal("foreign staged file bytes changed")
	}
	status := g(t, rootA, "status", "--porcelain", "--", foreign)
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
	rootA := mustEffectiveRoot(&f.cs[0])

	// Unstaged edit to destination in publisher tree.
	localEdit := "---\ntype: note\n---\n\nhuman mid-edit\n"
	os.WriteFile(filepath.Join(rootA, "notes/dirty.md"), []byte(localEdit), 0o644)
	_, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/dirty.md",
		Payload: []byte(mkNote("note", "clobber")), Expects: rev,
		Agent: "t", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "dirty_destination")
	if conf.Detail["state"] != "unstaged" {
		t.Fatalf("state = %v, want unstaged", conf.Detail["state"])
	}
	if got, _ := os.ReadFile(filepath.Join(rootA, "notes/dirty.md")); string(got) != localEdit {
		t.Fatal("unstaged edit did not survive byte-for-byte")
	}

	// Staged-only change to destination in publisher tree.
	g(t, rootA, "add", "notes/dirty.md")
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

// Proof 6: the git driver works independently of the cortex validation
// profile, commits exactly once, pushes, and makes an identical retry a no-op.
func TestProof6GitPublisherIsProfileIndependent(t *testing.T) {
	f := newFixture(t)
	for i := range f.cs {
		f.cs[i].Profile = "strict"
	}
	payload := mkNote("decision", "the pinned contract")
	if _, conf := f.put("hosta", "misty-step/pinned.md", payload); conf != nil {
		t.Fatalf("create failed: %s detail=%v", conf.Code, conf.Detail)
	}
	rootA := mustEffectiveRoot(&f.cs[0])
	logA := g(t, rootA, "log", "--format=%H", "--", "misty-step/pinned.md")
	if len(strings.Fields(logA)) != 1 {
		t.Fatalf("want one commit, got %q", logA)
	}
	remoteHead := g(t, f.origin, "rev-parse", "master")
	if remoteHead != f.head(rootA) {
		t.Fatal("push did not advance remote to local HEAD")
	}
	// Commit touches only the target path.
	stat := g(t, rootA, "show", "--name-only", "--format=", "HEAD")
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
	if g(t, rootA, "log", "--format=%H", "--", "misty-step/pinned.md") != logA {
		t.Fatal("no-op created a commit")
	}

	// Second no-op form: get-content retry — the payload carries the
	// stored provenance stamp back verbatim (pinned byte-equality).
	disk, _ := os.ReadFile(filepath.Join(rootA, "misty-step/pinned.md"))
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

	// created survives an update untouched when resubmitted verbatim.
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
	rootA := mustEffectiveRoot(&f.cs[0])
	disk, _ := os.ReadFile(filepath.Join(rootA, "notes/minimal.md"))
	if !strings.Contains(string(disk), "created: 2019-03-04T05:06:07Z") {
		t.Fatal("created was modified")
	}
	if res.Revision == "" {
		t.Fatal("missing new revision")
	}

	// Changing created aborts created_immutable and writes nothing.
	rev = f.rev("hosta", "notes/minimal.md")
	before, _ := os.ReadFile(filepath.Join(rootA, "notes/minimal.md"))
	tampered := "---\ntype: fleeting\ncreated: 2026-01-01T00:00:00Z\n---\n\nchanged body\n"
	_, conf = Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/minimal.md",
		Payload: []byte(tampered), Expects: rev,
		Agent: "u", Via: "cli", OwnPayload: true,
	})
	if conf == nil || conf.Code != "created_immutable" {
		t.Fatalf("want created_immutable, got %#v", conf)
	}
	if conf.Detail["stored"] != "2019-03-04T05:06:07Z" || conf.Detail["submitted"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("detail = %v", conf.Detail)
	}
	if after, _ := os.ReadFile(filepath.Join(rootA, "notes/minimal.md")); string(after) != string(before) {
		t.Fatal("rejected update touched the file")
	}

	// Dropping created also aborts.
	dropped := "---\ntype: fleeting\n---\n\nno created at all\n"
	_, conf = Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/minimal.md",
		Payload: []byte(dropped), Expects: rev,
		Agent: "u", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "created_immutable")

	// Unquoted RFC3339 is the common form; a changed unquoted value must
	// be caught (yaml.v3 decodes it to time.Time — lexical comparison).
	unq := "---\ntype: fleeting\ncreated: 2019-03-04T05:06:07Z\n---\n\nunquoted stored\n"
	if _, conf := f.put("hosta", "notes/unquoted.md", unq); conf != nil {
		t.Fatal(conf.Code)
	}
	revUQ := f.rev("hosta", "notes/unquoted.md")
	unqChanged := "---\ntype: fleeting\ncreated: 2030-06-06T06:06:06Z\n---\n\nsneaky\n"
	_, conf = Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/unquoted.md",
		Payload: []byte(unqChanged), Expects: revUQ,
		Agent: "u", Via: "cli", OwnPayload: true,
	})
	wantCode(t, conf, "created_immutable")

	// Filling a MISSING created is legal gap-fill.
	noCreated := "---\ntype: fleeting\n---\n\nnever dated\n"
	if _, conf := f.put("hosta", "notes/gapfill.md", noCreated); conf != nil {
		t.Fatal(conf.Code)
	}
	revGF := f.rev("hosta", "notes/gapfill.md")
	gapFilled := "---\ntype: fleeting\ncreated: 2024-02-02T02:02:02Z\n---\n\nnow dated\n"
	resGF, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta", Path: "notes/gapfill.md",
		Payload: []byte(gapFilled), Expects: revGF,
		Agent: "u", Via: "cli", OwnPayload: true,
	})
	if conf != nil || resGF == nil {
		t.Fatalf("gap-fill must succeed: %v", conf)
	}
}

// Proof 12: the create fast-path reads under the lock. While host A's
// put sits paused after its write (lock held), a concurrent create on
// the SAME cortex must block — never answer `exists` off A's transient
// bytes, which the unwind then removes. After A loses its push and
// converges, B's answer reflects the settled state (the winner's note).
func TestProof12CreateFastPathWaitsForLock(t *testing.T) {
	f := newFixture(t)
	aWritten := make(chan struct{})
	advanceRemote := make(chan struct{})
	releaseA := make(chan struct{})
	var once sync.Once

	aPayload := mkNote("note", "A transient create")
	beforePushHook = func() {
		once.Do(func() {
			close(aWritten) // A's file exists transiently, lock held
			<-advanceRemote // test lands the winner on the remote
			<-releaseA      // test lets A resume into push rejection
		})
	}

	doneA := make(chan string, 1)
	go func() {
		_, conf := Put(nil, f.cs, PutInput{
			CortexName: "hosta", Path: "notes/pause.md",
			Payload: []byte(aPayload),
			Agent:   "agent-a", Via: "cli", OwnPayload: true,
		})
		if conf != nil {
			doneA <- conf.Code
			return
		}
		doneA <- "pushed"
	}()
	<-aWritten

	// Same cortex, concurrent create: must NOT resolve while A holds
	// the lock — the pre-fix stat would answer `exists` instantly.
	doneB := make(chan string, 1)
	go func() {
		_, conf := Put(nil, f.cs, PutInput{
			CortexName: "hosta", Path: "notes/pause.md",
			Payload: []byte(mkNote("note", "B must wait")),
			Agent:   "agent-b", Via: "cli", OwnPayload: true,
		})
		if conf != nil {
			doneB <- conf.Code
			return
		}
		doneB <- "pushed"
	}()
	select {
	case code := <-doneB:
		t.Fatalf("B resolved while A held the lock: %s", code)
	case <-time.After(200 * time.Millisecond):
		// blocked on flock, as pinned
	}

	// Winner lands on the remote while A is still paused (plain git on
	// the other clone), then A resumes into a rejected push.
	winnerPayload := mkNote("note", "winner landed mid-race")
	os.MkdirAll(filepath.Join(f.b, "notes"), 0o755)
	os.WriteFile(filepath.Join(f.b, "notes/pause.md"), []byte(winnerPayload), 0o644)
	g(t, f.b, "add", "notes/pause.md")
	g(t, f.b, "commit", "-m", "cortex(test): winner create")
	g(t, f.b, "push")
	close(advanceRemote)
	close(releaseA)

	if code := <-doneA; code != "exists" {
		t.Fatalf("A (loser create) = %s, want exists", code)
	}
	if code := <-doneB; code != "exists" {
		t.Fatalf("B = %s, want exists against the settled winner", code)
	}
	beforePushHook = nil
	rootA := mustEffectiveRoot(&f.cs[0])
	if f.head(rootA) != g(t, f.origin, "rev-parse", "master") {
		t.Fatal("clone A did not converge onto the winner")
	}
	disk, _ := os.ReadFile(filepath.Join(rootA, "notes/pause.md"))
	if !strings.Contains(string(disk), "winner landed mid-race") {
		t.Fatalf("disk = %q, want winner bytes", disk)
	}
}

// Proof 13: concurrent registration from TWO PROCESSES must not lose
// entries — the registry load/check/save transaction is serialized
// under its own lock, and each save uses a unique temp file.
func TestProof13ConcurrentRegistrationTwoProcesses(t *testing.T) {
	testConfigEnv(t)
	if binPath == "" {
		t.Fatal("binary missing")
	}
	dirA, dirB := t.TempDir(), t.TempDir()
	os.MkdirAll(dirA, 0o755)
	os.MkdirAll(dirB, 0o755)

	run := func(name, path string) chan error {
		done := make(chan error, 1)
		cmd := exec.Command(binPath, "register", name, path, "--vcs", "none")
		go func() { done <- cmd.Run() }()
		return done
	}
	a, b := run("proc-a", dirA), run("proc-b", dirB)
	if err := <-a; err != nil {
		t.Fatalf("process A register failed: %v", err)
	}
	if err := <-b; err != nil {
		t.Fatalf("process B register failed: %v", err)
	}
	cs, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range cs {
		names[c.Name] = true
	}
	if !names["proc-a"] || !names["proc-b"] {
		t.Fatalf("lost a concurrent registration; registry holds %v", names)
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
	if conf.Detail["actual"] == conf.Detail["expected"] {
		t.Fatal("revision_conflict must observe a path change; actual==expected is a false publication conflict")
	}

	// Zero trace of B's commit; branch equals remote tip; bytes equal A's.
	rootB := mustEffectiveRoot(&f.cs[1])
	if bBranch, remote := f.head(rootB), g(t, f.origin, "rev-parse", "master"); bBranch != remote {
		t.Fatalf("B branch %s != remote %s", short(bBranch), short(remote))
	}
	disk, _ := os.ReadFile(filepath.Join(rootB, "notes/shared.md"))
	// Winner's committed bytes carry the winner's provenance stamp.
	if !strings.Contains(string(disk), "A wins with this content") || !strings.Contains(string(disk), "agent: agent-a") {
		t.Fatalf("B disk = %q, want A's stamped payload", disk)
	}
	bLog := g(t, rootB, "log", "--format=%s", "--", "notes/shared.md")
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
	g(t, f.b, "commit", "-m", "cortex(test): exocortex put notes/tail.md via agent-b")
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

	rootB := mustEffectiveRoot(&f.cs[1])
	if f.head(rootB) != g(t, f.origin, "rev-parse", "master") {
		t.Fatal("B did not converge onto remote tip")
	}
	disk, err := os.ReadFile(filepath.Join(rootB, "notes/new-hotness.md"))
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
