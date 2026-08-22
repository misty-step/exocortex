// Package kernel implements the exocortex memory-kernel operations over
// registered cortices: registry management, path resolution, and the
// read/write/search/lint surface shared by the CLI and MCP faces.
package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Cortex is a registered knowledge corpus.
type Cortex struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // absolute filesystem root
	VCS     string `json:"vcs"`  // "daybook" | "caller" | "none"
	Profile string `json:"profile"`
}

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

// LoadRegistry reads cortices.json. A missing file is an empty registry.
func LoadRegistry() ([]Cortex, error) {
	p, err := registryPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cs []Cortex
	if err := json.Unmarshal(raw, &cs); err != nil {
		return nil, fmt.Errorf("registry %s is not valid JSON: %w", p, err)
	}
	return cs, nil
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
func Register(name, path, vcs, profile string) (*Cortex, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("cortex name %q must match %s", name, nameRe)
	}
	if path == "" {
		return nil, errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("cortex path %s: %w", abs, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("cortex path %s is not a directory", abs)
	}
	if vcs == "" {
		vcs = "none"
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			vcs = "daybook"
		}
	}
	if !validVCS[vcs] {
		return nil, fmt.Errorf("vcs %q must be daybook, caller, or none", vcs)
	}
	if profile == "" {
		profile = "daybook"
	}
	if !profiles[profile] {
		return nil, fmt.Errorf("profile %q must be daybook or strict", profile)
	}
	// The whole load/check/save transaction runs under one registry
	// lock: concurrent CLI/MCP registrations must never lose entries.
	regLock, lerr := acquireLock("registry")
	if lerr != nil {
		return nil, lerr
	}
	defer regLock.release()
	cs, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c.Name == name {
			return nil, fmt.Errorf("cortex %q is already registered", name)
		}
		if c.Path == abs {
			return nil, fmt.Errorf("path %s is already registered as cortex %q", abs, c.Name)
		}
	}
	c := &Cortex{Name: name, Path: abs, VCS: vcs, Profile: profile}
	cs = append(cs, *c)
	sort.Slice(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
	if err := saveRegistry(cs); err != nil {
		return nil, err
	}
	return c, nil
}

// Resolve maps a user-supplied path onto a cortex and a cortex-relative
// destination. An explicit cortex name wins; otherwise the longest
// registered root containing the (cwd-resolved) path wins; otherwise a
// sole registered cortex interprets the path relative to itself.
func Resolve(cs []Cortex, nameFlag, p string) (*Cortex, string, error) {
	if p == "" {
		return nil, "", errors.New("path is required")
	}
	if nameFlag != "" {
		for i := range cs {
			if cs[i].Name == nameFlag {
				rel, err := jail(cs[i].Path, p)
				if err != nil {
					return nil, "", err
				}
				return &cs[i], rel, nil
			}
		}
		return nil, "", fmt.Errorf("no cortex named %q is registered", nameFlag)
	}
	abs := p
	if !filepath.IsAbs(abs) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		abs = filepath.Join(wd, p)
	}
	abs = filepath.Clean(abs)
	best := -1
	for i := range cs {
		root := filepath.Clean(cs[i].Path)
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			if best < 0 || len(root) > len(filepath.Clean(cs[best].Path)) {
				best = i
			}
		}
	}
	if best >= 0 {
		rel, err := filepath.Rel(filepath.Clean(cs[best].Path), abs)
		if err != nil {
			return nil, "", err
		}
		return &cs[best], rel, nil
	}
	switch len(cs) {
	case 0:
		return nil, "", errors.New("no cortices are registered; run: exocortex register <name> <path>")
	case 1:
		rel, err := jail(cs[0].Path, p)
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

// jail rejects destinations that escape the cortex root.
func jail(root, p string) (string, error) {
	rel := filepath.Clean(p)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %s escapes cortex root %s", p, root)
	}
	return rel, nil
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
