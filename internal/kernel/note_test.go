package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewULIDShape(t *testing.T) {
	now := time.Now().UTC()
	seen := map[string]bool{}
	for range 1000 { // same millisecond: uniqueness must come from randomness
		id := newULID(now)
		if len(id) != 26 {
			t.Fatalf("ULID length = %d, want 26 (%q)", len(id), id)
		}
		if seen[id] {
			t.Fatalf("duplicate ULID %q within one millisecond", id)
		}
		seen[id] = true
	}
	later := newULID(now.Add(time.Hour))
	for id := range seen {
		if later <= id {
			t.Fatalf("lexical chronology broken: %q >= %q", later, id)
		}
		break
	}
}

// Note writes immutable per-memory files through the put pipeline, and
// memo notes are silent under the daybook profile.
func TestNoteCreatesImmutableJournalFiles(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	c, err := Register("scratch", root, "none", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{*c}

	r1, conf := Note(nil, cs, NoteInput{Text: "first thought", Agent: "agent-a", Via: "cli"})
	if conf != nil {
		t.Fatal(conf.Code)
	}
	r2, conf := Note(nil, cs, NoteInput{Text: "second thought", Agent: "agent-a", Via: "cli"})
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if r1.Path == r2.Path {
		t.Fatal("two notes must land in distinct immutable files")
	}
	if !strings.HasPrefix(r1.Path, "journal/") || !strings.Contains(r1.Path, "-agent-a.md") {
		t.Fatalf("path layout wrong: %s", r1.Path)
	}

	disk, _ := os.ReadFile(filepath.Join(root, r1.Path))
	for _, want := range []string{"type: memo", "first thought", "provenance:", "agent: agent-a"} {
		if !strings.Contains(string(disk), want) {
			t.Fatalf("note file missing %q:\n%s", want, disk)
		}
	}

	lintRes, conf := Lint(cs, "scratch", r1.Path)
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if lintRes.Errors != 0 || lintRes.Warnings != 0 {
		t.Fatalf("memo lint must be silent: %+v", lintRes.Findings)
	}

	if _, conf := Note(nil, cs, NoteInput{Text: "   ", Agent: "x"}); conf == nil || conf.Code != "empty_note" {
		t.Fatal("empty note must conflict")
	}
}

// Proof 14: two journal writers racing across clones BOTH land. A's
// push is genuinely rejected after B's different-path note lands; the
// retry finds A's unique path still free and publishes on top of B's
// commit. No memory is lost to a cross-host race.
func TestProof14ConcurrentJournalPushesBothLand(t *testing.T) {
	f := newFixture(t)
	aWritten := make(chan struct{})
	var once sync.Once
	beforePushHook = func() {
		once.Do(func() {
			close(aWritten)
			// Winner (plain git on cloneB) lands a DIFFERENT journal
			// file while A sits paused post-commit.
			os.MkdirAll(filepath.Join(f.b, "journal", "2026-08-22"), 0o755)
			os.WriteFile(filepath.Join(f.b, "journal", "2026-08-22", "B-note.md"),
				[]byte("---\ntype: memo\ncreated: 2026-08-22T00:00:00Z\n---\n\nB landed first\n"), 0o644)
			g(t, f.b, "add", "journal/2026-08-22/B-note.md")
			g(t, f.b, "commit", "-m", "vault(journal): exocortex note via agent-b")
			g(t, f.b, "push")
		})
	}
	doneA := make(chan string, 1)
	go func() {
		res, conf := Note(nil, f.cs, NoteInput{CortexName: "hosta", Text: "A races B into the journal", Agent: "agent-a", Via: "mcp"})
		if conf != nil {
			doneA <- "conflict:" + conf.Code
			return
		}
		doneA <- res.Path
	}()
	<-aWritten
	path := <-doneA
	beforePushHook = nil

	if !strings.HasPrefix(path, "journal/") {
		t.Fatalf("A's note lost in cross-host race: %q", path)
	}
	rootA := mustEffectiveRoot(&f.cs[0])
	diskA, err := os.ReadFile(filepath.Join(rootA, path))
	if err != nil {
		t.Fatalf("A's own note missing after retry: %v", err)
	}
	if !strings.Contains(string(diskA), "A races B into the journal") {
		t.Fatalf("A note content wrong: %s", diskA)
	}
	bDisk, err := os.ReadFile(filepath.Join(rootA, "journal/2026-08-22/B-note.md"))
	if err != nil || !strings.Contains(string(bDisk), "B landed first") {
		t.Fatalf("B's note missing from converged clone: %v", err)
	}
}

// Every terminal conflict preserves the in-memory payload.
func TestNotePreservesPayloadOnNonRetryable(t *testing.T) {
	f := newFixture(t)
	// Provision writer clone
	if _, conf := f.put("hosta", "notes/provision.md", mkNote("note", "provisioned")); conf != nil {
		t.Fatal(conf.Code)
	}
	writer := mustEffectiveRoot(&f.cs[0])
	foreign := filepath.Join(writer, "notes/foreign-wip.md")
	os.MkdirAll(filepath.Dir(foreign), 0o755)
	os.WriteFile(foreign, []byte("---\ntype: x\n---\nseed\n"), 0o644)
	g(t, writer, "add", "notes/foreign-wip.md")
	g(t, writer, "commit", "-m", "seed foreign tracked file")
	os.WriteFile(foreign, []byte("---\ntype: x\n---\nwip mid-edit\n"), 0o644)

	_, conf := Note(nil, f.cs, NoteInput{CortexName: "hosta", Text: "precious thought", Agent: "a", Via: "cli"})
	if conf == nil || conf.Code != "foreign_unstaged_state" {
		t.Fatalf("want foreign_unstaged_state, got %#v", conf)
	}
	saved, ok := conf.Detail["payload_saved"].(string)
	if !ok || saved == "" {
		t.Fatalf("payload not preserved: %v", conf.Detail)
	}
	raw, rerr := os.ReadFile(saved)
	if rerr != nil || !strings.Contains(string(raw), "precious thought") {
		t.Fatalf("preserved payload wrong: %v", rerr)
	}
}
