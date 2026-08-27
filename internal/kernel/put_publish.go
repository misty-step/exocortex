package kernel

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Bounded replay of an already-stamped candidate after unrelated ref
// movement. The losing commit is never rebased.
const maxPublishReplay = 2

type pushOutcome int

const (
	pushUnknown pushOutcome = iota
	pushMoved
	pushRefused
)

// pushOverride, when non-nil, replaces `git push` once and disarms.
// Production leaves it nil; tests inject transport/lost-response outcomes.
var pushOverride func() error

func pushRepo(dir string) error {
	if h := pushOverride; h != nil {
		pushOverride = nil
		return h()
	}
	_, err := git(dir, "push")
	return err
}

func handlePushFailure(c *Cortex, in PutInput, rel, dir, abs, base, op string, res *PutResult, perr error, remaining int) *Conflict {
	switch classifyPushError(perr) {
	case pushMoved:
		return recoverMoved(c, in, rel, dir, abs, base, op, res, perr, remaining)
	case pushRefused:
		unwind := unwindAndConverge(dir, base, rel)
		return publicationConflict("publish_rejected", op, rel, in, perr, unwind,
			"the remote refused this push (auth, hook, or policy); fix the refusal and retry; this is not a lost write race")
	default:
		return recoverUnknown(c, in, rel, dir, abs, base, op, res, perr)
	}
}

func recoverMoved(c *Cortex, in PutInput, rel, dir, abs, base, op string, res *PutResult, perr error, remaining int) *Conflict {
	candidate := mustRead(abs)
	tip, err := fetchTip(dir)
	if err != nil {
		return keepUnknown(op, rel, in, perr)
	}
	remote, ok := fileAt(dir, tip, rel)
	unwind := convergeEvaluated(dir, base, rel, tip)
	if pathChanged(op, in.Expects, remote, ok) {
		return observedPathConflict(op, rel, abs, in, perr, unwind)
	}
	if len(unwind) > 0 || !writerAt(dir, tip) {
		return writerUnavailable(op, rel, in, perr, unwind, tip, remote, "rejected")
	}
	if remaining <= 0 {
		return publicationConflict("publish_unknown", op, rel, in, perr, unwind,
			"could not tell whether the push landed; re-read with get before retrying; do not create a second path")
	}
	return replayCandidate(c, in, rel, dir, abs, op, res, candidate, remaining-1)
}

func recoverUnknown(_ *Cortex, in PutInput, rel, dir, abs, base, op string, res *PutResult, perr error) *Conflict {
	candidate := mustRead(abs)
	tip, err := fetchTip(dir)
	if err != nil {
		return keepUnknown(op, rel, in, perr)
	}
	remote, ok := fileAt(dir, tip, rel)
	unwind := convergeEvaluated(dir, base, rel, tip)
	if ok && bytes.Equal(remote, candidate) {
		if len(unwind) > 0 || !writerAt(dir, tip) {
			return writerUnavailable(op, rel, in, perr, unwind, tip, remote, "landed")
		}
		res.Commit = tip
		res.Revision = Revision(candidate)
		res.Pushed = true
		return nil
	}
	return publicationConflict("publish_unknown", op, rel, in, perr, unwind,
		"could not tell whether the push landed; re-read with get before retrying; do not create a second path")
}

func replayCandidate(c *Cortex, in PutInput, rel, dir, abs, op string, res *PutResult, candidate []byte, remaining int) *Conflict {
	if conf := preflight(dir, rel, op == "update"); conf != nil {
		preservePayload(in, conf)
		return conf
	}
	base, gerr := git(dir, "rev-parse", "HEAD")
	if gerr != nil {
		return conflict("refresh_failed", op, rel, "replay lost HEAD after converge; retry",
			map[string]any{"detail": gerr.(*GitError).Stderr})
	}
	base = strings.TrimSpace(base)
	if werr := atomicWrite(abs, candidate); werr != nil {
		return conflict("write_failed", op, rel, "fix filesystem access and retry",
			map[string]any{"detail": werr.Error()})
	}
	msg := fmt.Sprintf("vault(%s): exocortex put %s via %s", commitScope(c, rel), rel, agentID(in.Agent))
	head, conf := commitPath(dir, rel, msg, op)
	if conf != nil {
		return conf
	}
	res.Commit = head
	res.Revision = Revision(candidate)
	if !hasUpstream(dir) {
		return nil
	}
	if perr := pushRepo(dir); perr != nil {
		return handlePushFailure(c, in, rel, dir, abs, base, op, res, perr, remaining)
	}
	res.Pushed = true
	return nil
}

