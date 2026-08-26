// Package skillsrc owns the bundled Exocortex skill file: this repository
// copy is source; omp-config and live installs are generated from it.
package skillsrc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const relSource = "skills/exocortex/SKILL.md"

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("skill source: working directory: %w", err)
	}
	dir := wd
	for {
		mod := filepath.Join(dir, "go.mod")
		src := filepath.Join(dir, filepath.FromSlash(relSource))
		if _, err := os.Stat(mod); err == nil {
			if _, err := os.Stat(src); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("skill source: no checkout containing go.mod and %s above %s", relSource, wd)
		}
		dir = parent
	}
}

// SourceFile returns the repository skill path.
func SourceFile() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(relSource)), nil
}

// Install writes the source skill to dest (a SKILL.md path or a directory).
func Install(dest string) error {
	src, err := SourceFile()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("skill install: read source %s: %w", src, err)
	}
	out := dest
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		out = filepath.Join(dest, "SKILL.md")
	} else if filepath.Ext(dest) == "" {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("skill install: create dest dir %s: %w", dest, err)
		}
		out = filepath.Join(dest, "SKILL.md")
	} else if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("skill install: create dest parent %s: %w", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(out, body, 0o644); err != nil {
		return fmt.Errorf("skill install: write %s: %w", out, err)
	}
	return nil
}

// Check reports whether dest is byte-identical to the repository source.
func Check(dest string) error {
	src, err := SourceFile()
	if err != nil {
		return err
	}
	want, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("skill check: read source %s: %w", src, err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		return fmt.Errorf("skill check: read dest %s: %w", dest, err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("skill dest %s differs from source %s (%d bytes dest, %d bytes source)", dest, src, len(got), len(want))
	}
	return nil
}
