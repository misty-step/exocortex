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
	"syscall"
	"time"

	"github.com/misty-step/exocortex/internal/qmd"
)

// SyncMarker records the pending or synced state of a cortex.
type SyncMarker struct {
	Cortex string    `json:"cortex"`
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
}

// SyncResult is the result of an `exocortex sync` invocation.
type SyncResult struct {
	Cortex        string `json:"cortex"`
	Updated       bool   `json:"updated"`
	IndexedCommit string `json:"indexed_commit,omitempty"`
	Embedded      bool   `json:"embedded"`
	DirtyCleared  bool   `json:"dirty_cleared"`
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

func cortexStateDir(cortexName string) (string, error) {
	cfg, err := ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfg, "state", cortexName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func dirtyMarkerDir(cortexName string) (string, error) {
	sDir, err := cortexStateDir(cortexName)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(sDir, "dirty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// markDirty atomically writes an immutable marker file for a cortex after a successful push.
func markDirty(cortexName, commit string) error {
	dir, err := dirtyMarkerDir(cortexName)
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

func acquireSyncLock(name string) (*cortexLock, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, name+"-sync.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &cortexLock{f: f}, nil
}

// syncHook is an optional test hook called right after qmd embed and before deleting snapshot markers.
var syncHook func(cortexName string)

// Sync executes qmd update and qmd embed under a single-owner lock,
// advancing .synced and deleting only the snapshotted dirty markers.
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
	for _, c := range targets {
		res, conf := syncOne(ctx, c)
		if conf != nil {
			return nil, conf
		}
		results = append(results, *res)
	}
	return results, nil
}

func syncOne(ctx context.Context, c Cortex) (*SyncResult, *Conflict) {
	lock, lerr := acquireSyncLock(c.Name)
	if lerr != nil {
		return nil, conflict("lock_failed", "sync", c.Name, "fix sync lock access and retry", map[string]any{"detail": lerr.Error()})
	}
	defer lock.release()

	dDir, err := dirtyMarkerDir(c.Name)
	if err != nil {
		return nil, conflict("state_failed", "sync", c.Name, "fix state directory access and retry", map[string]any{"detail": err.Error()})
	}
	sDir, _ := cortexStateDir(c.Name)
	errorPath := filepath.Join(sDir, "sync_error.json")
	syncedPath := filepath.Join(sDir, "synced.json")

	// Snapshot all pending dirty markers at the start of sync
	entries, rerr := os.ReadDir(dDir)
	if errors.Is(rerr, fs.ErrNotExist) || len(entries) == 0 {
		return &SyncResult{Cortex: c.Name, Updated: false, Embedded: false}, nil
	}
	if rerr != nil {
		return nil, conflict("state_failed", "sync", c.Name, "failed to read dirty markers", map[string]any{"detail": rerr.Error()})
	}

	var snapshotFiles []string
	var newestCommit string
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		snapshotFiles = append(snapshotFiles, entry.Name())
		b, err := os.ReadFile(filepath.Join(dDir, entry.Name()))
		if err != nil {
			return nil, conflict("state_failed", "sync", c.Name,
				"failed to read dirty marker file; markers retained",
				map[string]any{"file": entry.Name(), "detail": err.Error()})
		}
		var m SyncMarker
		if err := json.Unmarshal(b, &m); err != nil || m.Commit == "" {
			return nil, conflict("state_failed", "sync", c.Name,
				"malformed dirty marker file; markers retained",
				map[string]any{"file": entry.Name()})
		}
		newestCommit = m.Commit
	}

	if len(snapshotFiles) == 0 {
		return &SyncResult{Cortex: c.Name, Updated: false, Embedded: false}, nil
	}

	recordError := func(stage, detail string) {
		errData, _ := json.MarshalIndent(map[string]any{
			"cortex": c.Name,
			"commit": newestCommit,
			"stage":  stage,
			"error":  detail,
			"at":     time.Now().UTC().Format(time.RFC3339),
		}, "", "  ")
		_ = atomicWrite(errorPath, errData)
	}

	// 1. Run qmd update <cortex>
	if err := qmd.Update(ctx, c.Name); err != nil {
		recordError("update", err.Error())
		return nil, conflict("sync_failed", "sync", c.Name, "qmd update failed; inspect cortex index", map[string]any{"detail": err.Error()})
	}

	// 2. Run qmd embed -c <cortex>
	if err := qmd.Embed(ctx, c.Name); err != nil {
		recordError("embed", err.Error())
		return nil, conflict("embed_failed", "sync", c.Name, "qmd embed failed; inspect vector models and sqlite index", map[string]any{"detail": err.Error()})
	}

	if syncHook != nil {
		syncHook(c.Name)
	}

	// 3. Update synced.json with newest commit in snapshot
	syncedMarker := SyncMarker{
		Cortex: c.Name,
		Commit: newestCommit,
		At:     time.Now().UTC(),
	}
	syncedData, err := json.MarshalIndent(syncedMarker, "", "  ")
	if err != nil {
		return nil, conflict("state_failed", "sync", c.Name, "failed to marshal sync state", map[string]any{"detail": err.Error()})
	}
	if err := atomicWrite(syncedPath, syncedData); err != nil {
		return nil, conflict("state_failed", "sync", c.Name, "failed to write synced state; markers retained", map[string]any{"detail": err.Error()})
	}
	_ = os.Remove(errorPath)

	// 4. Delete ONLY the snapshotted files (leaving any files created during sync intact!)
	deletionFailed := false
	for _, fName := range snapshotFiles {
		if err := os.Remove(filepath.Join(dDir, fName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			deletionFailed = true
		}
	}

	// Check if any arrivals appeared during sync
	remaining, _ := os.ReadDir(dDir)
	cleared := !deletionFailed && len(remaining) == 0

	return &SyncResult{
		Cortex:        c.Name,
		Updated:       true,
		IndexedCommit: newestCommit,
		Embedded:      true,
		DirtyCleared:  cleared,
	}, nil
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
		st := StatusResult{Cortex: c.Name}
		sDir, _ := cortexStateDir(c.Name)
		dDir, _ := dirtyMarkerDir(c.Name)

		if entries, err := os.ReadDir(dDir); err == nil && len(entries) > 0 {
			var validMarkers []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					validMarkers = append(validMarkers, e.Name())
				}
			}
			if len(validMarkers) > 0 {
				st.Dirty = true
				st.DirtyCount = len(validMarkers)
				sort.Strings(validMarkers)
				newestFile := validMarkers[len(validMarkers)-1]
				if b, rerr := os.ReadFile(filepath.Join(dDir, newestFile)); rerr == nil {
					var dm SyncMarker
					if json.Unmarshal(b, &dm) == nil {
						st.DirtyCommit = dm.Commit
						st.DirtyAt = dm.At.Format(time.RFC3339)
					}
				}
			}
		}

		syncedPath := filepath.Join(sDir, "synced.json")
		if sBytes, err := os.ReadFile(syncedPath); err == nil {
			var sm SyncMarker
			if json.Unmarshal(sBytes, &sm) == nil {
				st.SyncedCommit = sm.Commit
				st.SyncedAt = sm.At.Format(time.RFC3339)
			}
		}

		errorPath := filepath.Join(sDir, "sync_error.json")
		if eBytes, err := os.ReadFile(errorPath); err == nil {
			var em map[string]any
			if json.Unmarshal(eBytes, &em) == nil {
				if errStr, ok := em["error"].(string); ok {
					st.LastSyncError = errStr
				}
				if atStr, ok := em["at"].(string); ok {
					st.LastErrorAt = atStr
				}
			}
		}

		if root, err := effectiveRoot(&c); err == nil {
			if head, gerr := git(root, "rev-parse", "HEAD"); gerr == nil {
				st.HeadCommit = strings.TrimSpace(head)
			}
		}

		results = append(results, st)
	}
	return results, nil
}
