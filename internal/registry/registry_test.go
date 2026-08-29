package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadScopesLocalCorticesToWorkingTree(t *testing.T) {
	base := t.TempDir()
	userPath := filepath.Join(base, "config", "exocortex", "cortices.json")
	globalRoot := filepath.Join(base, "daybook")
	writeRegistry(t, userPath, `[
  {"name":"daybook","path":"`+globalRoot+`","vcs":"daybook","profile":"daybook"}
]`)

	workspace := filepath.Join(base, "r90")
	localRoot := filepath.Join(workspace, "root")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRegistry(t, filepath.Join(workspace, ".exocortex", "cortices.json"), `[
  {"name":"root","path":"root","vcs":"daybook","profile":"daybook"}
]`)

	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	cs, err := Load(userPath, outside)
	if err != nil || len(cs) != 1 || cs[0].Name != "daybook" {
		t.Fatalf("outside registry=%v err=%v", cs, err)
	}

	cs, err = Load(userPath, workspace)
	if err != nil || len(cs) != 2 || cs[1].Name != "root" {
		t.Fatalf("workspace registry=%v err=%v", cs, err)
	}
	if cs[1].Path != localRoot {
		t.Fatalf("relative local path=%q want %q", cs[1].Path, localRoot)
	}

	nested := filepath.Join(workspace, "project", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cs, err = Load(userPath, nested)
	if err != nil || len(cs) != 2 || cs[1].Name != "root" {
		t.Fatalf("nested registry=%v err=%v", cs, err)
	}
}

func TestLoadUsesDeepestSameNamedCortex(t *testing.T) {
	base := t.TempDir()
	userPath := filepath.Join(base, "config", "cortices.json")
	workspace := filepath.Join(base, "r90")
	project := filepath.Join(workspace, "project")
	writeRegistry(t, filepath.Join(workspace, ".exocortex", "cortices.json"), `[
  {"name":"root","path":"root","vcs":"none","profile":"daybook"}
]`)
	writeRegistry(t, filepath.Join(project, ".exocortex", "cortices.json"), `[
  {"name":"root","path":"project-root","vcs":"none","profile":"strict"}
]`)

	cs, err := Load(userPath, project)
	if err != nil || len(cs) != 1 {
		t.Fatalf("registry=%v err=%v", cs, err)
	}
	want := filepath.Join(project, "project-root")
	if cs[0].Path != want || cs[0].Profile != "strict" {
		t.Fatalf("deep override=%+v want path %q profile strict", cs[0], want)
	}
}

func TestLoadFailsClosedOnMalformedLocalConfig(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, ".exocortex", "cortices.json")
	writeRegistry(t, configPath, "{")

	_, err := Load(filepath.Join(base, "user.json"), base)
	if err == nil || !strings.Contains(err.Error(), configPath) {
		t.Fatalf("malformed local config error=%v", err)
	}
}

func writeRegistry(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
