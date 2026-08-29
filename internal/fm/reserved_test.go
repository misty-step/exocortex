package fm

import (
	"bufio"
	"strings"
	"testing"
)

func TestValidateIndex(t *testing.T) {
	good := []byte("# Tools\n\n* [OKF](okf.md) - floor\n")
	if _, err := ValidateIndex(good, false); err != nil {
		t.Fatalf("nested index without frontmatter must pass: %v", err)
	}
	root := []byte("---\nokf_version: \"0.1\"\n---\n# Root\n\n* [Tools](tools/index.md) - dossiers\n")
	if _, err := ValidateIndex(root, true); err != nil {
		t.Fatalf("root index with okf_version must pass: %v", err)
	}
	if _, err := ValidateIndex([]byte("---\ntype: index\n---\n# Tools\n\n* [OKF](okf.md) - floor\n"), false); err == nil {
		t.Fatal("nested index with frontmatter must fail")
	}
	if _, err := ValidateIndex([]byte("anything"), false); err == nil {
		t.Fatal("index.md without a heading must fail")
	}
	if _, err := ValidateIndex([]byte("# Tools\n"), false); err == nil {
		t.Fatal("index.md without a catalog bullet must fail")
	}
	if _, err := ValidateIndex([]byte("# Tools\n\n* leftover prose\n"), false); err == nil {
		t.Fatal("index.md non-link bullets must fail")
	}
	if _, err := ValidateIndex([]byte("---\nokf_version: \"0.1\"\ntype: index\n---\n# Root\n\n* [Tools](tools/index.md) - dossiers\n"), true); err == nil {
		t.Fatal("root index with extra keys must fail")
	}
	if _, err := ValidateIndex([]byte("# Tools\n\n- [OKF](okf.md) - floor\n"), false); err == nil {
		t.Fatal("hyphen catalog bullets must fail")
	}
	oversized := "# Tools\n\n* [OKF](okf.md) - floor\n" + strings.Repeat("x", bufio.MaxScanTokenSize+1)
	if _, err := ValidateIndex([]byte(oversized), false); err == nil {
		t.Fatal("index scanner failure must fail closed")
	}
}

func TestValidateLog(t *testing.T) {
	ok := []byte("## 2026-08-29\n* **Init** | bootstrapped.\n\n## 2026-08-01\n* **Older** | earlier.\n")
	if _, err := ValidateLog(ok); err != nil {
		t.Fatalf("newest-first log must pass: %v", err)
	}
	if _, err := ValidateLog([]byte("---\ntype: log\n---\n## 2026-08-29\n")); err == nil {
		t.Fatal("log.md with frontmatter must fail")
	}
	if _, err := ValidateLog([]byte("## Today\n* **Init** |\n")); err == nil {
		t.Fatal("non-ISO ## heading must fail")
	}
	if _, err := ValidateLog([]byte("just prose\n")); err == nil {
		t.Fatal("log.md without a date heading must fail")
	}
	oldFirst := []byte("## 2026-08-01\n* **Older**\n\n## 2026-08-29\n* **Newer**\n")
	if _, err := ValidateLog(oldFirst); err == nil {
		t.Fatal("oldest-first log must fail")
	}
	oversized := "## 2026-08-29\n" + strings.Repeat("x", bufio.MaxScanTokenSize+1)
	if _, err := ValidateLog([]byte(oversized)); err == nil {
		t.Fatal("log scanner failure must fail closed")
	}
}

func TestReservedMarkdown(t *testing.T) {
	kind, root := ReservedMarkdown("index.md")
	if kind != "index" || !root {
		t.Fatalf("root index: %q %v", kind, root)
	}
	kind, root = ReservedMarkdown("tools/index.md")
	if kind != "index" || root {
		t.Fatalf("nested index: %q %v", kind, root)
	}
	kind, _ = ReservedMarkdown("log.md")
	if kind != "log" {
		t.Fatalf("log: %q", kind)
	}
	kind, _ = ReservedMarkdown("AGENTS.md")
	if kind != "" {
		t.Fatalf("AGENTS.md is not reserved: %q", kind)
	}
}
