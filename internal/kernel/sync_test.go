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
while [ "$1" = "--index" ]; do shift 2; done
cmd="$1"
log="%s"
fail="%s"

echo "$*" >> "$log"

if [ "$cmd" = "$fail" ]; then
  echo "simulated error in $cmd" >&2
  exit 1
fi

if [ "$cmd" = "collection" ]; then
  if [ -z "$EXOCORTEX_TEST_QMD_ROOT" ]; then
    echo "Collection not found: $3" >&2
    exit 1
  fi
  printf 'Collection: %%s\n  Path:     %%s\n' "$3" "$EXOCORTEX_TEST_QMD_ROOT"
  exit 0
fi

if [ "$cmd" = "update" ] || [ "$cmd" = "embed" ]; then
  lock="$EXOCORTEX_TEST_LOCK"
  if [ -n "$XDG_CONFIG_HOME" ] && [ -e "$lock" ]; then
    if ! command -v flock >/dev/null 2>&1; then
      echo "flock_missing $cmd" >> "$log"
    elif flock -n "$lock" true 2>/dev/null; then
      echo "lock_free $cmd" >> "$log"
    else
      echo "lock_held $cmd" >> "$log"
    fi
  fi
  if [ "$cmd" = "embed" ]; then
    echo "Done!"
  fi
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

func alignMockCollection(t *testing.T, c Cortex) {
	t.Helper()
	root, err := effectiveRoot(&c)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXOCORTEX_TEST_QMD_ROOT", root)
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
	alignMockCollection(t, f.cs[0])

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
	if !strings.Contains(callStr, "collection show hosta") {
		t.Fatalf("missing qmd collection show call: %s", callStr)
	}
	if !strings.Contains(callStr, "update hosta") {
		t.Fatalf("missing qmd update call: %s", callStr)
	}
	if !strings.Contains(callStr, "embed -c hosta") {
		t.Fatalf("missing qmd embed call: %s", callStr)
	}
	showAt := strings.Index(callStr, "collection show hosta")
	updateAt := strings.Index(callStr, "update hosta")
	if showAt < 0 || updateAt < 0 || showAt > updateAt {
		t.Fatalf("collection show must precede update: %s", callStr)
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
		_ = markDirty(f.cs[0].Identity(), cortexName, resBCommit)
	}
	defer func() { syncHook = nil }()
	alignMockCollection(t, f.cs[0])

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
	alignMockCollection(t, f.cs[0])

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "embed_failed" {
		t.Fatalf("want embed_failed, got %#v", sConf)
	}
	if len(syncRes) != 1 || !syncRes[0].Updated || syncRes[0].Embedded || syncRes[0].IndexedCommit != res.Commit {
		t.Fatalf("embed failure must report update-only partial result: %+v", syncRes)
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

func TestSyncUpdateFailureOmitsIndexedCommit(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "update")
	if _, conf := f.put("hosta", "notes/upd-fail.md", mkNote("note", "update fail")); conf != nil {
		t.Fatal(conf)
	}
	alignMockCollection(t, f.cs[0])

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "sync_failed" {
		t.Fatalf("want sync_failed, got %#v", sConf)
	}
	if len(syncRes) != 1 || syncRes[0].Updated || syncRes[0].IndexedCommit != "" {
		t.Fatalf("update failure must not claim indexed: %+v", syncRes)
	}
}

func TestMalformedDirtyMarkerFailsStateFailedAndRetainsMarkers(t *testing.T) {
	f := newFixture(t)
	dDir, err := dirtyMarkerDir(f.cs[0].Identity())
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
	st, _ := Status(f.cs, "hosta")
	if !strings.Contains(st[0].LastSyncError, "malformed") {
		t.Fatalf("last sync error = %q, want malformed", st[0].LastSyncError)
	}
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
	alignMockCollection(t, f.cs[0])
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
	dDir, err := dirtyMarkerDir(f.cs[0].Identity())
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dDir, "0001-broken.json")
	if err := os.WriteFile(broken, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	alignMockCollection(t, f.cs[1])
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
	if st[0].HeadCommit != "" {
		t.Fatalf("status must not report human checkout HEAD: %q", st[0].HeadCommit)
	}
	cfg, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "state", f.cs[0].Identity())); !os.IsNotExist(err) {
		t.Fatalf("status created state dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "writers", f.cs[0].Identity())); !os.IsNotExist(err) {
		t.Fatalf("status created writer clone: %v", err)
	}
}

