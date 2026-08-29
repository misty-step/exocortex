package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/misty-step/exocortex/internal/kernel"
)

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
