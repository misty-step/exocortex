package kernel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/misty-step/exocortex/internal/fm"
)

// PutInput is one write request. Payload travels separately from the
// destination path (SPEC: payload/destination separation).
type PutInput struct {
	CortexName string // optional explicit cortex
	Path       string // destination, resolved against the cortex root
	Payload    []byte
	Expects    string // "" = create-only; else required stored revision
	Agent      string // provenance agent id
	Via        string // "cli" | "mcp"
	OwnPayload bool   // true when the payload already lives in a caller-owned file (CLI --from file)
}

// PutResult is a successful put.
type PutResult struct {
	Operation string       `json:"operation"` // "create" | "update"
	Cortex    string       `json:"cortex"`
	Path      string       `json:"path"`
	Revision  string       `json:"revision"`
	Noop      bool         `json:"noop,omitempty"`
	Commit    string       `json:"commit,omitempty"`
	Pushed    bool         `json:"pushed,omitempty"`
	Warnings  []fm.Finding `json:"warnings,omitempty"`
}

// beforePushHook, when non-nil, runs once inside the critical section
// after the local commit and immediately before push, then disarms
// itself. Production leaves it nil; cross-host race tests use it to
// land a peer's winning commit on the remote between this put's
// refresh and its push, so the push is genuinely rejected.
var beforePushHook func()