func TestUnreadableDirtyPathFailsStateFailed(t *testing.T) {
	f := newFixture(t)
	sDir, err := cortexStateDir(f.cs[0].Identity())
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
	_, stConf := Status(f.cs, "hosta")
	if stConf == nil || stConf.Code != "state_failed" {
		t.Fatalf("status want state_failed, got %#v", stConf)
	}
}

func TestSyncHoldsWriteLock(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	t.Setenv("EXOCORTEX_TEST_LOCK", filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "exocortex", "locks", f.cs[0].Identity()+".lock"))
	if _, conf := f.put("hosta", "notes/lock-a.md", mkNote("note", "lock a")); conf != nil {
		t.Fatal(conf)
	}
	alignMockCollection(t, f.cs[0])

	putDone := make(chan string, 1)
	syncHook = func(string) {
		go func() {
			_, conf := Put(context.Background(), f.cs, PutInput{
				CortexName: "hosta",
				Path:       "notes/lock-b.md",
				Payload:    []byte(mkNote("note", "blocked by sync")),
				Agent:      "test",
				Via:        "test",
			})
			if conf != nil {
				putDone <- conf.Code
				return
			}
			putDone <- "ok"
		}()
		select {
		case code := <-putDone:
			t.Errorf("Put completed while Sync held the write lock: %s", code)
		case <-time.After(200 * time.Millisecond):
		}
	}
	defer func() { syncHook = nil }()

	if _, conf := Sync(context.Background(), f.cs, "hosta"); conf != nil {
		t.Fatal(conf)
	}
	calls, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "flock_missing") {
		t.Fatal("flock is required to prove the write lock is held during QMD")
	}
	if !strings.Contains(string(calls), "lock_held update") || !strings.Contains(string(calls), "lock_held embed") {
		t.Fatalf("QMD stages must observe the write lock: %s", calls)
	}
	select {
	case code := <-putDone:
		if code != "ok" {
			t.Fatalf("blocked Put failed: %s", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Put did not complete after Sync released the lock")
	}
}

func TestSyncFailsWhenCollectionPathMismatches(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	res, conf := f.put("hosta", "notes/mismatch.md", mkNote("note", "mismatch"))
	if conf != nil {
		t.Fatal(conf)
	}
	t.Setenv("EXOCORTEX_TEST_QMD_ROOT", f.cs[0].Path)

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "index_root_mismatch" {
		t.Fatalf("want index_root_mismatch, got %#v", sConf)
	}
	if len(syncRes) != 1 || syncRes[0].Updated || syncRes[0].IndexedCommit != "" {
		t.Fatalf("pre-update conflict must not claim indexed: %+v", syncRes)
	}
	if sConf.Detail["actual"] != f.cs[0].Path {
		t.Fatalf("actual = %v, want registered path", sConf.Detail["actual"])
	}
	st, _ := Status(f.cs, "hosta")
	if !st[0].Dirty || st[0].DirtyCommit != res.Commit {
		t.Fatalf("markers must be retained: %+v", st[0])
	}
}

func TestSyncFailsWhenCollectionUnverified(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	res, conf := f.put("hosta", "notes/unverified.md", mkNote("note", "unverified"))
	if conf != nil {
		t.Fatal(conf)
	}

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "index_root_unverified" {
		t.Fatalf("want index_root_unverified, got %#v", sConf)
	}
	if len(syncRes) != 1 || syncRes[0].Updated || syncRes[0].IndexedCommit != "" {
		t.Fatalf("pre-update conflict must not claim indexed: %+v", syncRes)
	}
	st, _ := Status(f.cs, "hosta")
	if !st[0].Dirty || st[0].DirtyCommit != res.Commit {
		t.Fatalf("markers must be retained: %+v", st[0])
	}
}

func TestSyncRejectsHumanCheckoutAfterWriterLoss(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	if _, conf := f.put("hosta", "notes/writer-loss.md", mkNote("note", "writer loss")); conf != nil {
		t.Fatal(conf)
	}
	w := writerDir(&f.cs[0])
	if w == "" {
		t.Fatal("writer missing after put")
	}
	if err := os.RemoveAll(w); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXOCORTEX_TEST_QMD_ROOT", f.cs[0].Path)

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "writer_unavailable" {
		t.Fatalf("want writer_unavailable, got %#v", sConf)
	}
	if len(syncRes) != 1 || syncRes[0].Updated || syncRes[0].IndexedCommit != "" {
		t.Fatalf("pre-update conflict must not claim indexed: %+v", syncRes)
	}
}

func TestSyncCleanupFailureRecordsError(t *testing.T) {
	f := newFixture(t)
	logFile := filepath.Join(t.TempDir(), "qmd-calls.log")
	setupSyncMockQMD(t, logFile, "")
	res, conf := f.put("hosta", "notes/cleanup.md", mkNote("note", "cleanup fail"))
	if conf != nil {
		t.Fatal(conf)
	}
	alignMockCollection(t, f.cs[0])
	dDir, err := dirtyMarkerPath(f.cs[0].Identity())
	if err != nil {
		t.Fatal(err)
	}
	syncHook = func(string) {
		if err := os.Chmod(dDir, 0o555); err != nil {
			t.Errorf("chmod dirty dir: %v", err)
		}
	}
	defer func() {
		syncHook = nil
		_ = os.Chmod(dDir, 0o755)
	}()

	syncRes, sConf := Sync(context.Background(), f.cs, "hosta")
	if sConf == nil || sConf.Code != "state_failed" {
		t.Fatalf("want state_failed on cleanup, got %#v / %+v", sConf, syncRes)
	}
	if len(syncRes) != 1 || !syncRes[0].Updated || !syncRes[0].Embedded || syncRes[0].DirtyCleared || syncRes[0].IndexedCommit != res.Commit {
		t.Fatalf("partial result = %+v, want index advanced for %s", syncRes, res.Commit)
	}
	st, _ := Status(f.cs, "hosta")
	if !st[0].Dirty || st[0].DirtyCommit != res.Commit {
		t.Fatalf("markers must remain: %+v", st[0])
	}
	if st[0].SyncedCommit != res.Commit {
		t.Fatalf("synced commit = %s, want %s", st[0].SyncedCommit, res.Commit)
	}
	if !strings.Contains(st[0].LastSyncError, "permission denied") {
		t.Fatalf("last sync error = %q", st[0].LastSyncError)
	}
}

func TestDirtyMarkerFailedWarningOnUnwritableState(t *testing.T) {
	f := newFixture(t)
	cfg, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	stateParent := filepath.Join(cfg, "state")
	if err := os.MkdirAll(stateParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stateParent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(stateParent, 0o755)

	res, conf := f.put("hosta", "notes/nowrite.md", mkNote("note", "marker fail"))
	if conf != nil {
		t.Fatalf("put must succeed: %v", conf)
	}
	found := false
	for _, w := range res.Warnings {
		if w.Rule == "dirty_marker_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want dirty_marker_failed warning, got %+v", res.Warnings)
	}
	st, sConf := Status(f.cs, "hosta")
	if sConf != nil {
		t.Fatal(sConf)
	}
	if st[0].Dirty {
		t.Fatal("status must stay clean when marker write failed")
	}
}

func TestSyncClearsStaleErrorWhenClean(t *testing.T) {
	f := newFixture(t)
	writeSyncError(f.cs[0].Identity(), "hosta", "deadbeef", "cleanup", "stale leftover")
	st, _ := Status(f.cs, "hosta")
	if !strings.Contains(st[0].LastSyncError, "stale leftover") {
		t.Fatalf("precondition: leftover error missing: %+v", st[0])
	}
	res, conf := Sync(context.Background(), f.cs, "hosta")
	if conf != nil || len(res) != 1 || res[0].Updated {
		t.Fatalf("clean sync = %+v / %v", res, conf)
	}
	st, _ = Status(f.cs, "hosta")
	if st[0].LastSyncError != "" || st[0].Dirty {
		t.Fatalf("stale last_sync_error survived: %+v", st[0])
	}
}
