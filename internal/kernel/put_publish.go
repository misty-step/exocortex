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

// afterConvergeHook, when non-nil, runs once at the start of replay
// after a successful converge, then disarms. Production leaves it nil.
var afterConvergeHook func()

func pushRepo(dir string) error {
	if h := pushOverride; h != nil {
		pushOverride = nil
		return h()
	}
	_, err := git(dir, "push")
	return err
}

func handlePushFailure(c *Cortex, in PutInput, rel, dir, abs, base, op string, res *PutResult, candidate []byte, perr error, remaining int) *Conflict {
	switch classifyPushError(perr) {
	case pushMoved:
		return recoverMoved(c, in, rel, dir, abs, base, op, res, candidate, perr, remaining)
	case pushRefused:
		unwind := unwindAndConverge(dir, base, rel)
		if len(unwind) > 0 {
			return writerUnavailable(op, rel, in, perr, unwind, "", nil, "rejected")
		}
		return publicationConflict("publish_rejected", op, rel, in, perr, unwind,
			"the remote refused this push (auth, hook, or policy); fix the refusal and retry; this is not a lost write race")
	default:
		return recoverUnknown(in, rel, dir, base, op, res, candidate, perr)
	}
}

func recoverMoved(c *Cortex, in PutInput, rel, dir, abs, base, op string, res *PutResult, candidate []byte, perr error, remaining int) *Conflict {
	tip, err := fetchTip(dir)
	if err != nil {
		conf := writerUnavailable(op, rel, in, perr, nil, "", nil, "rejected")
		conf.Detail["recovery_error"] = pushStderr(err)
		return conf
	}
	remote, ok, err := fileAt(dir, tip, rel)
	if err != nil {
		conf := writerUnavailable(op, rel, in, perr, nil, tip, nil, "rejected")
		conf.Detail["observation_error"] = pushStderr(err)
		return conf
	}
	unwind := convergeEvaluated(dir, base, rel, tip)
	if len(unwind) > 0 || !writerAt(dir, tip) {
		return writerUnavailable(op, rel, in, perr, unwind, tip, remote, "rejected")
	}
	if pathChanged(op, in.Expects, remote, ok) {
		return observedPathConflict(op, rel, remote, ok, in, perr, unwind)
	}
	if remaining <= 0 {
		conf := publicationConflict("publish_rejected", op, rel, in, perr, unwind,
			"unrelated remote movement exhausted bounded replay; the last push was rejected and did not land; retry the same path")
		conf.Detail["reason"] = "contention_exhausted"
		conf.Detail["observed_tip"] = tip
		conf.Detail["converged"] = true
		return conf
	}
	return replayCandidate(c, in, rel, dir, abs, op, res, candidate, remaining-1)
}

func recoverUnknown(in PutInput, rel, dir, base, op string, res *PutResult, candidate []byte, perr error) *Conflict {
	tip, err := fetchTip(dir)
	if err != nil {
		return keepUnknown(op, rel, in, perr, err, "")
	}
	remote, ok, err := fileAt(dir, tip, rel)
	if err != nil {
		return keepUnknown(op, rel, in, perr, err, tip)
	}
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
	if len(unwind) > 0 || !writerAt(dir, tip) {
		return writerUnavailable(op, rel, in, perr, unwind, tip, remote, "unknown")
	}
	return publicationConflict("publish_unknown", op, rel, in, perr, unwind,
		"could not tell whether the push landed; re-read with get before retrying; do not create a second path")
}

