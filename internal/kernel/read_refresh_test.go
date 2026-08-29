package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
