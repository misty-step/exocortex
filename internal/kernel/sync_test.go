package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupSyncMockQMD(t *testing.T, logFile string, failStage string) {
	t.Helper()
	binDir := t.TempDir()
	fakeQMD := filepath.Join(binDir, "qmd")

	script := fmt.Sprintf(`#!/bin/sh
cmd="$1"
log="%s"
fail="%s"

echo "$*" >> "$log"

if [ "$cmd" = "$fail" ]; then
  echo "simulated error in $cmd" >&2
  exit 1
fi

if [ "$cmd" = "update" ]; then
  exit 0
fi

if [ "$cmd" = "embed" ]; then
  exit 0
fi

exit 0
`, logFile, failStage)

	if err := os.WriteFile(fakeQMD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(filepath.ListSeparator)+origPath)
}

func TestDirtyMarkerWrittenOnSuccessfulPutAndNote(t *testing.T) {
	f := newFixture(t)
	res, conf := f.put("hosta", "notes/sync-test.md", mkNote("note", "sync marker proof"))
	if conf != nil || !res.Pushed {
		t.Fatalf("put failed: %v / %v", conf, res)
	}

	// Status should report dirty marker present with the commit SHA
	st, sConf := Status(f.cs, "hosta")
	if sConf != nil || len(st) != 1 {
		t.Fatalf("status failed: %v", sConf)
	}
	if !st[0].Dirty {
		t.Fatal("expected cortex to be dirty after put")
	}
	if st[0].DirtyCommit != res.Commit {
		t.Fatalf("dirty commit = %s, want %s", st[0].DirtyCommit, res.Commit)
	}
	if st[0].DirtyCount != 1 {
		t.Fatalf("dirty count = %d, want 1", st[0].DirtyCount)
	}
}

func TestSyncRunsUpdateAndEmbedClearingDirtySnapshot(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")

	res, conf := f.put("hosta", "notes/sync-run.md", mkNote("note", "sync run proof"))
	if conf != nil || !res.Pushed {
		t.Fatalf("put failed: %v", conf)
	}

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf != nil || len(syncRes) != 1 {
		t.Fatalf("sync failed: %v / %v", sConf, syncRes)
	}
	if !syncRes[0].Updated || !syncRes[0].Embedded || !syncRes[0].DirtyCleared {
		t.Fatalf("unexpected sync result: %+v", syncRes[0])
	}
	if syncRes[0].IndexedCommit != res.Commit {
		t.Fatalf("indexed commit = %s, want %s", syncRes[0].IndexedCommit, res.Commit)
	}

	// Verify QMD was invoked with update then embed
	calls, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	callStr := string(calls)
	if !strings.Contains(callStr, "update hosta") {
		t.Fatalf("missing qmd update call: %s", callStr)
	}
	if !strings.Contains(callStr, "embed -c hosta") {
		t.Fatalf("missing qmd embed call: %s", callStr)
	}

	// Status should now report clean (dirty=false) and synced_commit set
	st, _ := Status(f.cs, "hosta")
	if st[0].Dirty {
		t.Fatal("expected cortex to be clean after sync")
	}
	if st[0].SyncedCommit != res.Commit {
		t.Fatalf("synced commit = %s, want %s", st[0].SyncedCommit, res.Commit)
	}
}

func TestSyncPreservesConcurrentDirtyMarkerArrivals(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")

	resA, confA := f.put("hosta", "notes/note-a.md", mkNote("note", "commit A"))
	if confA != nil {
		t.Fatal(confA)
	}

	// Hook fires right after embed, simulating a concurrent write landing commit B
	var resBCommit string
	syncHook = func(cortexName string) {
		time.Sleep(10 * time.Millisecond)
		resBCommit = "commit-b-simulated-concurrent-arrival"
		_ = markDirty(cortexName, resBCommit)
	}
	defer func() { syncHook = nil }()

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf != nil {
		t.Fatalf("sync failed: %v", sConf)
	}
	if syncRes[0].DirtyCleared {
		t.Fatal("dirty_cleared should be false because marker B arrived during sync")
	}

	// Status should report Dirty = true, DirtyCommit = B, and SyncedCommit = A!
	st, _ := Status(f.cs, "hosta")
	if !st[0].Dirty {
		t.Fatal("expected cortex to remain dirty for arrival B")
	}
	if st[0].DirtyCommit != resBCommit {
		t.Fatalf("dirty commit = %s, want %s", st[0].DirtyCommit, resBCommit)
	}
	if st[0].SyncedCommit != resA.Commit {
		t.Fatalf("synced commit = %s, want %s", st[0].SyncedCommit, resA.Commit)
	}
}

func TestSyncFailureLeavesDirtyAndRecordsSyncError(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "embed") // simulate failure on qmd embed

	res, _ := f.put("hosta", "notes/fail-test.md", mkNote("note", "fail proof"))

	_, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "embed_failed" {
		t.Fatalf("want embed_failed, got %#v", sConf)
	}

	// Dirty marker must survive and sync_error must be recorded
	st, _ := Status(f.cs, "hosta")
	if !st[0].Dirty {
		t.Fatal("dirty marker was lost on failure")
	}
	if st[0].DirtyCommit != res.Commit {
		t.Fatalf("dirty commit = %s, want %s", st[0].DirtyCommit, res.Commit)
	}
	if !strings.Contains(st[0].LastSyncError, "simulated error in embed") {
		t.Fatalf("last sync error = %q, want simulated error", st[0].LastSyncError)
	}
}

