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
	c, err := Register("daybook", sub, "", "", "")
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
	if _, err := Register("daybook", root, "", "", ""); err == nil {
		t.Fatal("duplicate name must fail")
	}
	if _, err := Register("other", sub, "caller", "", ""); err == nil {
		t.Fatal("duplicate path must fail")
	}
	if _, err := Register("Bad_Name", root, "", "", ""); err == nil {
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

func TestSetProfileCAS(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	c, err := Register("box", dir, "none", "daybook", "journal")
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != "daybook" || c.JournalPrefix != "journal" || c.VCS != "none" {
		t.Fatalf("seed: %+v", c)
	}
	got, err := SetProfile("box", "okf", "daybook")
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "okf" || got.JournalPrefix != "journal" || got.VCS != "none" {
		t.Fatalf("cutover mutated extra fields: %+v", got)
	}
	cs, err := LoadRegistry()
	if err != nil || len(cs) != 1 || cs[0].Profile != "okf" {
		t.Fatalf("readback: %v %+v", err, cs)
	}
	_, err = SetProfile("box", "daybook", "daybook")
	conf, ok := err.(*Conflict)
	if !ok || conf.Code != "profile_conflict" {
		t.Fatalf("stale expects: %v (%T)", err, err)
	}
	got, err = SetProfile("box", "daybook", "okf")
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "daybook" {
		t.Fatalf("rollback: %+v", got)
	}
	cs, err = LoadRegistry()
	if err != nil || cs[0].Profile != "daybook" {
		t.Fatalf("rollback readback: %v %+v", err, cs)
	}
}

func TestSetProfileDoesNotPromoteScopedCortex(t *testing.T) {
	config := testConfigEnv(t)
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	localDir := filepath.Join(workspace, ".exocortex")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `[{"name":"local","path":"../root","vcs":"none","profile":"daybook"}]`
	if err := os.WriteFile(filepath.Join(localDir, "cortices.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	_, err = SetProfile("local", "okf", "daybook")
	conf, ok := err.(*Conflict)
	if !ok || conf.Code != "not_found" {
		t.Fatalf("scoped profile mutation = %v (%T), want not_found", err, err)
	}
	if _, err := os.Stat(filepath.Join(config, "exocortex", "cortices.json")); !os.IsNotExist(err) {
		t.Fatalf("scoped cortex was copied into global registry: %v", err)
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

func TestResolveAbsoluteInRootMatchesImplicit(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	cs := []Cortex{{Name: "box", Path: root, VCS: "none", Profile: "daybook"}}
	abs := filepath.Join(root, "notes", "x.md")

	named, namedRel, err := Resolve(cs, "box", abs)
	if err != nil {
		t.Fatalf("explicit abs-in-root: %v", err)
	}
	implied, impliedRel, err := Resolve(cs, "", abs)
	if err != nil {
		t.Fatalf("implicit abs-in-root: %v", err)
	}
	if named.Name != "box" || implied.Name != "box" {
		t.Fatalf("cortex: named=%s implied=%s", named.Name, implied.Name)
	}
	if namedRel != "notes/x.md" && namedRel != filepath.Join("notes", "x.md") {
		t.Fatalf("named rel=%q", namedRel)
	}
	if namedRel != impliedRel {
		t.Fatalf("explicit rel %q != implicit rel %q", namedRel, impliedRel)
	}
}

func TestResolveRejectsEscapes(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	sibling := root + "-evil"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{{Name: "box", Path: root, VCS: "none", Profile: "daybook"}}
	evil := filepath.Join(sibling, "x.md")

	for _, tc := range []struct {
		name, p string
	}{
		{"box", "../escape.md"},
		{"box", evil},
		{"", evil},
		{"box", root},
		{"", root},
	} {
		if _, _, err := Resolve(cs, tc.name, tc.p); err == nil {
			t.Fatalf("expected escape for cortex=%q path=%s", tc.name, tc.p)
		}
	}
}

func TestRegisterDuplicateConflictsAreTyped(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	other := t.TempDir()
	if _, err := Register("box", root, "none", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err := Register("box", other, "none", "", "")
	conf, ok := err.(*Conflict)
	if !ok || conf.Code != "duplicate_cortex" {
		t.Fatalf("name collision: %v (%T)", err, err)
	}
	if conf.Operation != "register" {
		t.Fatalf("operation=%q", conf.Operation)
	}
	_, err = Register("other", root, "none", "", "")
	conf, ok = err.(*Conflict)
	if !ok || conf.Code != "duplicate_path" {
		t.Fatalf("path collision: %v (%T)", err, err)
	}
	_, err = Register("Bad_Name", other, "none", "", "")
	conf, ok = err.(*Conflict)
	if !ok || conf.Code != "registration_failed" {
		t.Fatalf("invalid name: %v (%T)", err, err)
	}
	_, err = Register("box", filepath.Join(other, "missing"), "none", "", "")
	conf, ok = err.(*Conflict)
	if !ok || conf.Code != "duplicate_cortex" {
		t.Fatalf("taken name with missing path: %v (%T)", err, err)
	}

}

func TestRegisterRejectsSymlinkAliasOfExistingRoot(t *testing.T) {
	testConfigEnv(t)
	root := t.TempDir()
	if _, err := Register("box", root, "none", "", ""); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	_, err := Register("other", alias, "none", "", "")
	conf, ok := err.(*Conflict)
	if !ok || conf.Code != "duplicate_path" {
		t.Fatalf("symlink alias: %v (%T)", err, err)
	}
	if conf.Detail["name"] != "box" {
		t.Fatalf("existing name=%v", conf.Detail["name"])
	}
	cs, err := LoadRegistry()
	if err != nil || len(cs) != 1 || cs[0].Name != "box" {
		t.Fatalf("registry=%v err=%v", cs, err)
	}
}

func TestRegisterStoresCanonicalRootForResolve(t *testing.T) {
	testConfigEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	c, err := Register("box", alias, "none", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != root {
		t.Fatalf("stored path=%s want %s", c.Path, root)
	}
	cs := []Cortex{*c}
	note := filepath.Join(root, "n.md")
	named, rel, err := Resolve(cs, "box", note)
	if err != nil || named.Name != "box" || rel != "n.md" {
		t.Fatalf("explicit real path: %v %q %v", named, rel, err)
	}
	implied, rel, err := Resolve(cs, "", note)
	if err != nil || implied.Name != "box" || rel != "n.md" {
		t.Fatalf("implicit real path: %v %q %v", implied, rel, err)
	}
	viaAlias := filepath.Join(alias, "n.md")
	_, rel, err = Resolve(cs, "box", viaAlias)
	if err != nil || rel != "n.md" {
		t.Fatalf("explicit alias path: %q %v", rel, err)
	}
}

func TestRegisterSameNameRaceLeavesOneEntry(t *testing.T) {
	testConfigEnv(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	start := make(chan struct{})
	type outcome struct {
		c   *Cortex
		err error
	}
	out := make(chan outcome, 2)
	for _, dir := range []string{dirA, dirB} {
		go func(path string) {
			<-start
			c, err := Register("racer", path, "none", "", "")
			out <- outcome{c, err}
		}(dir)
	}
	close(start)
	var won, lost int
	for range 2 {
		o := <-out
		if o.err == nil {
			won++
			continue
		}
		lost++
		conf, ok := o.err.(*Conflict)
		if !ok || conf.Code != "duplicate_cortex" {
			t.Fatalf("loser: %v (%T)", o.err, o.err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("won=%d lost=%d", won, lost)
	}
	cs, err := LoadRegistry()
	if err != nil || len(cs) != 1 || cs[0].Name != "racer" {
		t.Fatalf("registry=%v err=%v", cs, err)
	}
}
