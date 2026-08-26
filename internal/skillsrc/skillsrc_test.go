package skillsrc_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/exocortex/internal/cli"
	"github.com/misty-step/exocortex/internal/skillsrc"
)

func installViaScript(t *testing.T, destDir string) string {
	t.Helper()
	src, err := skillsrc.SourceFile()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(src), "..", ".."))
	script := filepath.Join(root, "scripts", "install-skill.sh")
	cmd := exec.CommandContext(t.Context(), "sh", script, destDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-skill.sh: %v\n%s", err, out)
	}
	return filepath.Join(destDir, "SKILL.md")
}

func TestInstallScriptRoundTrip(t *testing.T) {
	dest := installViaScript(t, t.TempDir())
	if err := skillsrc.Check(dest); err != nil {
		t.Fatal(err)
	}
	src, err := skillsrc.SourceFile()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("script mutated bytes: dest %d source %d", len(got), len(want))
	}
}

func TestCheckFailsWhenDestEdited(t *testing.T) {
	dest := installViaScript(t, t.TempDir())
	f, err := os.OpenFile(dest, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# edited\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := skillsrc.Check(dest); err == nil {
		t.Fatal("edited dest must fail Check")
	}
}

func TestSkillMatchesHelpContract(t *testing.T) {
	src, err := skillsrc.SourceFile()
	if err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := cli.Main([]string{"help"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("help exit %d stderr %s", code, errb.String())
	}
	help := out.String()
	body := string(skill)
	for _, cmd := range []string{"search", "brief", "note", "put", "get", "sync", "status", "log", "lint"} {
		if !strings.Contains(help, "exocortex "+cmd) {
			t.Errorf("help missing exocortex %s", cmd)
		}
		if !strings.Contains(body, "exocortex "+cmd) {
			t.Errorf("skill missing exocortex %s", cmd)
		}
	}
	if !strings.Contains(help, "omitted mode is hybrid") {
		t.Fatal("help missing omitted-mode hybrid contract")
	}
	if !strings.Contains(body, "omitted mode is hybrid") {
		t.Fatal("skill missing omitted-mode hybrid contract")
	}
	if !strings.Contains(body, "--expects") {
		t.Fatal("skill missing put --expects")
	}
	if !strings.Contains(body, "writers/") {
		t.Fatal("skill missing sole-publisher writers path")
	}
}