// Put runs the pinned pipeline: lock → refresh → pre-flight → CAS →
// validate → no-op short-circuit → stamp → atomic write → VCS tail,
// all inside one per-cortex critical section.
func Put(ctx context.Context, cs []Cortex, in PutInput) (*PutResult, *Conflict) {
	op := "create"
	if in.Expects != "" {
		op = "update"
	}
	c, rel, err := Resolve(cs, in.CortexName, in.Path)
	if err != nil {
		return nil, conflict("resolve_failed", op, in.Path,
			"use a path inside a registered cortex or pass --cortex", map[string]any{"detail": err.Error()})
	}
	rel = filepath.ToSlash(rel)
	abs := filepath.Join(c.Path, filepath.FromSlash(rel))

	// Note: there is no intent inference and no separate missing_expects
	// code (operator decision 2026-08-21): bare put on any existing path
	// conflicts `exists`; updates simply require --expects. A malformed
	// or stale hash falls out as an ordinary CAS revision_conflict.

	// Create fast path: a locally existing destination answers `exists`
	// without touching git — a pull refusal over that file's staged or
	// dirty state must not mask the answer. The post-refresh CAS below
	// re-checks absence against fresh state.
	if op == "create" {
		if _, serr := os.Stat(abs); serr == nil {
			return nil, conflict("exists", op, rel,
				"bare put creates only; read the note with get and update it with --expects", nil)
		}
	}

	lock, lerr := acquireLock(c.Name)
	if lerr != nil {
		return nil, conflict("lock_failed", op, rel, "fix lock-file access and retry", map[string]any{"detail": lerr.Error()})
	}
	defer lock.release()

	res := &PutResult{Operation: op, Cortex: c.Name, Path: rel}

	var base string
	if c.VCS == "daybook" {
		// Cleanliness gate BEFORE refresh: with the tree already clean,
		// the mandated `pull --rebase --autostash` has no in-flight work
		// to cycle through a stash/pop conflict window.
		if conf := preflight(c.Path, rel, op == "update"); conf != nil {
			return nil, conf
		}
		if hasUpstream(c.Path) {
			if _, gerr := git(c.Path, "pull", "--rebase", "--autostash"); gerr != nil {
				return nil, conflict("refresh_failed", op, rel,
					"resolve the pull failure and retry; nothing was written",
					map[string]any{"detail": gerr.(*GitError).Stderr})
			}
		}
		b, gerr := git(c.Path, "rev-parse", "HEAD")
		if gerr != nil {
			return nil, conflict("refresh_failed", op, rel, "the cortex has no HEAD commit; make an initial commit", map[string]any{"detail": gerr.(*GitError).Stderr})
		}
		base = strings.TrimSpace(b)
		// Recheck after refresh: the pull window may have changed state.
		if conf := preflight(c.Path, rel, op == "update"); conf != nil {
			return nil, conf
		}
	}

	// CAS against fresh state.
	stored, serr := os.ReadFile(abs)
	switch {
	case op == "create" && serr == nil:
		return nil, conflict("exists", op, rel, "bare put creates only; read the note with get and update it with --expects", nil)
	case op == "create" && !errors.Is(serr, fs.ErrNotExist):
		return nil, conflict("read_failed", op, rel, "fix filesystem access and retry", map[string]any{"detail": serr.Error()})
	case op == "update" && errors.Is(serr, fs.ErrNotExist):
		return nil, conflict("not_found", op, rel, "the note vanished; search the cortex and re-apply on top of current state", nil)
	case op == "update" && serr != nil:
		return nil, conflict("read_failed", op, rel, "fix filesystem access and retry", map[string]any{"detail": serr.Error()})
	}
	storedRev := ""
	if op == "update" {
		storedRev = Revision(stored)
		if storedRev != in.Expects {
			conf := conflict("revision_conflict", op, rel,
				"remote or a peer moved this note; re-read with get, re-apply your change on top, retry with the new revision",
				map[string]any{"expected": in.Expects, "actual": storedRev})
			preservePayload(in, conf)
			return nil, conf
		}
	}

	// Validate under the cortex profile BEFORE any comparison or write.
	findings, verr := fm.Validate(c.Profile, fm.Split(in.Payload))
	if verr != nil {
		f, _ := fm.ContractFinding(verr)
		return nil, conflict("invalid_note", op, rel,
			"fix the payload frontmatter; the cortex profile is "+c.Profile,
			map[string]any{"rule": f.Rule, "message": f.Message})
	}
	for _, f := range findings {
		if f.Level == "warning" {
			res.Warnings = append(res.Warnings, f)
		}
	}

	// created is immutable (SPEC): an update that changes or drops an
	// existing non-empty created aborts as data; filling a MISSING
	// created is legal. Comparison uses the lexical scalar text —
	// yaml.v3 would decode unquoted timestamps to time.Time and hide
	// the common case from map assertions.
	if op == "update" {
		storedNote := fm.Split(stored)
		if storedCreated, ok := fm.TopLevelScalar(storedNote, "created"); ok && strings.TrimSpace(storedCreated) != "" {
			submitted, pok := fm.TopLevelScalar(fm.Split(in.Payload), "created")
			if !pok || submitted != storedCreated {
				return nil, conflict("created_immutable", op, rel,
					"created never changes; resubmit with created: "+storedCreated,
					map[string]any{"stored": storedCreated, "submitted": submitted})
			}
		}
	}
	// No-op short-circuit: identical retries are free. Two forms
	// qualify: the PINNED byte-equality against stored content (the
	// get → put-unchanged round trip carries the stamp back), and
	// draft-retry equivalence, where the payload matches stored minus
	// the kernel-owned provenance block.
	if op == "update" && (bytes.Equal(stored, in.Payload) ||
		bytes.Equal(fm.StripProvenance(stored), in.Payload)) {
		res.Noop = true
		res.Revision = storedRev
		return res, nil
	}

	// Stamp provenance, atomic write.
	final := fm.SpliceProvenance(in.Payload, fm.Provenance{
		Agent: agentID(in.Agent), At: time.Now(), Via: viaID(in.Via),
	})
	if werr := atomicWrite(abs, final); werr != nil {
		return nil, conflict("write_failed", op, rel, "fix filesystem access and retry", map[string]any{"detail": werr.Error()})
	}
	res.Revision = Revision(final)

	// VCS tail.
	if c.VCS == "daybook" {
		if _, gerr := git(c.Path, "add", "--", filepath.FromSlash(rel)); gerr != nil {
			return nil, conflict("stage_failed", op, rel, "inspect the repository state; the file is written but uncommitted", map[string]any{"detail": gerr.(*GitError).Stderr})
		}
		msg := fmt.Sprintf("vault(%s): exocortex put %s via %s", commitScope(c, rel), rel, agentID(in.Agent))
		if _, gerr := git(c.Path, "commit", "-m", msg, "--", filepath.FromSlash(rel)); gerr != nil {
			return nil, conflict("commit_failed", op, rel, "inspect the repository state; the file is written but uncommitted", map[string]any{"detail": gerr.(*GitError).Stderr})
		}
		head, gerr := git(c.Path, "rev-parse", "HEAD")
		if gerr != nil {
			return nil, conflict("commit_failed", op, rel, "the file is written and committed; revision lookup failed", map[string]any{"detail": gerr.(*GitError).Stderr})
		}
		res.Commit = strings.TrimSpace(head)
		if h := beforePushHook; h != nil {
			beforePushHook = nil
			h()
		}
		if !hasUpstream(c.Path) {
			// Remoteless cortex: commit locally; nothing to push and no
			// cross-host CAS. The next upstream-aware put refreshes.
			return res, nil
		}
		if _, perr := git(c.Path, "push"); perr != nil {
			ge := perr.(*GitError)
			unwindErrs := unwindAndConverge(c.Path, base, rel)
			actual := Revision(mustRead(abs))
			var conf *Conflict
			if op == "create" {
				// A create that loses a push race means the winner's
				// note now exists at this path (operator decision:
				// existing path -> exists).
				conf = conflict("exists", op, rel,
					"a peer created this note first; after the automatic restore-to-remote, re-read with get and update it with --expects",
					map[string]any{
						"actual":      actual,
						"push_stderr": strings.TrimSpace(ge.Stderr),
						"unwind":      unwindErrs,
						"base":        base,
					})
			} else {
				conf = conflict("revision_conflict", op, rel,
					"remote moved; re-read with get and retry",
					map[string]any{
						"expected":    in.Expects,
						"actual":      actual,
						"push_stderr": strings.TrimSpace(ge.Stderr),
						"unwind":      unwindErrs,
						"base":        base,
					})
			}
			preservePayload(in, conf)
			return nil, conf
		}
		res.Pushed = true
	}
	return res, nil
}

