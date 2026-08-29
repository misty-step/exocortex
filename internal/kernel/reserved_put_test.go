package kernel

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPutOkfReservedLeavesNoFrontmatter(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	c, err := Register("okfbox", dir, "none", "okf", "")
	if err != nil {
		t.Fatal(err)
	}
	cs := []Cortex{*c}
	catalog := "# Tools\n\n* [OKF](okf.md) - floor\n"
	_, conf := Put(nil, cs, PutInput{
		CortexName: "okfbox", Path: "tools/index.md", Payload: []byte(catalog),
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("put catalog: %s %v", conf.Code, conf.Detail)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tools/index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte(catalog)) {
		t.Fatalf("nested index bytes changed:\n%s", raw)
	}
	if bytes.HasPrefix(raw, []byte("---\n")) {
		t.Fatal("nested index.md must have no frontmatter")
	}

	logBody := "## 2026-08-29\n* **Init** | bootstrapped.\n"
	_, conf = Put(nil, cs, PutInput{
		CortexName: "okfbox", Path: "log.md", Payload: []byte(logBody),
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("put log: %s %v", conf.Code, conf.Detail)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte(logBody)) {
		t.Fatalf("log.md bytes changed:\n%s", raw)
	}

	note := "---\ntype: Concept\n---\nbody\n"
	_, conf = Put(nil, cs, PutInput{
		CortexName: "okfbox", Path: "tools/x.md", Payload: []byte(note),
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf != nil {
		t.Fatalf("put note: %s", conf.Code)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "tools/x.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("provenance:")) {
		t.Fatal("ordinary notes still get a provenance stamp")
	}

	_, conf = Put(nil, cs, PutInput{
		CortexName: "okfbox", Path: "tools/other.md", Payload: []byte(catalog),
		Agent: "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf == nil || conf.Code != "invalid_note" {
		t.Fatalf("non-reserved path still needs type, got %#v", conf)
	}
}

func TestPutOkfRejectsTypedIndex(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	c, err := Register("okfbox", dir, "none", "okf", "")
	if err != nil {
		t.Fatal(err)
	}
	_, conf := Put(nil, []Cortex{*c}, PutInput{
		CortexName: "okfbox", Path: "tools/index.md",
		Payload: []byte("---\ntype: moc\n---\n# Tools\n\n* [OKF](okf.md) - floor\n"),
		Agent:   "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf == nil || conf.Code != "invalid_note" {
		t.Fatalf("typed nested index must fail put, got %#v", conf)
	}
}

func TestPutDaybookIndexStillRequiresType(t *testing.T) {
	testConfigEnv(t)
	dir := t.TempDir()
	c, err := Register("day", dir, "none", "daybook", "")
	if err != nil {
		t.Fatal(err)
	}
	_, conf := Put(nil, []Cortex{*c}, PutInput{
		CortexName: "day", Path: "journal/2026/index.md",
		Payload: []byte("# Year\n"),
		Agent:   "test-agent", Via: "cli", OwnPayload: true,
	})
	if conf == nil || conf.Code != "invalid_note" {
		t.Fatalf("daybook index without type must fail, got %#v", conf)
	}
}
