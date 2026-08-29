// Package registry loads user and directory-scoped cortex declarations.
package registry

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// Identity is the stable key for a cortex's persistent operational state.
func (c Cortex) Identity() string {
	root := filepath.Clean(c.Path)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	sum := sha256.Sum256([]byte(root))
	return fmt.Sprintf("%s-%x", c.Name, sum[:8])
}

// Load combines the user registry with directory-scoped registries that apply
// to cwd. Deeper scopes replace same-named entries.
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

// LoadFile reads one registry. Relative cortex paths resolve from base.
func LoadFile(path, base string) ([]Cortex, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cs []Cortex
	if err := json.Unmarshal(raw, &cs); err != nil {
		return nil, fmt.Errorf("registry %s is not valid JSON: %w", path, err)
	}
	seen := make(map[string]struct{}, len(cs))
	for i := range cs {
		if _, ok := seen[cs[i].Name]; ok {
			return nil, fmt.Errorf("registry %s repeats cortex name %q", path, cs[i].Name)
		}
		seen[cs[i].Name] = struct{}{}
		if cs[i].Path != "" && !filepath.IsAbs(cs[i].Path) {
			cs[i].Path = filepath.Clean(filepath.Join(base, cs[i].Path))
		}
	}
	return cs, nil
}

func merge(base, overlay []Cortex, _ string) ([]Cortex, error) {
	index := make(map[string]int, len(base)+len(overlay))
	for i := range base {
		index[base[i].Name] = i
	}
	for _, c := range overlay {
		if i, ok := index[c.Name]; ok {
			base[i] = c
			continue
		}
		index[c.Name] = len(base)
		base = append(base, c)
	}
	sort.Slice(base, func(i, j int) bool { return base[i].Name < base[j].Name })
	return base, nil
}
