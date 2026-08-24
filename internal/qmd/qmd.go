// Package qmd shells out to the local QMD CLI for retrieval. The kernel
// never re-implements search (SPEC contract): this is a thin, typed
// passthrough over `qmd query|search|vsearch --format json`.
package qmd

import (
	"bytes"
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
func Search(ctx context.Context, query string, collections []string, mode string, limit int) ([]Hit, error) {
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
		tmpFile, err := os.CreateTemp("", "qmd-search-*.json")
		if err != nil {
			return nil, err
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		args := []string{cmdName, "--format", "json", "-n", strconv.Itoa(limit)}
		for _, c := range collections {
			if c != "" {
				args = append(args, "-c", c)
			}
		}
		args = append(args, query)
		cmd := exec.CommandContext(ctx, "qmd", args...)
		cmd.Env = sanitizeEnv()
		cmd.Stdout = tmpFile
		var stderr strings.Builder
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("qmd %s failed: %s: %w", cmdName, strings.TrimSpace(stderr.String()), err)
		}
		tmpFile.Close()

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return nil, err
		}
		start := bytes.IndexByte(data, '[')
		if start < 0 {
			return nil, fmt.Errorf("qmd %s returned non-JSON output: %s", cmdName, strings.TrimSpace(string(data)))
		}
		var hits []Hit
		if err := json.Unmarshal(data[start:], &hits); err != nil {
			return nil, fmt.Errorf("qmd returned unparseable JSON: %w", err)
		}
		return hits, nil
	}
	hits, err := run(sub)
	if err != nil && mode == "hybrid" {
		return run("search")
	}
	return hits, err
}

// Update runs `qmd update <collection>`.
func Update(ctx context.Context, collection string) error {
	args := []string{"update"}
	if collection != "" {
		args = append(args, collection)
	}
	cmd := exec.CommandContext(ctx, "qmd", args...)
	cmd.Env = sanitizeEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("qmd update failed: %s %s: %w", strings.TrimSpace(stderr.String()), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Embed runs `qmd embed -c <collection>`.
func Embed(ctx context.Context, collection string) error {
	args := []string{"embed"}
	if collection != "" {
		args = append(args, "-c", collection)
	}
	cmd := exec.CommandContext(ctx, "qmd", args...)
	cmd.Env = sanitizeEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("qmd embed failed: %s %s: %w", strings.TrimSpace(stderr.String()), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CollectionPath returns the on-disk root of a named QMD collection by
// parsing `qmd collection show`. Missing collections and unparseable
// output fail closed.
func CollectionPath(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("qmd collection show: empty collection name")
	}
	cmd := exec.CommandContext(ctx, "qmd", "collection", "show", name)
	cmd.Env = sanitizeEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("qmd collection show %s: collection must exist and expose a non-empty Path: %s %w", name, strings.TrimSpace(stderr.String()), err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "Path:")
		if !ok {
			continue
		}
		p := strings.TrimSpace(rest)
		if p == "" {
			return "", fmt.Errorf("qmd collection show %s: collection must expose a non-empty Path", name)
		}
		return p, nil
	}
	return "", fmt.Errorf("qmd collection show %s: collection must exist and expose a non-empty Path", name)
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
