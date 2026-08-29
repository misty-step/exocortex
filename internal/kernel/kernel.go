// Package kernel implements the exocortex memory-kernel operations over
// registered cortices: registry management, path resolution, and the
// read/write/search/lint surface shared by the CLI and MCP faces.
package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	cortexregistry "github.com/misty-step/exocortex/internal/registry"
)

// Cortex is a registered knowledge corpus.
type Cortex = cortexregistry.Cortex

var (
	nameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	validVCS  = map[string]bool{"daybook": true, "caller": true, "none": true}
	profiles  = map[string]bool{"daybook": true, "strict": true}
	dupSuffix = ".tmp-exocortex"
)

// ConfigDir returns ${XDG_CONFIG_HOME:-~/.config}/exocortex.
func ConfigDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "exocortex"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "exocortex"), nil
}

func registryPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cortices.json"), nil
}

// LoadRegistry combines the user registry with directory-scoped registries
// inherited from the current working directory.
func LoadRegistry() ([]Cortex, error) {
	p, err := registryPath()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return cortexregistry.Load(p, cwd)
}

func loadGlobalRegistry() ([]Cortex, error) {
	p, err := registryPath()
	if err != nil {
		return nil, err
	}
	return cortexregistry.LoadFile(p, filepath.Dir(p))
}

// saveRegistry atomically replaces cortices.json via a UNIQUE
// same-directory temp (a shared fixed name collides between concurrent
// writers). Callers must hold the registry lock: this is the write
// half of a load-modify-write transaction.
func saveRegistry(cs []Cortex) error {
	p, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(p), ".cortices-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, p)
}

// Register binds a cortex into the registry.
func Register(name, path, vcs, profile, journalPrefix string) (*Cortex, error) {
	if err := checkRegisterName(name); err != nil {
		return nil, err
	}
	abs, err := absRegisterPath(name, path)
	if err != nil {
		return nil, err
	}
	regLock, lerr := acquireLock("registry")
	if lerr != nil {
		return nil, conflict("registration_failed", "register", name,
			"fix lock-file access and retry", map[string]any{"detail": lerr.Error()})
	}
	defer regLock.release()
	effective, err := LoadRegistry()
	if err != nil {
		return nil, conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": err.Error()})
	}
	global, err := loadGlobalRegistry()
	if err != nil {
		return nil, conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": err.Error()})
	}
	for _, c := range effective {
		if c.Name == name {
			return nil, conflict("duplicate_cortex", "register", name,
				"pick a new name or inspect the existing cortex with get/search",
				map[string]any{"path": c.Path})
		}
	}
	abs, err = statRegisterDir(abs)
	if err != nil {
		return nil, err
	}
	vcs, profile, err = registerPolicy(name, abs, vcs, profile)
	if err != nil {
		return nil, err
	}
	for _, c := range effective {
		if sameRoot(c.Path, abs) {
			return nil, conflict("duplicate_path", "register", abs,
				"pick a new path or use the existing cortex",
				map[string]any{"name": c.Name})
		}
	}
	jp := filepath.ToSlash(filepath.Clean(journalPrefix))
	if jp == "." {
		jp = "journal"
	}
	c := &Cortex{Name: name, Path: abs, VCS: vcs, Profile: profile, JournalPrefix: jp}
	global = append(global, *c)
	sort.Slice(global, func(i, j int) bool { return global[i].Name < global[j].Name })
	if err := saveRegistry(global); err != nil {
		return nil, conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": err.Error()})
	}
	return c, nil
}

func checkRegisterName(name string) error {
	if !nameRe.MatchString(name) {
		return conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": fmt.Sprintf("cortex name %q must match %s", name, nameRe)})
	}
	return nil
}

func absRegisterPath(name, path string) (string, error) {
	if path == "" {
		return "", conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": "path is required"})
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", conflict("registration_failed", "register", path,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": err.Error()})
	}
	return abs, nil
}

