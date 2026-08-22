package qmd

import "testing"

// Empty mode must select the deterministic BM25 subcommand: the kernel
// primitive may not depend on LLM-backed expansion availability.
func TestSubcommandFor(t *testing.T) {
	cases := map[string]string{
		"":       "search",
		"bm25":   "search",
		"hybrid": "query",
		"vector": "vsearch",
	}
	for mode, want := range cases {
		got, err := subcommandFor(mode)
		if err != nil || got != want {
			t.Errorf("subcommandFor(%q) = %q, %v; want %q", mode, got, err, want)
		}
	}
	if _, err := subcommandFor("semantic"); err == nil {
		t.Error("unknown mode must error")
	}
}