func replayCandidate(c *Cortex, in PutInput, rel, dir, abs, op string, res *PutResult, candidate []byte, remaining int) *Conflict {
	if conf := preflight(dir, rel, op == "update"); conf != nil {
		preservePayload(in, conf)
		return conf
	}
	if h := afterConvergeHook; h != nil {
		afterConvergeHook = nil
		h()
	}
	base, gerr := git(dir, "rev-parse", "HEAD")
	if gerr != nil {
		conf := conflict("refresh_failed", op, rel, "replay lost HEAD after converge; retry",
			map[string]any{"detail": gerr.(*GitError).Stderr})
		preservePayload(in, conf)
		return conf
	}
	base = strings.TrimSpace(base)
	if werr := atomicWrite(abs, candidate); werr != nil {
		conf := conflict("write_failed", op, rel, "fix filesystem access and retry",
			map[string]any{"detail": werr.Error()})
		preservePayload(in, conf)
		return conf
	}
	msg := fmt.Sprintf("cortex(%s): exocortex put %s via %s", commitScope(c, rel), rel, agentID(in.Agent))
	head, conf := commitPath(dir, rel, msg, op)
	if conf != nil {
		preservePayload(in, conf)
		return conf
	}
	res.Commit = head
	res.Revision = Revision(candidate)
	if !hasUpstream(dir) {
		return nil
	}
	if perr := pushRepo(dir); perr != nil {
		return handlePushFailure(c, in, rel, dir, abs, base, op, res, candidate, perr, remaining)
	}
	res.Pushed = true
	return nil
}

func observedPathConflict(op, rel string, remote []byte, ok bool, in PutInput, perr error, unwind []string) *Conflict {
	actual := ""
	if ok {
		actual = Revision(remote)
	}
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
	if unwind == nil {
		unwind = []string{}
	}
	conf := conflict(code, op, rel, hint, map[string]any{
		"push_stderr": pushStderr(perr),
		"unwind":      unwind,
	})
	preservePayload(in, conf)
	return conf
}

func keepUnknown(op, rel string, in PutInput, perr, recoveryErr error, tip string) *Conflict {
	conf := conflict("publish_unknown", op, rel,
		"could not tell whether the push landed and recovery could not observe the remote; repair access before retrying; do not create a second path",
		map[string]any{
			"push_stderr":    pushStderr(perr),
			"recovery_error": pushStderr(recoveryErr),
			"unwound":        false,
		})
	if tip != "" {
		conf.Detail["observed_tip"] = tip
	}
	preservePayload(in, conf)
	return conf
}

func writerUnavailable(op, rel string, in PutInput, perr error, unwind []string, tip string, remote []byte, remoteOutcome string) *Conflict {
	hint := "the push was rejected and the writer could not converge for replay; inspect the publisher clone; do not create a second path"
	switch remoteOutcome {
	case "landed":
		hint = "the push landed on the remote but the publisher clone did not converge; repair the writer to proved_commit/proved_revision; do not retry this write and do not create a second path"
	case "unknown":
		hint = "the push outcome is unknown and the publisher clone did not converge; repair the writer before using get; do not retry or create a second path"
	}
	conf := publicationConflict("writer_unavailable", op, rel, in, perr, unwind, hint)
	if conf.Detail == nil {
		conf.Detail = map[string]any{}
	}
	conf.Detail["remote"] = remoteOutcome
	conf.Detail["converged"] = false
	if remoteOutcome == "landed" {
		conf.Detail["proved_commit"] = tip
		if remote != nil {
			conf.Detail["proved_revision"] = Revision(remote)
		}
	} else if tip != "" {
		conf.Detail["observed_tip"] = tip
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
	if strings.Contains(msg, "permission to ") && strings.Contains(msg, " denied to ") {
		return true
	}
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

func fileAt(dir, rev, rel string) ([]byte, bool, error) {
	path := filepath.ToSlash(rel)
	entry, err := git(dir, "ls-tree", "-z", "--name-only", rev, "--", ":(literal)"+path)
	if err != nil {
		return nil, false, err
	}
	if entry == "" {
		return nil, false, nil
	}
	if entry != path+"\x00" {
		return nil, false, fmt.Errorf("git ls-tree returned unexpected path %q for %q", entry, path)
	}
	raw, err := git(dir, "show", rev+":"+path)
	if err != nil {
		return nil, false, err
	}
	return []byte(raw), true, nil
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