func statRegisterDir(abs string) (string, error) {
	st, err := os.Stat(abs)
	if err != nil {
		return "", conflict("registration_failed", "register", abs,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": fmt.Sprintf("cortex path %s: %v", abs, err)})
	}
	if !st.IsDir() {
		return "", conflict("registration_failed", "register", abs,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": fmt.Sprintf("cortex path %s is not a directory", abs)})
	}
	return canon(abs), nil
}

func registerPolicy(name, abs, vcs, profile string) (string, string, error) {
	if vcs == "" {
		vcs = "none"
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			vcs = "daybook"
		}
	}
	if !validVCS[vcs] {
		return "", "", conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": fmt.Sprintf("vcs %q must be daybook, caller, or none", vcs)})
	}
	if profile == "" {
		profile = "daybook"
	}
	if !profiles[profile] {
		return "", "", conflict("registration_failed", "register", name,
			"fix the name (lowercase slug), path, vcs, or profile and retry",
			map[string]any{"detail": fmt.Sprintf("profile %q must be daybook or strict", profile)})
	}
	return vcs, profile, nil
}

// Resolve maps a user-supplied path onto a cortex and a cortex-relative
// destination. An explicit cortex name wins; otherwise the longest
// registered root containing the (cwd-resolved) path wins; otherwise a
// sole registered cortex interprets the path relative to itself.
// After selection, relUnderRoot is the only destination rule.
func Resolve(cs []Cortex, nameFlag, p string) (*Cortex, string, error) {
	if p == "" {
		return nil, "", errors.New("path is required")
	}
	if nameFlag != "" {
		return resolveNamed(cs, nameFlag, p)
	}
	if c, rel, err, ok := resolveLongestRoot(cs, p); ok {
		return c, rel, err
	}
	return resolveSoleOrAmbiguous(cs, p)
}

func resolveNamed(cs []Cortex, nameFlag, p string) (*Cortex, string, error) {
	for i := range cs {
		if cs[i].Name == nameFlag {
			rel, err := relUnderRoot(cs[i].Path, p)
			if err != nil {
				return nil, "", err
			}
			return &cs[i], rel, nil
		}
	}
	return nil, "", fmt.Errorf("no cortex named %q is registered", nameFlag)
}

func resolveLongestRoot(cs []Cortex, p string) (*Cortex, string, error, bool) {
	abs := p
	if !filepath.IsAbs(abs) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, "", err, true
		}
		abs = filepath.Join(wd, p)
	}
	abs = canon(abs)
	best := -1
	for i := range cs {
		root := canon(cs[i].Path)
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			if best < 0 || len(root) > len(canon(cs[best].Path)) {
				best = i
			}
		}
	}
	if best < 0 {
		return nil, "", nil, false
	}
	rel, err := relUnderRoot(cs[best].Path, abs)
	if err != nil {
		return nil, "", err, true
	}
	return &cs[best], rel, nil, true
}

func resolveSoleOrAmbiguous(cs []Cortex, p string) (*Cortex, string, error) {
	switch len(cs) {
	case 0:
		return nil, "", errors.New("no cortices are registered; run: exocortex register <name> <path>")
	case 1:
		rel, err := relUnderRoot(cs[0].Path, p)
		if err != nil {
			return nil, "", err
		}
		return &cs[0], rel, nil
	default:
		names := make([]string, len(cs))
		for i := range cs {
			names[i] = cs[i].Name
		}
		return nil, "", fmt.Errorf("path %s does not fall under any registered cortex (%s); pass --cortex",
			p, strings.Join(names, ", "))
	}
}

// relUnderRoot maps p onto a destination under root. Absolute paths
// inside root Rel the same way implicit longest-prefix Resolve does.
// ".", "..", and paths outside root fail.
func relUnderRoot(root, p string) (string, error) {
	var rel string
	if filepath.IsAbs(p) {
		var err error
		rel, err = filepath.Rel(canon(root), canon(p))
		if err != nil {
			return "", err
		}
	} else {
		rel = filepath.Clean(p)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes cortex root %s", p, root)
	}
	return rel, nil
}

// canon returns p with existing symlink parents evaluated. A missing
// final element keeps its base name under the canonical parent.
func canon(p string) string {
	p = filepath.Clean(p)
	if e, err := filepath.EvalSymlinks(p); err == nil {
		return e
	}
	dir := filepath.Dir(p)
	if dir == p {
		return p
	}
	return filepath.Join(canon(dir), filepath.Base(p))
}

// GitError carries git's stderr so conflicts can quote real diagnostics.
type GitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed: %s", strings.Join(e.Args, " "), strings.TrimSpace(e.Stderr))
}

// git runs git in dir and returns stdout.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", &GitError{Args: args, Stderr: stderr.String(), Err: err}
	}
	return string(out), nil
}

// ResolveCortex selects a cortex without a path (whole-cortex
// operations): an explicit name wins, a sole registered cortex is
// implicit, anything else is ambiguous.
func ResolveCortex(cs []Cortex, nameFlag string) (*Cortex, error) {
	if nameFlag != "" {
		for i := range cs {
			if cs[i].Name == nameFlag {
				return &cs[i], nil
			}
		}
		return nil, fmt.Errorf("no cortex named %q is registered", nameFlag)
	}
	switch len(cs) {
	case 0:
		return nil, errors.New("no cortices are registered; run: exocortex register <name> <path>")
	case 1:
		return &cs[0], nil
	default:
		names := make([]string, len(cs))
		for i := range cs {
			names[i] = cs[i].Name
		}
		return nil, fmt.Errorf("multiple cortices registered (%s); pass --cortex", strings.Join(names, ", "))
	}
}
