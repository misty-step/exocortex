package kernel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/exocortex/internal/qmd"
)

func TestHumanPushVisibleToGetAndLint(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/x.md", mkNote("Concept", "from-agent")); conf != nil {
		t.Fatal(conf.Code)
	}

	g(t, f.b, "pull")
	human := mkNote("Concept", "from-human")
	if err := os.WriteFile(filepath.Join(f.b, "notes/x.md"), []byte(human), 0o644); err != nil {
		t.Fatal(err)
	}
	g(t, f.b, "add", "notes/x.md")
	g(t, f.b, "commit", "-m", "human correction")
	g(t, f.b, "push")

	if err := os.MkdirAll(filepath.Join(f.a, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.a, "notes/x.md"), []byte("UNCOMMITTED DIRT"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, conf := Get(f.cs, "hosta", "notes/x.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if !strings.Contains(got.Content, "from-human") {
		t.Fatalf("Get missed origin: %s", got.Content)
	}
	if strings.Contains(got.Content, "UNCOMMITTED DIRT") {
		t.Fatal("Get saw human working-tree dirt")
	}

	lint, conf := Lint(f.cs, "hosta", "notes/x.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if lint.Errors != 0 {
		t.Fatalf("lint errors on valid human note: %+v", lint.Findings)
	}

	if err := os.WriteFile(filepath.Join(f.b, "notes/x.md"), []byte("not a note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g(t, f.b, "add", "notes/x.md")
	g(t, f.b, "commit", "-m", "human break frontmatter")
	g(t, f.b, "push")
	lint, conf = Lint(f.cs, "hosta", "notes/x.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if lint.Errors == 0 {
		t.Fatal("lint must see human-pushed invalid note")
	}
}

func TestLintRejectsDirtyPublisherTree(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/x.md", mkNote("Concept", "clean")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := writerDir(&f.cs[0])
	dirty := filepath.Join(writer, "notes", "uncommitted.md")
	if err := os.WriteFile(dirty, []byte(mkNote("Concept", "dirty")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, conf := Lint(f.cs, "hosta", "")
	if conf == nil || conf.Code != "cortex_unavailable" {
		t.Fatalf("dirty publisher lint conflict=%#v", conf)
	}
	if _, err := os.Stat(dirty); err != nil {
		t.Fatalf("dirty publisher bytes were changed: %v", err)
	}
}

func TestGetHitsRefreshesEachCortexOnce(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"notes/a.md", "notes/b.md"} {
		if _, conf := f.put("hosta", path, mkNote("Concept", path)); conf != nil {
			t.Fatal(conf.Code)
		}
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "fetch.log")
	wrapper := "#!/bin/sh\nif [ \"$1\" = fetch ]; then echo fetch >> \"$EXOCORTEX_FETCH_LOG\"; fi\nexec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXOCORTEX_FETCH_LOG", logPath)
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	results := GetHits(f.cs, []qmd.Hit{
		{File: "qmd://hosta/notes/a.md"},
		{File: "qmd://hosta/notes/b.md"},
	})
	if len(results) != 2 || results[0] == nil || results[1] == nil {
		t.Fatalf("batched results=%#v", results)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "fetch\n"); got != 1 {
		t.Fatalf("fetch count=%d want 1; log=%q", got, raw)
	}
}

func TestGetWaitsForPutLockThenSeesCommit(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/x.md", mkNote("Concept", "one")); conf != nil {
		t.Fatal(conf.Code)
	}
	rev := f.rev("hosta", "notes/x.md")

	started := make(chan struct{})
	release := make(chan struct{})
	beforePushHook = func() {
		beforePushHook = nil
		close(started)
		<-release
	}
	defer func() { beforePushHook = nil }()

	var putConf *Conflict
	donePut := make(chan struct{})
	go func() {
		defer close(donePut)
		_, putConf = Put(nil, f.cs, PutInput{
			CortexName: "hosta",
			Path:       "notes/x.md",
			Payload:    []byte(mkNote("Concept", "two")),
			Expects:    rev,
			Agent:      "test-agent",
			Via:        "cli",
			OwnPayload: true,
		})
	}()
	<-started

	gotCh := make(chan string, 1)
	errCh := make(chan string, 1)
	go func() {
		got, conf := Get(f.cs, "hosta", "notes/x.md")
		if conf != nil {
			errCh <- conf.Code
			return
		}
		gotCh <- got.Content
	}()

	select {
	case s := <-gotCh:
		t.Fatalf("Get returned while Put held the lock: %s", s)
	case s := <-errCh:
		t.Fatalf("Get failed while Put held the lock: %s", s)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	<-donePut
	if putConf != nil {
		t.Fatal(putConf.Code)
	}
	select {
	case s := <-errCh:
		t.Fatal(s)
	case content := <-gotCh:
		if !strings.Contains(content, "two") {
			t.Fatalf("Get after Put: %s", content)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get did not return after Put released the lock")
	}
}
