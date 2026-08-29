package registry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/exocortex/internal/kernel"
)

func TestRegisterDoesNotPersistInheritedCortex(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	workspace := t.TempDir()
	localConfigDir := filepath.Join(workspace, ".exocortex")
	if err := os.MkdirAll(localConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "root"), 0o755); err != nil {
		t.Fatal(err)
	}
	localConfig := `[{"name":"root","path":"root","vcs":"none","profile":"daybook"}]`
	if err := os.WriteFile(filepath.Join(localConfigDir, "cortices.json"), []byte(localConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	if _, err := kernel.Register("other", t.TempDir(), "none", "daybook", "journal"); err != nil {
		t.Fatal(err)
	}
	inside, err := kernel.LoadRegistry()
	if err != nil || len(inside) != 2 {
		t.Fatalf("inside registry=%v err=%v", inside, err)
	}

	t.Chdir(t.TempDir())
	outside, err := kernel.LoadRegistry()
	if err != nil || len(outside) != 1 || outside[0].Name != "other" {
		t.Fatalf("outside registry=%v err=%v", outside, err)
	}
}
