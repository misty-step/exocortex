// Package skillsrc owns the bundled Exocortex skill file: this repository
// copy is source; omp-config and live installs are generated from it.
package skillsrc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const relSource = "skills/exocortex/SKILL.md"

// SourceFile returns the repository skill path.
func SourceFile() (string, error) {
	_, this, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("skill source: cannot locate package file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(this), "..", ".."))
	p := filepath.Join(root, filepath.FromSlash(relSource))
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("skill source %s: %w", p, err)
	}
	return p, nil
}

// Install writes the source skill to dest (a SKILL.md path or a directory).
func Install(dest string) error {
	src, err := SourceFile()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := dest
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		out = filepath.Join(dest, "SKILL.md")
	} else if filepath.Ext(dest) == "" {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		out = filepath.Join(dest, "SKILL.md")
	} else if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, body, 0o644)
}

// Check reports whether dest is byte-identical to the repository source.
func Check(dest string) error {
	src, err := SourceFile()
	if err != nil {
		return err
	}
	want, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		return fmt.Errorf("skill dest %s: %w", dest, err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("skill dest %s differs from source %s (%d bytes dest, %d bytes source)", dest, src, len(got), len(want))
	}
	return nil
}
