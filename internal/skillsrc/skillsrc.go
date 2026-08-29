// Package skillsrc locates the bundled Exocortex skill and compares
// generated copies to it. The only copier is scripts/install-skill.sh.
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
