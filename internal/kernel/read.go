package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/misty-step/exocortex/internal/fm"
)

type GetResult struct {
	Cortex      string         `json:"cortex"`
	Path        string         `json:"path"`
	Revision    string         `json:"revision"`
	Frontmatter map[string]any `json:"frontmatter"`
	Content     string         `json:"content"`
}

type GetRequest struct {
	CortexName string
	Path       string
}

type GetOutcome struct {
	Result   *GetResult
	Conflict *Conflict
}

type readSnapshot struct {
	repo string
	sha  string
}

func (s readSnapshot) read(path string) ([]byte, error) {
	if s.sha != "" {
		raw, err := git(s.repo, "show", s.sha+":"+path)
		if err != nil {
			if tree, treeErr := git(s.repo, "ls-tree", "-z", s.sha, "--", path); treeErr == nil && tree == "" {
				return nil, fs.ErrNotExist
			}
			return nil, err
		}
		return []byte(raw), nil
	}
	return os.ReadFile(filepath.Join(s.repo, filepath.FromSlash(path)))
}

func (s readSnapshot) log(path string, limit int) ([]LogEntry, error) {
	args := []string{"log", fmt.Sprintf("-n%d", limit), "--format=%H%x1f%an%x1f%aI%x1f%s"}
	if s.sha != "" {
		args = append(args, s.sha)
	}
	args = append(args, "--", filepath.FromSlash(path))
	out, err := git(s.repo, args...)
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) == 4 {
			entries = append(entries, LogEntry{SHA: parts[0], Author: parts[1], Date: parts[2], Subject: parts[3]})
		}
	}
	return entries, nil
}

func (s readSnapshot) markdownPaths() ([]string, error) {
	if s.sha != "" {
		return s.committedMarkdown()
	}
	return s.walkMarkdown()
}

func (s readSnapshot) committedMarkdown() ([]string, error) {
	out, err := git(s.repo, "ls-tree", "-r", "-z", "--name-only", s.sha)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSuffix(out, "\x00")
	if out == "" {
		return nil, nil
	}
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" && strings.HasSuffix(path, ".md") {
			paths = append(paths, filepath.ToSlash(path))
		}
	}
	return paths, nil
}

