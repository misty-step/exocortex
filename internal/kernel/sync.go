package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/exocortex/internal/qmd"
)

// SyncMarker records the pending or synced state of a cortex.
type SyncMarker struct {
	Cortex string    `json:"cortex"`
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
}

type SyncResult struct {
	Cortex        string `json:"cortex"`
	Updated       bool   `json:"updated"`
	IndexedCommit string `json:"indexed_commit,omitempty"`
	Embedded      bool   `json:"embedded"`
	DirtyCleared  bool   `json:"dirty_cleared"`
	Error         string `json:"error,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// StatusResult describes the current synchronization state and lag for one cortex.
type StatusResult struct {
	Cortex        string `json:"cortex"`
	Dirty         bool   `json:"dirty"`
	DirtyCount    int    `json:"dirty_count,omitempty"`
	DirtyCommit   string `json:"dirty_commit,omitempty"`
	DirtyAt       string `json:"dirty_at,omitempty"`
	SyncedCommit  string `json:"synced_commit,omitempty"`
	SyncedAt      string `json:"synced_at,omitempty"`
	HeadCommit    string `json:"head_commit,omitempty"`
	LastSyncError string `json:"last_sync_error,omitempty"`
	LastErrorAt   string `json:"last_error_at,omitempty"`
}

func cortexStatePath(cortexName string) (string, error) {
	cfg, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "state", cortexName), nil
}

func dirtyMarkerPath(cortexName string) (string, error) {
	sDir, err := cortexStatePath(cortexName)
	if err != nil {
		return "", err
	}
	return filepath.Join(sDir, "dirty"), nil
}

// markDirty atomically writes an immutable marker after a durable write.
// commit is the sync identity: a git SHA when one exists, otherwise the
// content revision. Empty identities are rejected by syncOne.
func markDirty(cortexName, commit string) error {
	dir, err := dirtyMarkerPath(cortexName)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	marker := SyncMarker{
		Cortex: cortexName,
		Commit: commit,
		At:     now,
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	fileName := fmt.Sprintf("%020d-%s.json", now.UnixNano(), commit)
	return atomicWrite(filepath.Join(dir, fileName), data)
}

func sameRoot(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if ea, err := filepath.EvalSymlinks(a); err == nil {
		a = ea
	}
	if eb, err := filepath.EvalSymlinks(b); err == nil {
		b = eb
	}
	return a == b
}

// syncHook is an optional test hook called right after qmd embed and before deleting snapshot markers.
var syncHook func(cortexName string)

// Sync executes qmd update and qmd embed under the same per-cortex
// write lock as Put, advancing synced.json and deleting only the
// snapshotted dirty markers. It fail-closes if the QMD collection
// does not point at the indexed root.

func writeSyncError(name, commit, stage, detail string) {
	sDir, err := cortexStatePath(name)
	if err != nil {
		return
	}
	errData, _ := json.MarshalIndent(map[string]any{
		"cortex": name,
		"commit": commit,
		"stage":  stage,
		"error":  detail,
		"at":     time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	_ = atomicWrite(filepath.Join(sDir, "sync_error.json"), errData)
}

func Sync(ctx context.Context, cs []Cortex, nameFlag string) ([]SyncResult, *Conflict) {
	var targets []Cortex
	if nameFlag != "" {
		c, err := ResolveCortex(cs, nameFlag)
		if err != nil {
			return nil, conflict("resolve_failed", "sync", "", "register a cortex or pass --cortex <name>", map[string]any{"detail": err.Error()})
		}
		targets = append(targets, *c)
	} else {
		targets = cs
	}

	var results []SyncResult
	var first *Conflict
	for _, c := range targets {
		res, conf := syncOne(ctx, c)
		if conf != nil {
			detail := ""
			if conf.Detail != nil {
				if d, ok := conf.Detail["detail"].(string); ok {
					detail = d
				}
			}
			sr := SyncResult{Cortex: c.Name, Error: conf.Code, Detail: detail}
			if res != nil {
				sr.Updated = res.Updated
				sr.IndexedCommit = res.IndexedCommit
				sr.Embedded = res.Embedded
				sr.DirtyCleared = res.DirtyCleared
			}
			results = append(results, sr)
			if first == nil {
				first = conf
			}
			continue
		}
		results = append(results, *res)
	}
	return results, first
}

func syncOne(ctx context.Context, c Cortex) (*SyncResult, *Conflict) {
	lock, lerr := acquireLock(c.Name)
	if lerr != nil {
		return nil, conflict("lock_failed", "sync", c.Name, "fix lock-file access and retry", map[string]any{"detail": lerr.Error()})
	}
	defer lock.release()

	dDir, err := dirtyMarkerPath(c.Name)
	if err != nil {
		return nil, conflict("state_failed", "sync", c.Name, "fix state directory access and retry", map[string]any{"detail": err.Error()})
	}
	sDir, _ := cortexStatePath(c.Name)
	errorPath := filepath.Join(sDir, "sync_error.json")
	syncedPath := filepath.Join(sDir, "synced.json")

	snapshotFiles, newestCommit, empty, conf := snapshotDirtyMarkers(c.Name, dDir, errorPath)
	if conf != nil {
		return nil, conf
	}
	if empty {
		return &SyncResult{Cortex: c.Name, Updated: false, Embedded: false}, nil
	}

	out := &SyncResult{Cortex: c.Name}
	recordError := func(stage, detail string) {
		writeSyncError(c.Name, newestCommit, stage, detail)
	}
	if conf = verifyIndexedRoot(ctx, c, out, recordError); conf != nil {
		return out, conf
	}
	if conf = runQmdSync(ctx, c, newestCommit, out, recordError); conf != nil {
		return out, conf
	}
	if syncHook != nil {
		syncHook(c.Name)
	}
	return persistSyncedState(c, snapshotFiles, newestCommit, dDir, errorPath, syncedPath, out)
}

func snapshotDirtyMarkers(name, dDir, errorPath string) (files []string, newest string, empty bool, conf *Conflict) {
	entries, rerr := os.ReadDir(dDir)
	if errors.Is(rerr, fs.ErrNotExist) || (rerr == nil && len(entries) == 0) {
		if err := os.Remove(errorPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, "", false, conflict("state_failed", "sync", name, "failed to clear last_sync_error; inspect state and retry", map[string]any{"detail": err.Error()})
		}
		return nil, "", true, nil
	}
	if rerr != nil {
		return nil, "", false, conflict("state_failed", "sync", name, "failed to read dirty markers", map[string]any{"detail": rerr.Error()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, entry.Name())
		b, err := os.ReadFile(filepath.Join(dDir, entry.Name()))
		if err != nil {
			writeSyncError(name, newest, "marker", err.Error())
			return nil, "", false, conflict("state_failed", "sync", name,
				"failed to read dirty marker file; markers retained",
				map[string]any{"file": entry.Name(), "detail": err.Error()})
		}
		var m SyncMarker
		if err := json.Unmarshal(b, &m); err != nil || m.Commit == "" {
			writeSyncError(name, newest, "marker", "malformed dirty marker file")
			return nil, "", false, conflict("state_failed", "sync", name,
				"malformed dirty marker file; markers retained",
				map[string]any{"file": entry.Name()})
		}
		newest = m.Commit
	}
	if len(files) == 0 {
		return nil, "", true, nil
	}
	return files, newest, false, nil
}

func verifyIndexedRoot(ctx context.Context, c Cortex, out *SyncResult, recordError func(string, string)) *Conflict {
	var wantRoot string
	if c.VCS == "daybook" {
		w := writerDir(&c)
		if w == "" {
			recordError("root", "publisher clone missing")
			return conflict("writer_unavailable", "sync", c.Name,
				"failed to resolve the indexed root; markers retained",
				map[string]any{"detail": "publisher clone missing"})
		}
		wantRoot = w
	} else {
		wantRoot = c.Path
	}
	gotRoot, rerr := qmd.CollectionPath(ctx, c.Name)
	if rerr != nil {
		recordError("collection", rerr.Error())
		return conflict("index_root_unverified", "sync", c.Name,
			"qmd collection show failed; markers retained. Point the collection at the indexed root and retry",
			map[string]any{"detail": rerr.Error(), "expected": wantRoot})
	}
	if !sameRoot(wantRoot, gotRoot) {
		recordError("collection", fmt.Sprintf("collection path %s != indexed root %s", gotRoot, wantRoot))
		return conflict("index_root_mismatch", "sync", c.Name,
			"qmd collection does not point at the indexed root; markers retained. Rebind the collection and retry",
			map[string]any{"expected": wantRoot, "actual": gotRoot})
	}
	return nil
}

func runQmdSync(ctx context.Context, c Cortex, newestCommit string, out *SyncResult, recordError func(string, string)) *Conflict {
	if err := qmd.Update(ctx, c.Name); err != nil {
		recordError("update", err.Error())
		return conflict("sync_failed", "sync", c.Name, "qmd update failed; inspect cortex index", map[string]any{"detail": err.Error()})
	}
	out.Updated = true
	out.IndexedCommit = newestCommit
	if err := qmd.Embed(ctx, c.Name); err != nil {
		recordError("embed", err.Error())
		return conflict("embed_failed", "sync", c.Name, "qmd embed failed; inspect vector models and sqlite index", map[string]any{"detail": err.Error()})
	}
	out.Embedded = true
	return nil
}

func persistSyncedState(c Cortex, snapshotFiles []string, newestCommit, dDir, errorPath, syncedPath string, out *SyncResult) (*SyncResult, *Conflict) {
	syncedMarker := SyncMarker{
		Cortex: c.Name,
		Commit: newestCommit,
		At:     time.Now().UTC(),
	}
	syncedData, err := json.MarshalIndent(syncedMarker, "", "  ")
	if err != nil {
		return out, conflict("state_failed", "sync", c.Name, "failed to marshal sync state", map[string]any{"detail": err.Error()})
	}
	if err := atomicWrite(syncedPath, syncedData); err != nil {
		writeSyncError(c.Name, newestCommit, "synced", err.Error())
		return out, conflict("state_failed", "sync", c.Name, "failed to write synced state; markers retained", map[string]any{"detail": err.Error()})
	}
	for _, fName := range snapshotFiles {
		if err := os.Remove(filepath.Join(dDir, fName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			writeSyncError(c.Name, newestCommit, "cleanup", err.Error())
			return out, conflict("state_failed", "sync", c.Name, "index advanced but snapshotted markers were not deleted; inspect state and retry", map[string]any{"detail": err.Error()})
		}
	}
	remaining, rerr := os.ReadDir(dDir)
	if rerr != nil {
		writeSyncError(c.Name, newestCommit, "cleanup", rerr.Error())
		return out, conflict("state_failed", "sync", c.Name, "index advanced but remaining dirty markers could not be read; inspect state and retry", map[string]any{"detail": rerr.Error()})
	}
	if err := os.Remove(errorPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return out, conflict("state_failed", "sync", c.Name, "index advanced but last_sync_error could not be cleared; inspect state and retry", map[string]any{"detail": err.Error()})
	}
	out.DirtyCleared = len(remaining) == 0
	return out, nil
}

// Status inspects dirty markers, synced.json, and sync_error.json for cortices.
func Status(cs []Cortex, nameFlag string) ([]StatusResult, *Conflict) {
	var targets []Cortex
	if nameFlag != "" {
		c, err := ResolveCortex(cs, nameFlag)
		if err != nil {
			return nil, conflict("resolve_failed", "status", "", "register a cortex or pass --cortex <name>", map[string]any{"detail": err.Error()})
		}
		targets = append(targets, *c)
	} else {
		targets = cs
	}

	var results []StatusResult
	for _, c := range targets {
		st, conf := statusOne(c)
		if conf != nil {
			return nil, conf
		}
		results = append(results, st)
	}
	return results, nil
}

func statusOne(c Cortex) (StatusResult, *Conflict) {
	st := StatusResult{Cortex: c.Name}
	sDir, err := cortexStatePath(c.Name)
	if err != nil {
		return st, nil
	}
	if conf := fillDirtyStatus(&st, filepath.Join(sDir, "dirty")); conf != nil {
		return st, conf
	}
	fillSyncedStatus(&st, filepath.Join(sDir, "synced.json"))
	fillSyncErrorStatus(&st, filepath.Join(sDir, "sync_error.json"))
	fillHeadStatus(&st, c)
	return st, nil
}

func fillDirtyStatus(st *StatusResult, dDir string) *Conflict {
	entries, derr := os.ReadDir(dDir)
	if derr != nil {
		if errors.Is(derr, fs.ErrNotExist) {
			return nil
		}
		return conflict("state_failed", "status", st.Cortex, "failed to read dirty markers", map[string]any{"detail": derr.Error()})
	}
	if len(entries) == 0 {
		return nil
	}
	var validMarkers []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			validMarkers = append(validMarkers, e.Name())
		}
	}
	if len(validMarkers) == 0 {
		return nil
	}
	st.Dirty = true
	st.DirtyCount = len(validMarkers)
	sort.Strings(validMarkers)
	newestFile := validMarkers[len(validMarkers)-1]
	b, rerr := os.ReadFile(filepath.Join(dDir, newestFile))
	if rerr != nil {
		return nil
	}
	var dm SyncMarker
	if json.Unmarshal(b, &dm) == nil {
		st.DirtyCommit = dm.Commit
		st.DirtyAt = dm.At.Format(time.RFC3339)
	}
	return nil
}

func fillSyncedStatus(st *StatusResult, syncedPath string) {
	sBytes, err := os.ReadFile(syncedPath)
	if err != nil {
		return
	}
	var sm SyncMarker
	if json.Unmarshal(sBytes, &sm) == nil {
		st.SyncedCommit = sm.Commit
		st.SyncedAt = sm.At.Format(time.RFC3339)
	}
}

func fillSyncErrorStatus(st *StatusResult, errorPath string) {
	eBytes, err := os.ReadFile(errorPath)
	if err != nil {
		return
	}
	var em map[string]any
	if json.Unmarshal(eBytes, &em) != nil {
		return
	}
	if errStr, ok := em["error"].(string); ok {
		st.LastSyncError = errStr
	}
	if atStr, ok := em["at"].(string); ok {
		st.LastErrorAt = atStr
	}
}

func fillHeadStatus(st *StatusResult, c Cortex) {
	var root string
	if c.VCS != "daybook" {
		root = c.Path
	} else if w := writerDir(&c); w != "" {
		root = w
	}
	if root == "" {
		return
	}
	if head, gerr := git(root, "rev-parse", "HEAD"); gerr == nil {
		st.HeadCommit = strings.TrimSpace(head)
	}
}
