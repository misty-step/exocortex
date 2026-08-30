package fm

import "testing"

func TestValidatePathOKF(t *testing.T) {
	cases := []struct {
		name, profile, path, raw, rule string
		reserved                       bool
	}{
		{"daybook unchanged", "daybook", "index.md", "# Root\n* [Tool](tool.md)\n", "fm_missing", false},
		{"root index", "okf", "index.md", "---\nokf_version: 0.1\n---\n# Root\n* [Tool](tool.md) - catalog\n", "", true},
		{"nested index", "okf", "tools/index.md", "# Tools\n* [Tool](tool.md)\n", "", true},
		{"root extra key", "okf", "index.md", "---\ntype: index\n---\n# Root\n* [Tool](tool.md)\n", "unknown_keys", true},
		{"nested frontmatter", "okf", "tools/index.md", "---\ntype: index\n---\n# Tools\n* [Tool](tool.md)\n", "reserved_frontmatter", true},
		{"hyphen catalog", "okf", "index.md", "# Root\n- [Tool](tool.md)\n", "index_format", true},
		{"malformed catalog", "okf", "index.md", "# Root\n* prose\n", "index_format", true},
		{"log", "okf", "log.md", "##\t2026-08-29\nentry\n## 2026-08-01\nolder\n", "", true},
		{"log date", "okf", "log.md", "##\tToday\n", "log_heading", true},
		{"log order", "okf", "log.md", "## 2026-08-01\n## 2026-08-29\n", "log_order", true},
		{"ordinary note", "okf", "tools/tool.md", "body\n", "fm_missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reserved, _, err := ValidatePath(tc.profile, tc.path, ParseDocument([]byte(tc.raw)))
			if reserved != tc.reserved {
				t.Fatalf("reserved=%v, want %v", reserved, tc.reserved)
			}
			finding, failed := ContractFinding(err)
			if tc.rule == "" && err != nil || tc.rule != "" && (!failed || finding.Rule != tc.rule) {
				t.Fatalf("error=%v finding=%+v, want rule %q", err, finding, tc.rule)
			}
		})
	}
}