func (s readSnapshot) walkMarkdown() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(s.repo, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(s.repo, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	return paths, err
}

func Get(cs []Cortex, nameFlag, p string) (*GetResult, *Conflict) {
	outcome := GetMany(cs, []GetRequest{{CortexName: nameFlag, Path: p}})[0]
	return outcome.Result, outcome.Conflict
}

func GetMany(cs []Cortex, requests []GetRequest) []GetOutcome {
	outcomes := make([]GetOutcome, len(requests))
	rels := make([]string, len(requests))
	groups, order := groupGetRequests(cs, requests, outcomes, rels)
	for _, c := range order {
		fillGetGroup(c, groups[c.Name], rels, outcomes)
	}
	return outcomes
}

func groupGetRequests(cs []Cortex, requests []GetRequest, outcomes []GetOutcome, rels []string) (map[string][]int, []*Cortex) {
	groups := make(map[string][]int)
	var order []*Cortex
	for i, request := range requests {
		c, rel, err := Resolve(cs, request.CortexName, request.Path)
		if err != nil {
			outcomes[i].Conflict = conflict("resolve_failed", "get", request.Path,
				"use a path inside a registered cortex or pass --cortex", map[string]any{"detail": err.Error()})
			continue
		}
		rels[i] = filepath.ToSlash(rel)
		if groups[c.Name] == nil {
			order = append(order, c)
		}
		groups[c.Name] = append(groups[c.Name], i)
	}
	return groups, order
}

func fillGetGroup(c *Cortex, indexes []int, rels []string, outcomes []GetOutcome) {
	conf := withReadSnapshot(c, "get", rels[indexes[0]], func(snapshot readSnapshot) *Conflict {
		for _, i := range indexes {
			raw, err := snapshot.read(rels[i])
			if err != nil {
				outcomes[i].Conflict = snapshotReadConflict(c, "get", rels[i], err)
				continue
			}
			outcomes[i].Result = getResult(c, rels[i], raw)
		}
		return nil
	})
	if conf == nil {
		return
	}
	for _, i := range indexes {
		outcomes[i].Conflict = conflictForRequest(conf, "get", rels[i])
	}
}

func getResult(c *Cortex, rel string, raw []byte) *GetResult {
	return &GetResult{
		Cortex:      c.Name,
		Path:        filepath.ToSlash(rel),
		Revision:    Revision(raw),
		Frontmatter: fm.ParseDocument(raw).Map,
		Content:     string(raw),
	}
}

type LogEntry struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

func Log(cs []Cortex, nameFlag, p string, limit int) ([]LogEntry, *Conflict) {
	if limit <= 0 {
		limit = 50
	}
	c, rel, err := Resolve(cs, nameFlag, p)
	if err != nil {
		return nil, conflict("resolve_failed", "log", p, "use a path inside a registered cortex or pass --cortex", map[string]any{"detail": err.Error()})
	}
	rel = filepath.ToSlash(rel)
	var entries []LogEntry
	conf := withReadSnapshot(c, "log", rel, func(snapshot readSnapshot) *Conflict {
		var readErr error
		entries, readErr = snapshot.log(rel, limit)
		if readErr == nil {
			return nil
		}
		if c.VCS == "daybook" {
			return snapshotUnavailable("log", rel, readErr)
		}
		return conflict("log_unavailable", "log", rel,
			"lineage requires a git-backed cortex; check that the cortex path is a repository",
			map[string]any{"detail": gitDetail(readErr)})
	})
	return entries, conf
}

type LintResult struct {
	Cortex   string       `json:"cortex"`
	Path     string       `json:"path,omitempty"`
	Findings []fm.Finding `json:"findings"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
}

func Lint(cs []Cortex, nameFlag, p string) (*LintResult, *Conflict) {
	var c *Cortex
	var rel string
	if p == "" {
		rc, rerr := ResolveCortex(cs, nameFlag)
		if rerr != nil {
			return nil, conflict("resolve_failed", "lint", "",
				"register a cortex or pass --cortex <name>", map[string]any{"detail": rerr.Error()})
		}
		c = rc
	} else {
		rc, rrel, rerr := Resolve(cs, nameFlag, p)
		if rerr != nil {
			return nil, conflict("resolve_failed", "lint", p,
				"use a path inside a registered cortex or pass --cortex", map[string]any{"detail": rerr.Error()})
		}
		c, rel = rc, filepath.ToSlash(rrel)
	}
	res := &LintResult{Cortex: c.Name, Path: rel}
	conf := withReadSnapshot(c, "lint", rel, func(snapshot readSnapshot) *Conflict {
		return lintSnapshot(c, rel, snapshot, res)
	})
	if conf != nil {
		return nil, conf
	}
	for _, f := range res.Findings {
		if f.Level == "error" {
			res.Errors++
		} else {
			res.Warnings++
		}
	}
	return res, nil
}

func lintSnapshot(c *Cortex, rel string, snapshot readSnapshot, res *LintResult) *Conflict {
	if rel != "" {
		return lintFile(c, rel, snapshot, res)
	}
	return lintWalk(c, snapshot, res)
}

func addLintFinding(res *LintResult, findings []fm.Finding, verr error) {
	if verr != nil {
		if f, ok := fm.ContractFinding(verr); ok {
			res.Findings = append(res.Findings, f)
		} else {
			res.Findings = append(res.Findings, fm.Finding{Level: "error", Rule: "read_failed", Message: verr.Error()})
		}
	}
	res.Findings = append(res.Findings, findings...)
}

func lintFile(c *Cortex, path string, snapshot readSnapshot, res *LintResult) *Conflict {
	raw, err := snapshot.read(path)
	if err != nil {
		if c.VCS == "daybook" {
			return snapshotUnavailable("lint", path, err)
		}
		addLintFinding(res, nil, err)
		return nil
	}
	_, findings, err := fm.ValidatePath(c.Profile, path, fm.ParseDocument(raw))
	addLintFinding(res, findings, err)
	return nil
}

func lintWalk(c *Cortex, snapshot readSnapshot, res *LintResult) *Conflict {
	paths, err := snapshot.markdownPaths()
	if err != nil {
		if c.VCS == "daybook" {
			return snapshotUnavailable("lint", "", err)
		}
		return conflict("walk_failed", "lint", "", "fix filesystem access and retry",
			map[string]any{"detail": err.Error()})
	}
	for _, path := range paths {
		if conf := lintFile(c, path, snapshot, res); conf != nil {
			return conf
		}
	}
	return nil
}

func withReadSnapshot(c *Cortex, operation, path string, fn func(readSnapshot) *Conflict) *Conflict {
	if c.VCS != "daybook" {
		root, err := effectiveRoot(c)
		if err != nil {
			return snapshotUnavailable(operation, path, err)
		}
		return fn(readSnapshot{repo: root})
	}

	lock, lockConf := lockNamed(c.Name, operation, path)
	if lockConf != nil {
		return snapshotUnavailable(operation, path, errors.New(lockConf.Error()))
	}
	fail := func(err error) *Conflict {
		return attachUnlock(snapshotUnavailable(operation, path, err), lock.release(), operation, path)
	}
	existingWriter := writerDir(c) != ""
	root, err := effectiveRoot(c)
	if err != nil {
		return fail(err)
	}
	if existingWriter {
		if err := ffToUpstream(root); err != nil {
			return fail(err)
		}
	}
	if err := requireCleanPublisher(root); err != nil {
		return fail(err)
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return fail(err)
	}
	if head = strings.TrimSpace(head); head == "" {
		return fail(errors.New("publisher repository has no HEAD commit"))
	}
	snapshot := readSnapshot{repo: root, sha: head}
	rerr := lock.release()
	conf := fn(snapshot)
	return attachUnlock(conf, rerr, operation, path)
}

func snapshotUnavailable(operation, path string, err error) *Conflict {
	return conflict("cortex_unavailable", operation, path,
		"publisher committed snapshot is unavailable; check repository access and retry",
		map[string]any{"detail": err.Error()})
}

func conflictForRequest(source *Conflict, operation, path string) *Conflict {
	detail := make(map[string]any, len(source.Detail))
	for key, value := range source.Detail {
		detail[key] = value
	}
	return conflict(source.Code, operation, path, source.Hint, detail)
}

func snapshotReadConflict(c *Cortex, operation, path string, err error) *Conflict {
	if errors.Is(err, fs.ErrNotExist) {
		return conflict("not_found", operation, path,
			"check the path; search the cortex to locate the note", nil)
	}
	if c.VCS == "daybook" {
		return snapshotUnavailable(operation, path, err)
	}
	return conflict("read_failed", operation, path, "fix filesystem access and retry",
		map[string]any{"detail": err.Error()})
}

func gitDetail(err error) string {
	var gerr *GitError
	if errors.As(err, &gerr) && strings.TrimSpace(gerr.Stderr) != "" {
		return strings.TrimSpace(gerr.Stderr)
	}
	return err.Error()
}

func requireCleanPublisher(w string) error {
	out, err := git(w, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("publisher status failed: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("publisher clone is dirty; inspect %s before retrying", w)
	}
	return nil
}

func Revision(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
