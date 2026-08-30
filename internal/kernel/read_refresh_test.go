package kernel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadOperationsRejectDirtyPublisher(t *testing.T) {
	cases := []struct {
		name, path, contents string
		staged               bool
	}{
		{"tracked", "README.md", "DIRTY_TRACKED", false},
		{"staged", "notes/staged-wip.md", "DIRTY_STAGED", true},
		{"untracked", "notes/untracked-wip.md", "DIRTY_UNTRACKED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if _, conf := f.put("hosta", "notes/clean.md", mkNote("note", "clean committed bytes")); conf != nil {
				t.Fatal(conf.Code)
			}
			writer := mustEffectiveRoot(&f.cs[0])
			if err := os.WriteFile(filepath.Join(writer, tc.path), []byte(tc.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if tc.staged {
				g(t, writer, "add", tc.path)
			}
			for _, op := range []string{"get", "log", "lint"} {
				var returned bool
				var conf *Conflict
				switch op {
				case "get":
					result, c := Get(f.cs, "hosta", "notes/clean.md")
					returned, conf = result != nil, c
				case "log":
					entries, c := Log(f.cs, "hosta", "notes/clean.md", 10)
					returned, conf = len(entries) > 0, c
				default:
					result, c := Lint(f.cs, "hosta", "notes/clean.md")
					returned, conf = result != nil, c
				}
				if conf == nil || conf.Code != "cortex_unavailable" {
					t.Fatalf("%s conflict=%#v, want cortex_unavailable", op, conf)
				}
				if returned {
					t.Fatalf("%s returned bytes from a dirty publisher", op)
				}
			}
		})
	}
}

func TestNextReadSnapshotSeesPushedOriginCommit(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/x.md", mkNote("note", "from-agent")); conf != nil {
		t.Fatal(conf.Code)
	}
	if _, conf := Get(f.cs, "hosta", "notes/x.md"); conf != nil {
		t.Fatalf("initial get: %s", conf.Code)
	}
	g(t, f.b, "pull")

	for i, tc := range []struct {
		payload, subject, content string
		errors                    int
	}{
		{mkNote("note", "from-human"), "human correction", "from-human", 0},
		{"not a note\n", "human break frontmatter", "", 1},
	} {
		if err := os.WriteFile(filepath.Join(f.b, "notes/x.md"), []byte(tc.payload), 0o644); err != nil {
			t.Fatal(err)
		}
		g(t, f.b, "add", "notes/x.md")
		g(t, f.b, "commit", "-m", tc.subject)
		g(t, f.b, "push")

		if i == 0 {
			got, conf := Get(f.cs, "hosta", "notes/x.md")
			if conf != nil || got.Revision != Revision([]byte(tc.payload)) || !strings.Contains(got.Content, tc.content) {
				t.Fatalf("get after origin push=%+v, conflict=%#v", got, conf)
			}
			entries, conf := Log(f.cs, "hosta", "notes/x.md", 10)
			if conf != nil || len(entries) == 0 || entries[0].Subject != tc.subject {
				t.Fatalf("log after origin push=%+v, conflict=%#v", entries, conf)
			}
		}
		lint, conf := Lint(f.cs, "hosta", "notes/x.md")
		if conf != nil || lint == nil || lint.Errors != tc.errors {
			t.Fatalf("lint[%d]=%+v, conflict=%#v", i, lint, conf)
		}
	}
}

