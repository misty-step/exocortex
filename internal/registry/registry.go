// Package registry loads user and directory-scoped cortex declarations.
package registry

import (
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

func merge(base, overlay []Cortex, source string) ([]Cortex, error) {
	names := make(map[string]struct{}, len(base)+len(overlay))
	for i := range base {
		names[base[i].Name] = struct{}{}
	}
	for _, c := range overlay {
		if _, ok := names[c.Name]; ok {
			return nil, fmt.Errorf("registry %s conflicts on cortex name %q", source, c.Name)
		}
		names[c.Name] = struct{}{}
		base = append(base, c)
	}
	sort.Slice(base, func(i, j int) bool { return base[i].Name < base[j].Name })
	return base, nil
}
