// Package registry loads user and directory-scoped cortex declarations.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Cortex is one registered knowledge corpus.
type Cortex struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	VCS           string `json:"vcs"`
	Profile       string `json:"profile"`
	JournalPrefix string `json:"journal_prefix,omitempty"`
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Load combines the user registry with directory-scoped registries that apply
// to cwd. Duplicate cortex names across scopes fail closed.
func Load(userPath, cwd string) ([]Cortex, error) {
	cs, err := LoadFile(userPath, filepath.Dir(userPath))
	if err != nil {
		return nil, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	var localPaths []string
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		localPaths = append(localPaths, filepath.Join(dir, ".exocortex", "cortices.json"))
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	for i := len(localPaths) - 1; i >= 0; i-- {
		base := filepath.Dir(filepath.Dir(localPaths[i]))
		local, lerr := LoadFile(localPaths[i], base)
		if lerr != nil {
			return nil, lerr
		}
		cs, lerr = merge(cs, local, localPaths[i])
		if lerr != nil {
			return nil, lerr
		}
	}
	return cs, nil
}

// LoadFile reads and validates one registry. Relative cortex paths resolve from base.
func LoadFile(path, base string) ([]Cortex, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cs []Cortex
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cs); err != nil {
		return nil, fmt.Errorf("registry %s is not valid JSON: %w", path, err)
	}
	if cs == nil {
		return nil, fmt.Errorf("registry %s must contain a JSON array", path)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("additional JSON value")
		}
		return nil, fmt.Errorf("registry %s is not valid JSON: %w", path, err)
	}
	names := make(map[string]struct{}, len(cs))
	for i := range cs {
		if _, ok := names[cs[i].Name]; ok {
			return nil, fmt.Errorf("registry %s repeats cortex name %q", path, cs[i].Name)
		}
		names[cs[i].Name] = struct{}{}
	}
	roots := make(map[string]string, len(cs))
	for i := range cs {
		normalized, nerr := Normalize(cs[i], base)
		if nerr != nil {
			return nil, fmt.Errorf("registry %s entry %q: %w", path, cs[i].Name, nerr)
		}
		cs[i] = normalized
		if existing, ok := roots[cs[i].Path]; ok {
			return nil, fmt.Errorf("registry %s repeats cortex path %q for %q and %q", path, cs[i].Path, existing, cs[i].Name)
		}
		roots[cs[i].Path] = cs[i].Name
	}
	return cs, nil
}

// Normalize validates one declaration, resolves its root, and applies defaults.
func Normalize(c Cortex, base string) (Cortex, error) {
	if !nameRE.MatchString(c.Name) {
		return Cortex{}, fmt.Errorf("cortex name %q must match %s", c.Name, nameRE)
	}
	root, err := normalizeRoot(c.Path, base)
	if err != nil {
		return Cortex{}, err
	}
	c.Path = root
	c, err = normalizePolicy(c)
	if err != nil {
		return Cortex{}, err
	}
	c.JournalPrefix = filepath.ToSlash(filepath.Clean(c.JournalPrefix))
	if c.JournalPrefix == "." {
		c.JournalPrefix = "journal"
	}
	return c, nil
}

func normalizeRoot(path, base string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	if !filepath.IsAbs(path) && base != "" {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cortex path %s: %w", abs, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("cortex path %s is not a directory", abs)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize cortex path %s: %w", abs, err)
	}
	return root, nil
}

func normalizePolicy(c Cortex) (Cortex, error) {
	if c.VCS == "" {
		c.VCS = "none"
		if _, err := os.Stat(filepath.Join(c.Path, ".git")); err == nil {
			c.VCS = "daybook"
		}
	}
	switch c.VCS {
	case "daybook", "caller", "none":
	default:
		return Cortex{}, fmt.Errorf("vcs %q must be daybook, caller, or none", c.VCS)
	}
	if c.Profile == "" {
		c.Profile = "daybook"
	}
	switch c.Profile {
	case "daybook", "strict", "okf":
	default:
		return Cortex{}, fmt.Errorf("profile %q must be daybook, strict, or okf", c.Profile)
	}
	return c, nil
}

func merge(base, overlay []Cortex, source string) ([]Cortex, error) {
	names := make(map[string]struct{}, len(base)+len(overlay))
	roots := make(map[string]string, len(base)+len(overlay))
	for i := range base {
		names[base[i].Name] = struct{}{}
		roots[base[i].Path] = base[i].Name
	}
	for _, c := range overlay {
		if _, ok := names[c.Name]; ok {
			return nil, fmt.Errorf("registry %s conflicts on cortex name %q", source, c.Name)
		}
		if existing, ok := roots[c.Path]; ok {
			return nil, fmt.Errorf("registry %s conflicts on cortex path %q for %q and %q", source, c.Path, existing, c.Name)
		}
		names[c.Name] = struct{}{}
		roots[c.Path] = c.Name
		base = append(base, c)
	}
	sort.Slice(base, func(i, j int) bool { return base[i].Name < base[j].Name })
	return base, nil
}
