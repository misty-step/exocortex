package registry_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestRegisterRejectsHiddenGlobalRootAlias(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalRoot := t.TempDir()
	userConfigDir := filepath.Join(configHome, "exocortex")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userConfig := `[{"name":"root","path":"` + globalRoot + `","vcs":"none","profile":"daybook"}]`
	if err := os.WriteFile(filepath.Join(userConfigDir, "cortices.json"), []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	localRoot := filepath.Join(workspace, "local-root")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	localConfigDir := filepath.Join(workspace, ".exocortex")
	if err := os.MkdirAll(localConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localConfig := `[{"name":"root","path":"local-root","vcs":"none","profile":"daybook"}]`
	if err := os.WriteFile(filepath.Join(localConfigDir, "cortices.json"), []byte(localConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	_, err := kernel.Register("alias", globalRoot, "none", "daybook", "journal")
	if err == nil || !strings.Contains(err.Error(), "duplicate_path") {
		t.Fatalf("hidden global root alias error=%v", err)
	}
}

func TestSameNamedScopedCorticesUseSeparateOperationalState(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	base := t.TempDir()
	seed := func(name string) kernel.Cortex {
		origin := filepath.Join(base, name+".git")
		clone := filepath.Join(base, name)
		gitRun(t, base, "init", "--bare", "-b", "master", origin)
		gitRun(t, base, "clone", origin, clone)
		gitRun(t, clone, "config", "user.email", name+"@test")
		gitRun(t, clone, "config", "user.name", name)
		if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, clone, "add", "README.md")
		gitRun(t, clone, "commit", "-m", "seed")
		gitRun(t, clone, "push", "-u", "origin", "master")
		return kernel.Cortex{Name: "root", Path: clone, VCS: "daybook", Profile: "daybook"}
	}

	for _, c := range []kernel.Cortex{seed("first"), seed("second")} {
		_, conf := kernel.Put(context.Background(), []kernel.Cortex{c}, kernel.PutInput{
			CortexName: "root",
			Path:       "notes/scoped.md",
			Payload:    []byte("---\ntype: note\n---\n\nscoped\n"),
			Agent:      "test-agent",
			Via:        "cli",
			OwnPayload: true,
		})
		if conf != nil {
			t.Fatalf("put %s: %s detail=%v", c.Path, conf.Code, conf.Detail)
		}
	}

	for _, dir := range []string{"writers", "state"} {
		entries, err := os.ReadDir(filepath.Join(configHome, "exocortex", dir))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("%s entries=%d want 2", dir, len(entries))
		}
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