func observedPathConflict(op, rel, abs string, in PutInput, perr error, unwind []string) *Conflict {
	actual := Revision(mustRead(abs))
	hint := "a peer created this note first; after the automatic restore-to-remote, re-read with get and update it with --expects"
	code := "exists"
	if op != "create" {
		code = "revision_conflict"
		hint = "remote moved this note; re-read with get and retry"
	}
	conf := publicationConflict(code, op, rel, in, perr, unwind, hint)
	if conf.Detail == nil {
		conf.Detail = map[string]any{}
	}
	conf.Detail["actual"] = actual
	if op != "create" {
		conf.Detail["expected"] = in.Expects
	}
	return conf
}

func publicationConflict(code, op, rel string, in PutInput, perr error, unwind []string, hint string) *Conflict {
	conf := conflict(code, op, rel, hint, map[string]any{
		"push_stderr": pushStderr(perr),
		"unwind":      unwind,
	})
	preservePayload(in, conf)
	return conf
}

func keepUnknown(op, rel string, in PutInput, perr error) *Conflict {
	conf := conflict("publish_unknown", op, rel,
		"could not tell whether the push landed; re-read with get before retrying; do not create a second path",
		map[string]any{
			"push_stderr": pushStderr(perr),
			"unwound":     false,
		})
	preservePayload(in, conf)
	return conf
}

func writerUnavailable(op, rel string, in PutInput, perr error, unwind []string, tip string, remote []byte, remoteOutcome string) *Conflict {
	hint := "the push was rejected and the writer could not converge for replay; inspect the publisher clone; do not create a second path"
	if remoteOutcome == "landed" {
		hint = "the push landed on the remote but the writer could not converge; re-read with get; do not create a second path"
	}
	conf := publicationConflict("writer_unavailable", op, rel, in, perr, unwind, hint)
	if conf.Detail == nil {
		conf.Detail = map[string]any{}
	}
	conf.Detail["remote"] = remoteOutcome
	conf.Detail["proved_commit"] = tip
	conf.Detail["converged"] = false
	if remote != nil {
		conf.Detail["proved_revision"] = Revision(remote)
	}
	return conf
}

func writerAt(dir, tip string) bool {
	head, err := git(dir, "rev-parse", "HEAD")
	return err == nil && strings.TrimSpace(head) == tip
}

func classifyPushError(err error) pushOutcome {
	msg := strings.ToLower(pushStderr(err))
	if isNonFastForward(msg) {
		return pushMoved
	}
	if isRemoteRefusal(msg) {
		return pushRefused
	}
	return pushUnknown
}

func isNonFastForward(msg string) bool {
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "tip of your current branch is behind") ||
		strings.Contains(msg, "cannot lock ref") ||
		strings.Contains(msg, "failed to update ref")
}

func isRemoteRefusal(msg string) bool {
	for _, n := range []string{
		"hook declined",
		"pre-receive hook",
		"remote rejected",
		"authentication failed",
		"permission denied",
		"could not read username",
		"access denied",
		"invalid username or password",
		"terminal prompts disabled",
	} {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

func pathChanged(op, expects string, remote []byte, ok bool) bool {
	if op == "create" {
		return ok
	}
	if !ok {
		return true
	}
	return Revision(remote) != expects
}

func fetchTip(dir string) (string, error) {
	if _, err := git(dir, "fetch"); err != nil {
		return "", err
	}
	tip, err := git(dir, "rev-parse", "@{u}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(tip), nil
}

func fileAt(dir, rev, rel string) ([]byte, bool) {
	raw, err := git(dir, "show", rev+":"+filepath.ToSlash(rel))
	if err != nil {
		return nil, false
	}
	return []byte(raw), true
}

func convergeEvaluated(dir, base, rel, tip string) []string {
	errs := unwindPath(dir, base, rel)
	return append(errs, convergeTo(dir, tip)...)
}

func pushStderr(err error) string {
	if err == nil {
		return ""
	}
	var ge *GitError
	if errors.As(err, &ge) {
		return strings.TrimSpace(ge.Stderr)
	}
	return strings.TrimSpace(err.Error())
}

func commitPath(dir, rel, msg, op string) (string, *Conflict) {
	relPath := filepath.FromSlash(rel)
	if _, gerr := git(dir, "add", "--", relPath); gerr != nil {
		return "", conflict("stage_failed", op, rel,
			"inspect the repository state; the file is written but uncommitted",
			map[string]any{"detail": gerr.(*GitError).Stderr})
	}
	if _, gerr := git(dir, "commit", "-m", msg, "--", relPath); gerr != nil {
		return "", conflict("commit_failed", op, rel,
			"inspect the repository state; the file is written but uncommitted",
			map[string]any{"detail": gerr.(*GitError).Stderr})
	}
	head, gerr := git(dir, "rev-parse", "HEAD")
	if gerr != nil {
		return "", conflict("commit_failed", op, rel,
			"the file is written and committed; revision lookup failed",
			map[string]any{"detail": gerr.(*GitError).Stderr})
	}
	return strings.TrimSpace(head), nil
}
