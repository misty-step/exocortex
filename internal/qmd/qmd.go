// Package qmd shells out to the local QMD CLI for retrieval. The kernel
// never re-implements search (SPEC contract): this is a thin, typed
// passthrough over `qmd query|search|vsearch --format json`.
package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Hit is one record of qmd's JSON output.
type Hit struct {
	DocID   string  `json:"docid"`
	File    string  `json:"file"` // qmd://<collection>/<path>
	Score   float64 `json:"score"`
	Line    int     `json:"line"`
	Title   string  `json:"title"`
	Context string  `json:"context"`
	Snippet string  `json:"snippet"`
}

// subcommand maps retrieval modes onto qmd subcommands.
var subcommand = map[string]string{
	"hybrid": "query",
	"bm25":   "search",
	"vector": "vsearch",
}

// Search runs one qmd retrieval and returns raw hits. The default mode
// is bm25: a kernel primitive must be deterministic and must not depend
// on LLM availability (qmd's hybrid expansion is disabled under CI=true).
func Search(ctx context.Context, query, collection, mode string, limit int) ([]Hit, error) {
	if mode == "" {
		mode = "bm25"
	}
	sub, ok := subcommand[mode]
	if !ok {
		return nil, fmt.Errorf("mode %q must be hybrid, bm25, or vector", mode)
	}
	if limit <= 0 {
		limit = 20
	}
	args := []string{sub, "--format", "json", "-n", strconv.Itoa(limit)}
	if collection != "" {
		args = append(args, "-c", collection)
	}
	args = append(args, query)
	cmd := exec.CommandContext(ctx, "qmd", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("qmd %s failed: %s: %w", sub, strings.TrimSpace(stderr.String()), err)
	}
	var hits []Hit
	if err := json.Unmarshal(out, &hits); err != nil {
		return nil, fmt.Errorf("qmd returned unparseable JSON: %w", err)
	}
	return hits, nil
}

// SplitURI decomposes a qmd file URI into its collection and
// collection-relative path ("qmd://daybook/misty-step/x.md" ->
// "daybook", "misty-step/x.md").
func SplitURI(file string) (collection, rel string, ok bool) {
	const p = "qmd://"
	if !strings.HasPrefix(file, p) {
		return "", "", false
	}
	rest := file[len(p):]
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return rest, "", true
	}
	return rest[:i], rest[i+1:], true
}
