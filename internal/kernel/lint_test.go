package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLintReservedIndexAndLog(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "index.md", "# Root\n\n* [Tools](tools/index.md) - dossiers\n")
	writeFile(t, dir, "log.md", "## 2026-08-29\n* **Init** | bootstrapped.\n")
	writeFile(t, dir, "tools/index.md", "# Tools\n\n* [OKF](../systems/okf.md) - floor\n")
	writeFile(t, dir, "systems/okf.md", "---\ntype: Concept\n---\nbody\n")

	c, err := Register("box", dir, "none", "okf", "")
	if err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{*c}
	res, conf := Lint(cs, "box", "")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if res.Errors != 0 {
		t.Fatalf("clean reserved vault: %+v", res.Findings)
	}

	writeFile(t, dir, "AGENTS.md", "# law\n")
	res, conf = Lint(cs, "box", "")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if res.Errors == 0 {
		t.Fatal("AGENTS.md is not reserved; missing type must still fail")
	}
}

func TestLintRejectsMalformedReservedFiles(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "index.md", "anything")
	c, err := Register("box", dir, "none", "okf", "")
	if err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{*c}
	res, conf := Lint(cs, "box", "index.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if res.Errors == 0 {
		t.Fatal("malformed index.md must fail lint")
	}

	writeFile(t, dir, "log.md", "## Today\n")
	res, conf = Lint(cs, "box", "log.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if res.Errors == 0 {
		t.Fatal("non-ISO log heading must fail lint")
	}
}

func TestLintDaybookMocIndexStillANote(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "journal/2026/index.md", "---\ntype: moc\n---\n\n```dataview\nLIST\n```\n")
	c, err := Register("day", dir, "none", "daybook", "")
	if err != nil {
		t.Fatal(err)
	}
	res, conf := Lint([]Cortex{*c}, "day", "journal/2026/index.md")
	if conf != nil {
		t.Fatal(conf.Code)
	}
	if res.Errors != 0 {
		t.Fatalf("daybook moc index must remain a type-floor note: %+v", res.Findings)
	}
}