func TestGetManyDoesNotMixRevisionsAfterOriginAdvance(t *testing.T) {
	f := newFixture(t)
	notes := []struct {
		path, body string
	}{
		{"notes/a.md", "a before advance"},
		{"notes/b.md", "b before advance"},
	}
	storedRevisions := make([]string, len(notes))
	for i, note := range notes {
		result, conf := f.put("hosta", note.path, mkNote("note", note.body))
		if conf != nil {
			t.Fatal(conf.Code)
		}
		if result == nil {
			t.Fatal("initial put returned no result")
		}
		storedRevisions[i] = result.Revision
	}
	g(t, f.b, "pull")
	for _, note := range notes {
		if err := os.WriteFile(filepath.Join(f.b, note.path), []byte(mkNote("note", note.body+" after advance")), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writer := mustEffectiveRoot(&f.cs[0])
	originBefore := g(t, f.origin, "rev-parse", "master")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "origin-advanced")
	wrapper := `#!/bin/sh
set -e
if [ "$1" = "show" ] && [ ! -e "$EXOCORTEX_ADVANCE_MARK" ]; then
  touch "$EXOCORTEX_ADVANCE_MARK"
  "$EXOCORTEX_REAL_GIT" -C "$EXOCORTEX_PEER" add -- notes/a.md notes/b.md >/dev/null
  "$EXOCORTEX_REAL_GIT" -C "$EXOCORTEX_PEER" commit -m "origin advance" >/dev/null
  "$EXOCORTEX_REAL_GIT" -C "$EXOCORTEX_PEER" push >/dev/null
  "$EXOCORTEX_REAL_GIT" -C "$EXOCORTEX_WRITER" fetch origin >/dev/null
  "$EXOCORTEX_REAL_GIT" -C "$EXOCORTEX_WRITER" merge --ff-only "@{u}" >/dev/null
fi
exec "$EXOCORTEX_REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXOCORTEX_ADVANCE_MARK", marker)
	t.Setenv("EXOCORTEX_REAL_GIT", realGit)
	t.Setenv("EXOCORTEX_PEER", f.b)
	t.Setenv("EXOCORTEX_WRITER", writer)
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	outcomes := GetMany(f.cs, []GetRequest{
		{CortexName: "hosta", Path: "notes/a.md"},
		{CortexName: "hosta", Path: "notes/b.md"},
	})
	if len(outcomes) != len(notes) {
		t.Fatalf("outcomes=%d, want %d", len(outcomes), len(notes))
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("origin did not advance after snapshot pin: %v", err)
	}
	originAfter := g(t, f.origin, "rev-parse", "master")
	if originAfter == originBefore {
		t.Fatal("origin did not advance during the pinned read")
	}
	publisherAfter := g(t, writer, "rev-parse", "HEAD")
	if publisherAfter != originAfter {
		t.Fatalf("publisher HEAD=%s, origin=%s; wrapper did not fast-forward publisher", publisherAfter, originAfter)
	}
	for i, outcome := range outcomes {
		if outcome.Conflict != nil || outcome.Result == nil {
			t.Fatalf("outcome[%d]=%+v", i, outcome)
		}
		note := notes[i]
		if outcome.Result.Revision != storedRevisions[i] ||
			!strings.Contains(outcome.Result.Content, note.body) {
			t.Fatalf("outcome[%d]=%+v, want %q", i, outcome.Result, note.body)
		}
		if strings.Contains(outcome.Result.Content, "after advance") {
			t.Fatalf("outcome[%d] mixed a post-pin revision: %q", i, outcome.Result.Content)
		}
	}
}

func TestLintWholeCortexHandlesNonASCIICommittedPath(t *testing.T) {
	f := newFixture(t)
	if _, conf := Get(f.cs, "hosta", "README.md"); conf != nil {
		t.Fatalf("warm publisher: %s", conf.Code)
	}

	const rel = "notes/café.md"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(f.b, rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.b, rel), []byte("not a note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g(t, f.b, "add", rel)
	g(t, f.b, "commit", "-m", "add non-ASCII markdown path")
	g(t, f.b, "push")

	result, conf := Lint(f.cs, "hosta", "")
	if conf != nil {
		t.Fatalf("whole-cortex lint: %s", conf.Code)
	}
	if result == nil {
		t.Fatal("whole-cortex lint returned no result")
	}
	missingFrontmatter := 0
	for _, finding := range result.Findings {
		if finding.Rule == "fm_missing" {
			missingFrontmatter++
		}
	}
	if missingFrontmatter != 2 {
		t.Fatalf("whole-cortex lint findings=%+v, want missing frontmatter for README.md and %s", result.Findings, rel)
	}
}

func TestGetWaitsForActivePutLockThenSeesCommit(t *testing.T) {
	f := newFixture(t)
	one := mkNote("note", "one")
	if _, conf := f.put("hosta", "notes/x.md", one); conf != nil {
		t.Fatal(conf.Code)
	}
	revision := f.rev("hosta", "notes/x.md")
	two := mkNote("note", "two")

	started, release := make(chan struct{}), make(chan struct{})
	beforePushHook = func() {
		beforePushHook = nil
		close(started)
		<-release
	}
	t.Cleanup(func() { beforePushHook = nil })

	putDone := make(chan *Conflict, 1)
	go func() {
		_, conf := Put(nil, f.cs, PutInput{
			CortexName: "hosta", Path: "notes/x.md", Payload: []byte(two),
			Expects: revision, Agent: "test-agent", Via: "cli", OwnPayload: true,
		})
		putDone <- conf
	}()
	<-started

	type readDone struct {
		result *GetResult
		conf   *Conflict
	}
	getStarted, getDone := make(chan struct{}), make(chan readDone, 1)
	go func() {
		close(getStarted)
		result, conf := Get(f.cs, "hosta", "notes/x.md")
		getDone <- readDone{result, conf}
	}()
	<-getStarted
	select {
	case got := <-getDone:
		t.Fatalf("get returned while put held lock: %+v", got)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	if conf := <-putDone; conf != nil {
		t.Fatalf("put: %s", conf.Code)
	}
	wantRevision := f.rev("hosta", "notes/x.md")
	select {
	case got := <-getDone:
		if got.conf != nil {
			t.Fatalf("get after put: %s", got.conf.Code)
		}
		if got.result == nil || got.result.Revision != wantRevision || !strings.Contains(got.result.Content, "two") {
			t.Fatalf("get after put=%+v", got.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("get did not return after put released the lock")
	}
}