// hasUpstream reports whether the branch tracks a remote — remoteless
// git cortices skip refresh and push (no cross-host CAS exists there).
func hasUpstream(dir string) bool {
	_, err := git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	return err == nil
}

// preflight aborts on foreign staged paths and foreign unstaged
// modifications — all conflict-as-data, before any write. Untracked
// files outside the destination are allowed. The
// destination check runs for updates only: a create overwrites nothing,
// so the CAS existence check answers `exists` for any pre-existing
// destination (operator decision 2026-08-21), while foreign staged and
// unstaged state abort in BOTH modes — the push-failure unwind's mixed
// reset would unstage every foreign staged path in the index.
func preflight(repo, rel string, checkDest bool) *Conflict {
	out, err := git(repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return conflict("status_failed", "put", rel, "inspect the repository state and retry", map[string]any{"detail": err.(*GitError).Stderr})
	}
	var stagedForeign, unstagedForeign []string
	destState := ""
	rest := out
	for len(rest) >= 4 { // "XY " + at least a 1-byte path
		x, y := rest[0], rest[1]
		if rest[2] != ' ' {
			return conflict("status_failed", "put", rel,
				"git status produced an unexpected record; inspect the repository manually",
				map[string]any{"detail": fmt.Sprintf("malformed porcelain record %q", rest[:min(16, len(rest))])})
		}
		rest = rest[3:] // XY plus its separator space
		var path string
		if i := strings.IndexByte(rest, 0); i >= 0 {
			path, rest = rest[:i], rest[i+1:]
		} else {
			path, rest = rest, ""
		}
		if x == 'R' || x == 'C' { // rename/copy: second NUL-separated orig path follows
			if i := strings.IndexByte(rest, 0); i >= 0 {
				rest = rest[i+1:]
			} else {
				rest = ""
			}
		}
		untracked := x == '?' && y == '?'
		staged := x != ' ' && !untracked // index differs from HEAD
		worktreeDirty := y == 'M' || y == 'D' || y == 'T' || y == 'C'
		switch {
		case path == rel:
			// The destination is never "foreign". For creates the CAS
			// existence check answers `exists` regardless of state;
			// only updates abort on destination dirtiness.
			if checkDest {
				switch {
				case staged:
					destState = "staged"
				case untracked || worktreeDirty:
					destState = "unstaged"
				}
			}
		case untracked:
			// untracked foreign files are someone's fresh work; allowed
		case staged:
			stagedForeign = append(stagedForeign, path)
		case worktreeDirty:
			unstagedForeign = append(unstagedForeign, path)
		}
	}
	if len(stagedForeign) > 0 {
		return conflict("foreign_staged_state", "put", rel,
			"another worker staged paths in this cortex; commit or unstage your own staged work first; never stash or discard theirs",
			map[string]any{"paths": stagedForeign})
	}
	if destState != "" {
		return conflict("dirty_destination", "put", rel,
			"the destination holds someone's in-flight work; inspect it with get and coordinate before overwriting",
			map[string]any{"state": destState})
	}
	if len(unstagedForeign) > 0 {
		return conflict("foreign_unstaged_state", "put", rel,
			"unrelated tracked files have unstaged edits; the push-failure unwind must never be able to reach them, so put aborts; leave them be or coordinate",
			map[string]any{"paths": unstagedForeign})
	}
	return nil
}