func TestMalformedDirtyMarkerFailsStateFailedAndRetainsMarkers(t *testing.T) {
	f := newFixture(t)
	dDir, err := dirtyMarkerDir("hosta")
	if err != nil {
		t.Fatal(err)
	}
	brokenFile := filepath.Join(dDir, "9999999999-broken.json")
	if err := os.WriteFile(brokenFile, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "state_failed" {
		t.Fatalf("want state_failed, got %#v", sConf)
	}

	// Broken file must still exist on disk (never silently deleted)
	if _, err := os.Stat(brokenFile); err != nil {
		t.Fatal("malformed marker file was deleted on error")
	}
}

func TestDirtyMarkerWrittenOnNoneVCSPut(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	c, err := Register("nonebox", root, "none", "daybook", "")
	if err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{*c}
	res, conf := Put(context.Background(), cs, PutInput{
		CortexName: "nonebox",
		Path:       "notes/none.md",
		Payload:    []byte(mkNote("note", "none vcs marker")),
		Agent:      "test",
		Via:        "test",
	})
	if conf != nil {
		t.Fatalf("put failed: %v", conf)
	}
	if res.Commit != "" || res.Pushed {
		t.Fatalf("none vcs should not commit: %+v", res)
	}
	if res.Revision == "" {
		t.Fatal("expected content revision")
	}
	st, sConf := Status(cs, "nonebox")
	if sConf != nil || len(st) != 1 {
		t.Fatalf("status failed: %v", sConf)
	}
	if !st[0].Dirty || st[0].DirtyCommit != res.Revision {
		t.Fatalf("status = %+v, want dirty revision %s", st[0], res.Revision)
	}
}

func TestDirtyMarkerWrittenOnRemotelessDaybookPut(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/seed-writer.md", mkNote("note", "provision writer")); conf != nil {
		t.Fatal(conf)
	}
	w := writerDir(&f.cs[0])
	if w == "" {
		t.Fatal("writer clone missing after first put")
	}
	g(t, w, "branch", "--unset-upstream")

	res, conf := f.put("hosta", "notes/local-only.md", mkNote("note", "remoteless commit"))
	if conf != nil {
		t.Fatal(conf)
	}
	if res.Pushed {
		t.Fatal("expected remoteless put to skip push")
	}
	if res.Commit == "" {
		t.Fatal("expected local commit identity")
	}
	st, sConf := Status(f.cs, "hosta")
	if sConf != nil {
		t.Fatal(sConf)
	}
	if !st[0].Dirty || st[0].DirtyCommit != res.Commit {
		t.Fatalf("status = %+v, want dirty commit %s", st[0], res.Commit)
	}
}

func TestNoopPutDoesNotMarkDirty(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")

	payload := mkNote("note", "noop marker")
	res, conf := f.put("hosta", "notes/noop.md", payload)
	if conf != nil {
		t.Fatal(conf)
	}
	if _, sConf := Sync(context.Background(), f.cs, "hosta"); sConf != nil {
		t.Fatal(sConf)
	}
	st, _ := Status(f.cs, "hosta")
	if st[0].Dirty {
		t.Fatal("expected clean after sync")
	}

	again, conf := Put(nil, f.cs, PutInput{
		CortexName: "hosta",
		Path:       "notes/noop.md",
		Payload:    []byte(payload),
		Expects:    res.Revision,
		Agent:      "test",
		Via:        "test",
	})
	if conf != nil || !again.Noop {
		t.Fatalf("want noop, got %+v / %v", again, conf)
	}
	st, _ = Status(f.cs, "hosta")
	if st[0].Dirty {
		t.Fatal("noop put must not create a dirty marker")
	}
}

func TestSyncContinuesAfterSiblingFailure(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")

	if _, conf := f.put("hostb", "notes/ok.md", mkNote("note", "sibling survives")); conf != nil {
		t.Fatal(conf)
	}
	dDir, err := dirtyMarkerDir("hosta")
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dDir, "0001-broken.json")
	if err := os.WriteFile(broken, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	syncRes, sConf := Sync(context.Background(), f.cs, "")
	if sConf == nil || sConf.Code != "state_failed" {
		t.Fatalf("want state_failed, got %#v", sConf)
	}
	var hosta, hostb *SyncResult
	for i := range syncRes {
		switch syncRes[i].Cortex {
		case "hosta":
			hosta = &syncRes[i]
		case "hostb":
			hostb = &syncRes[i]
		}
	}
	if hosta == nil || hosta.Error != "state_failed" {
		t.Fatalf("hosta result = %+v", hosta)
	}
	if hostb == nil || !hostb.Updated || !hostb.Embedded || !hostb.DirtyCleared {
		t.Fatalf("hostb should still sync, got %+v", hostb)
	}
}

func TestStatusDoesNotCreateStateOrWriter(t *testing.T) {
	f := newFixture(t)
	st, conf := Status(f.cs, "hosta")
	if conf != nil || len(st) != 1 {
		t.Fatalf("status failed: %v / %v", conf, st)
	}
	if st[0].Dirty {
		t.Fatal("untouched cortex must not report dirty")
	}
	cfg, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "state", "hosta")); !os.IsNotExist(err) {
		t.Fatalf("status created state dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "writers", "hosta")); !os.IsNotExist(err) {
		t.Fatalf("status created writer clone: %v", err)
	}
}

func TestUnreadableDirtyPathFailsStateFailed(t *testing.T) {
	f := newFixture(t)
	sDir, err := cortexStateDir("hosta")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sDir, "dirty"), []byte("not-a-directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "state_failed" {
		t.Fatalf("want state_failed, got %#v", sConf)
	}
}
