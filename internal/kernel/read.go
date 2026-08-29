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

// GetResult is the pinned `get` output.
type GetResult struct {
	Cortex      string         `json:"cortex"`
	Path        string         `json:"path"`
	Revision    string         `json:"revision"`
	Frontmatter map[string]any `json:"frontmatter"`
	Content     string         `json:"content"`
}

func Get(cs []Cortex, nameFlag, p string) (*GetResult, *Conflict) {
	c, rel, err := Resolve(cs, nameFlag, p)
	if err != nil {
		return nil, conflict("resolve_failed", "get", p, "use a path inside a registered cortex or pass --cortex", map[string]any{"detail": err.Error()})
	}
	var out *GetResult
	conf := withFreshPublisher(c, "get", rel, func(root string) *Conflict {
		if c.VCS == "daybook" {
			raw, gerr := git(root, "show", "HEAD:"+filepath.ToSlash(rel))
			if gerr != nil {
				return conflict("not_found", "get", rel, "check the path; search the cortex to locate the note", nil)
			}
			out = getResult(c, rel, []byte(raw))
			return nil
		}
		abs := filepath.Join(root, rel)
		raw, serr := os.ReadFile(abs)
		if errors.Is(serr, fs.ErrNotExist) {
			return conflict("not_found", "get", rel, "check the path; search the cortex to locate the note", nil)
		}
		if serr != nil {
			return conflict("read_failed", "get", rel, "fix filesystem access and retry", map[string]any{"detail": serr.Error()})
		}
		out = getResult(c, rel, raw)
		return nil
	})
	return out, conf
}

func getResult(c *Cortex, rel string, raw []byte) *GetResult {
	doc := fm.ParseDocument(raw)
	return &GetResult{
		Cortex:      c.Name,
		Path:        filepath.ToSlash(rel),
		Revision:    Revision(raw),
		Frontmatter: doc.Map,
		Content:     string(raw),
	}
}

// LogEntry is one line of git lineage for a note.
type LogEntry struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"` // RFC3339 in the commit's timezone offset
	Subject string `json:"subject"`
}

// Log returns the git lineage of one note.
func Log(cs []Cortex, nameFlag, p string, limit int) ([]LogEntry, *Conflict) {
	if limit <= 0 {
		limit = 50
	}
	c, rel, err := Resolve(cs, nameFlag, p)
	if err != nil {
		return nil, conflict("resolve_failed", "log", p, "use a path inside a registered cortex or pass --cortex", map[string]any{"detail": err.Error()})
	}
	var entries []LogEntry
	conf := withFreshPublisher(c, "log", rel, func(root string) *Conflict {
		out, gerr := git(root, "log", fmt.Sprintf("-n%d", limit),
			"--format=%H%x1f%an%x1f%aI%x1f%s", "--", filepath.FromSlash(rel))
		if gerr != nil {
			return conflict("log_unavailable", "log", rel,
				"lineage requires a git-backed cortex; check that the cortex path is a repository",
				map[string]any{"detail": gerr.(*GitError).Stderr})
		}
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\x1f", 4)
			if len(parts) != 4 {
				continue
			}
			entries = append(entries, LogEntry{SHA: parts[0], Author: parts[1], Date: parts[2], Subject: parts[3]})
		}
		return nil
	})
	return entries, conf
}

// LintResult reports tiered findings for one note or a whole cortex.
type LintResult struct {
	Cortex   string       `json:"cortex"`
	Path     string       `json:"path,omitempty"`
	Findings []fm.Finding `json:"findings"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
}

// Lint runs the cortex's validation profile over one note or every .md
// file in the cortex. Findings are tiered; only errors block.
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
		c, rel = rc, rrel
	}
	res := &LintResult{Cortex: c.Name, Path: filepath.ToSlash(rel)}
	add := func(path string, findings []fm.Finding, verr error) {
		if verr != nil {
			if f, ok := fm.ContractFinding(verr); ok {
				res.Findings = append(res.Findings, f)
			} else {
				res.Findings = append(res.Findings, fm.Finding{Level: "error", Rule: "read_failed", Message: verr.Error()})
			}
		}
		res.Findings = append(res.Findings, findings...)
	}
	conf := withFreshPublisher(c, "lint", rel, func(root string) *Conflict {
		if rel != "" {
			findings, verr := lintOne(c.Profile, filepath.Join(root, rel))
			add(rel, findings, verr)
			return nil
		}
		werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
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
			relPath, _ := filepath.Rel(root, path)
			findings, verr := lintOne(c.Profile, path)
			add(filepath.ToSlash(relPath), findings, verr)
			return nil
		})
		if werr != nil {
			return conflict("walk_failed", "lint", "", "fix filesystem access and retry", map[string]any{"detail": werr.Error()})
		}
		return nil
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

func lintOne(profile, abs string) ([]fm.Finding, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return fm.Validate(profile, fm.ParseDocument(raw))
}

// withFreshPublisher runs fn against the publisher tree. For daybook it
// takes the per-cortex lock, provisions if needed, fast-forwards to
// origin when upstream exists, and holds the lock for the whole read.
// ensureWriter stays provisioning-only and does not lock. Dirty
// worktrees are left in place; Get/Log use git show HEAD.
func withFreshPublisher(c *Cortex, op, rel string, fn func(root string) *Conflict) *Conflict {
	if c.VCS != "daybook" {
		root, err := effectiveRoot(c)
		if err != nil {
			return conflict("cortex_unavailable", op, rel, "publisher repository is unavailable; check remote access", map[string]any{"detail": err.Error()})
		}
		return fn(root)
	}
	lock, conf := lockNamed(c.Name, op, rel)
	if conf != nil {
		return conf
	}
	defer lock.release()
	root, err := effectiveRoot(c)
	if err != nil {
		return conflict("cortex_unavailable", op, rel, "publisher repository is unavailable; check remote access", map[string]any{"detail": err.Error()})
	}
	if err := refreshPublisher(root); err != nil {
		return conflict("cortex_unavailable", op, rel, "publisher repository is unavailable; check remote access", map[string]any{"detail": err.Error()})
	}
	return fn(root)
}

func refreshPublisher(w string) error {
	if !hasUpstream(w) {
		return nil
	}
	return ffToUpstream(w)
}

// Revision is the lowercase hex sha256 of a note's exact file bytes —
// the pinned revision identity reported by get and required by --expects.
func Revision(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
