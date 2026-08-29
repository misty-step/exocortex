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
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestLoadRejectsSameNamedCortexAcrossScopes(t *testing.T) {
	base := t.TempDir()
	userPath := filepath.Join(base, "config", "cortices.json")
	workspace := filepath.Join(base, "r90")
	project := filepath.Join(workspace, "project")
	if err := os.MkdirAll(filepath.Join(workspace, "root"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "project-root"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRegistry(t, filepath.Join(workspace, ".exocortex", "cortices.json"), `[
  {"name":"root","path":"root","vcs":"none","profile":"daybook"}
]`)
	writeRegistry(t, filepath.Join(project, ".exocortex", "cortices.json"), `[
  {"name":"root","path":"project-root","vcs":"none","profile":"strict"}
]`)

	_, err := Load(userPath, project)
	if err == nil || !strings.Contains(err.Error(), `conflicts on cortex name "root"`) {
		t.Fatalf("same-name scope conflict=%v", err)
	}
}

func TestLoadFileRejectsRepeatedNames(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "cortices.json")
	writeRegistry(t, configPath, `[
  {"name":"root","path":"a"},
  {"name":"root","path":"b"}
]`)

	_, err := LoadFile(configPath, base)
	if err == nil || !strings.Contains(err.Error(), `repeats cortex name "root"`) {
		t.Fatalf("same-file duplicate=%v", err)
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

func TestLoadFileRejectsInvalidEntries(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"unknown-field", `[{"name":"root","path":"` + root + `","vcs":"none","profile":"daybook","extra":true}]`, `unknown field "extra"`},
		{"invalid-name", `[{"name":"Root","path":"` + root + `","vcs":"none","profile":"daybook"}]`, "must match"},
		{"invalid-vcs", `[{"name":"root","path":"` + root + `","vcs":"daybok","profile":"daybook"}]`, `vcs "daybok"`},
		{"invalid-profile", `[{"name":"root","path":"` + root + `","vcs":"none","profile":"loose"}]`, `profile "loose"`},
		{"missing-path", `[{"name":"root","vcs":"none","profile":"daybook"}]`, "path is required"},
		{"null-registry", `null`, "must contain a JSON array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(base, tt.name, "cortices.json")
			writeRegistry(t, configPath, tt.raw)
			_, err := LoadFile(configPath, base)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadFile error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadFileAppliesRegistrationDefaults(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(base, "cortices.json")
	writeRegistry(t, configPath, `[{"name":"root","path":"root"}]`)

	cs, err := LoadFile(configPath, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Path != root || cs[0].VCS != "daybook" || cs[0].Profile != "daybook" {
		t.Fatalf("normalized registry=%v", cs)
	}
}

func TestLoadFileRejectsRepeatedCanonicalRoots(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "root-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(base, "cortices.json")
	writeRegistry(t, configPath, `[
  {"name":"first","path":"root","vcs":"none","profile":"daybook"},
  {"name":"second","path":"root-alias","vcs":"none","profile":"daybook"}
]`)

	_, err := LoadFile(configPath, base)
	if err == nil || !strings.Contains(err.Error(), "repeats cortex path") {
		t.Fatalf("same-file canonical root error=%v", err)
	}
}

func TestLoadRejectsSameCanonicalRootAcrossScopes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	userPath := filepath.Join(base, "config", "cortices.json")
	writeRegistry(t, userPath, `[{"name":"global","path":"`+root+`","vcs":"none","profile":"daybook"}]`)

	workspace := filepath.Join(base, "r90")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(workspace, "root-alias")); err != nil {
		t.Fatal(err)
	}
	writeRegistry(t, filepath.Join(workspace, ".exocortex", "cortices.json"), `[
  {"name":"local","path":"root-alias","vcs":"none","profile":"daybook"}
]`)

	_, err := Load(userPath, workspace)
	if err == nil || !strings.Contains(err.Error(), "conflicts on cortex path") {
		t.Fatalf("same canonical root error=%v", err)
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
