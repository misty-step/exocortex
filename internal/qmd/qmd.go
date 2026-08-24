// Package qmd shells out to the local QMD CLI for retrieval. The kernel
// never re-implements search (SPEC contract): this is a thin, typed
// passthrough over `qmd query|search|vsearch --format json`.
package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
func subcommandFor(mode string) (string, error) {
	if mode == "" {
		mode = "bm25"
	}
	sub, ok := subcommand[mode]
	if !ok {
		return "", fmt.Errorf("mode %q must be hybrid, bm25, or vector", mode)
	}
	return sub, nil
}

func sanitizeEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CI=") && !strings.HasPrefix(e, "CI_") {
			env = append(env, e)
		}
	}
	return env
}

// Search runs one qmd retrieval with a sanitized environment and returns raw hits.
// If hybrid query expansion fails, it falls back to deterministic BM25 search.
func Search(ctx context.Context, query, collection, mode string, limit int) ([]Hit, error) {
	if mode == "" {
		mode = "hybrid"
	}
	sub, ok := subcommand[mode]
	if !ok {
		return nil, fmt.Errorf("mode %q must be hybrid, bm25, or vector", mode)
	}
	if limit <= 0 {
		limit = 20
	}

	run := func(cmdName string) ([]Hit, error) {
		args := []string{cmdName, "--format", "json", "-n", strconv.Itoa(limit)}
		if collection != "" {
			args = append(args, "-c", collection)
		}
		args = append(args, query)
		cmd := exec.CommandContext(ctx, "qmd", args...)
		cmd.Env = sanitizeEnv()
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("qmd %s failed: %s: %w", cmdName, strings.TrimSpace(stderr.String()), err)
		}
		var hits []Hit
		if err := json.Unmarshal(out, &hits); err != nil {
			return nil, fmt.Errorf("qmd returned unparseable JSON: %w", err)
		}
		return hits, nil
	}

	hits, err := run(sub)
	if err != nil && mode == "hybrid" {
		// Fallback to deterministic BM25 search if hybrid fails
		return run("search")
	}
	return hits, err
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