// unwindAndConverge restores the pre-put state path-scoped and then
// converges the clone onto the remote tip so a lost race never leaves
// stale bytes behind. HEAD moves with `reset --soft` — the index is
// never swept repo-wide, so even a foreign path staged after pre-flight
// keeps its index entry. Only this operation's path is restored: an
// updated path from base via `git restore --staged --worktree`; a
// created path (absent from base) via `git rm --cached` plus file
// removal, because a leftover untracked file would block the ff merge.
// Errors are collected as strings; every step is best-effort and
// non-destructive beyond the touched path.
func unwindAndConverge(repo, base, rel string) []string {
	var errs []string
	if _, err := git(repo, "reset", "--soft", base); err != nil {
		errs = append(errs, err.Error())
	}
	if _, cerr := git(repo, "cat-file", "-e", base+":"+rel); cerr != nil {
		// Created by this operation: drop it from index and disk.
		if _, err := git(repo, "rm", "--cached", "--force", "--", filepath.FromSlash(rel)); err != nil {
			errs = append(errs, err.Error())
		}
		if rmErr := os.Remove(filepath.Join(repo, filepath.FromSlash(rel))); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			errs = append(errs, rmErr.Error())
		}
	} else if _, err := git(repo, "restore", "--source="+base, "--staged", "--worktree", "--", filepath.FromSlash(rel)); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := git(repo, "fetch"); err != nil {
		errs = append(errs, err.Error())
		return errs
	}
	if _, err := git(repo, "merge", "--ff-only", "@{u}"); err != nil {
		errs = append(errs, "ff-only restore failed; next put's refresh heals: "+err.Error())
	}
	return errs
}

// preservePayload persists an in-memory payload (stdin/MCP) when a put
// fails after validation, so the caller never loses work.
func preservePayload(in PutInput, conf *Conflict) {
	if in.OwnPayload || len(in.Payload) == 0 {
		return
	}
	dir, err := os.MkdirTemp("", "exocortex-payload-")
	if err != nil {
		return
	}
	p := filepath.Join(dir, "payload.md")
	if err := os.WriteFile(p, in.Payload, 0o644); err != nil {
		return
	}
	if conf.Detail == nil {
		conf.Detail = map[string]any{}
	}
	conf.Detail["payload_saved"] = p
}

func acquireLock(name string) (*cortexLock, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &cortexLock{f: f}, nil
}

type cortexLock struct{ f *os.File }

func (l *cortexLock) release() {
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
}

func atomicWrite(abs string, data []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".exocortex-put-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return err
	}
	tmpName = "" // renamed away; nothing to clean
	return nil
}

func mustRead(abs string) []byte {
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	return b
}

func agentID(a string) string {
	if a != "" {
		return a
	}
	if a = os.Getenv("EXOCORTEX_AGENT"); a != "" {
		return a
	}
	return "unknown"
}

func viaID(v string) string {
	if v == "mcp" {
		return "mcp"
	}
	return "cli"
}

// commitScope mirrors daybook's vault(<area>): convention — the first
// path segment, or the cortex name for root-level notes.
func commitScope(c *Cortex, rel string) string {
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return c.Name
}
