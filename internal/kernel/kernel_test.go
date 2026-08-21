package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testConfigEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("EXOCORTEX_AGENT", "test-agent")
	return dir
}

func TestRegisterAndLoad(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()

	if cs, err := LoadRegistry(); err != nil || len(cs) != 0 {
		t.Fatalf("empty registry: %v %v", cs, err)
	}

	sub := filepath.Join(root, "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := Register("daybook", sub, "", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if c.VCS != "none" { // no .git yet -> auto none
		t.Fatalf("auto-detect vcs = %q, want none", c.VCS)
	}
	if _, err := os.Stat(filepath.Join(sub, ".git")); err == nil {
		t.Fatal("unexpected .git")
	}
	// Duplicate name and duplicate path both rejected.
	if _, err := Register("daybook", root, "", ""); err == nil {
		t.Fatal("duplicate name must fail")
	}
	if _, err := Register("other", sub, "caller", ""); err == nil {
		t.Fatal("duplicate path must fail")
	}
	if _, err := Register("Bad_Name", root, "", ""); err == nil {
		t.Fatal("invalid slug must fail")
	}
	cs, err := LoadRegistry()
	if err != nil || len(cs) != 1 || cs[0].Name != "daybook" {
		t.Fatalf("roundtrip: %v %v", cs, err)
	}
	if !filepath.IsAbs(cs[0].Path) {
		t.Fatal("registered path must be absolute")
	}
}

func TestResolve(t *testing.T) {
	testConfigEnv(t)
	rootA := t.TempDir()
	rootB := t.TempDir()
	cs := []Cortex{
		{Name: "aaa", Path: rootA, VCS: "daybook", Profile: "daybook"},
		{Name: "bbb", Path: rootB, VCS: "none", Profile: "strict"},
	}

	// Explicit name wins; relative paths jail-checked against the root.
	c, rel, err := Resolve(cs, "bbb", "notes/x.md")
	if err != nil || c.Name != "bbb" || rel != "notes/x.md" {
		t.Fatalf("named resolve: %v %v %v", c, rel, err)
	}
	if _, _, err := Resolve(cs, "bbb", "../escape.md"); err == nil {
		t.Fatal("path escaping cortex root must be rejected")
	}
	if _, _, err := Resolve(cs, "nope", "x.md"); err == nil {
		t.Fatal("unknown cortex must be rejected")
	}

	// Longest-prefix match over absolute paths.
	nested := filepath.Join(rootB, "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cs = append(cs, Cortex{Name: "ccc", Path: nested, VCS: "none", Profile: "daybook"})
	c, rel, err = Resolve(cs, "", filepath.Join(nested, "y.md"))
	if err != nil || c.Name != "ccc" || rel != "y.md" {
		t.Fatalf("prefix resolve: %v %v %v", c.Name, rel, err)
	}

	// Ambiguous bare relative path with multiple cortices.
	if _, _, err := Resolve(cs, "", "somewhere.md"); err == nil || !strings.Contains(err.Error(), "--cortex") {
		t.Fatalf("ambiguous path must demand --cortex: %v", err)
	}

	// Sole cortex interprets bare relative paths.
	one := []Cortex{cs[0]}
	c, rel, err = Resolve(one, "", "deep/note.md")
	if err != nil || c.Name != "aaa" || rel != "deep/note.md" {
		t.Fatalf("sole-cortex fallback: %v %v %v", c, rel, err)
	}
}

func TestResolveCortex(t *testing.T) {
	a := Cortex{Name: "a"}
	if _, err := ResolveCortex(nil, ""); err == nil {
		t.Fatal("empty registry must fail")
	}
	if _, err := ResolveCortex([]Cortex{a}, ""); err != nil {
		t.Fatalf("sole cortex implicit: %v", err)
	}
	two := []Cortex{a, {Name: "b"}}
	if _, err := ResolveCortex(two, ""); err == nil {
		t.Fatal("ambiguous registry must fail without a name")
	}
	if c, err := ResolveCortex(two, "b"); err != nil || c.Name != "b" {
		t.Fatalf("named selection: %v %v", c, err)
	}
}
