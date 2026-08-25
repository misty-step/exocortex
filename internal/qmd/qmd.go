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
		if strings.HasPrefix(e, "CI=") || strings.HasPrefix(e, "CI_") ||
			strings.HasPrefix(e, "INDEX_PATH=") || strings.HasPrefix(e, "QMD_CONFIG_DIR=") {
			continue
		}
		env = append(env, e)
	}
	return env
}

// indexFlag pins the global QMD index so cwd-local .qmd trees cannot
// steal search or sync from the fleet collection database.
var indexFlag = []string{"--index", "index"}

func qmdArgs(args ...string) []string {
	out := make([]string, 0, len(indexFlag)+len(args))
	out = append(out, indexFlag...)
	return append(out, args...)
}

// Search runs one qmd retrieval with a sanitized environment and returns raw hits.
// If hybrid query expansion fails, it falls back to deterministic BM25 search.
func Search(ctx context.Context, query string, collections []string, mode string, limit int) ([]Hit, error) {
	sub, err := subcommandFor(mode)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	run := func(cmdName string) ([]Hit, error) {
		args := qmdArgs(cmdName, "--format", "json", "-n", strconv.Itoa(limit))
		for _, c := range collections {
			if c != "" {
				args = append(args, "-c", c)
			}
		}
		args = append(args, query)
		cmd := exec.CommandContext(ctx, "qmd", args...)
		cmd.Env = sanitizeEnv()
		var stderr strings.Builder
		cmd.Stderr = &stderr

		data, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("qmd %s failed: %s: %w", cmdName, strings.TrimSpace(stderr.String()), err)
		}

		return parseJSONHits(cmdName, data)
	}
	hits, err := run(sub)
	if err != nil && (mode == "hybrid" || (mode == "" && sub == "query")) && ctx.Err() == nil {
		return run("search")
	}
	return hits, err
}

// findJSONArrayStart locates the offset of a top-level JSON array opener in data.
// It skips non-JSON logging preambles (e.g. "[info] ...") by requiring the '[' to
// appear at a line boundary followed by valid array contents ('{' or ']').
func findJSONArrayStart(data []byte) (int, bool) {
	for offset := 0; offset < len(data); {
		// Skip leading line-break and whitespace bytes
		for offset < len(data) && (data[offset] == ' ' || data[offset] == '\t' || data[offset] == '\r' || data[offset] == '\n') {
			offset++
		}
		if offset >= len(data) {
			break
		}
		if data[offset] == '[' {
			rest := data[offset+1:]
			for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\r' || rest[0] == '\n') {
				rest = rest[1:]
			}
			if len(rest) > 0 && (rest[0] == '{' || rest[0] == ']') {
				return offset, true
			}
		}
		nl := bytes.IndexByte(data[offset:], '\n')
		if nl < 0 {
			break
		}
		offset += nl + 1
	}
	return -1, false
}

// parseJSONHits parses stdout into a slice of Hit records in memory.
func parseJSONHits(cmdName string, data []byte) ([]Hit, error) {
	start, ok := findJSONArrayStart(data)
	if !ok {
		preview := strings.TrimSpace(string(data))
		if len(preview) > 256 {
			preview = preview[:256] + "..."
		}
		return nil, fmt.Errorf("qmd %s returned non-JSON output: %s", cmdName, preview)
	}
	var hits []Hit
	if err := json.Unmarshal(data[start:], &hits); err != nil {
		return nil, fmt.Errorf("qmd returned unparseable JSON: %w", err)
	}
	if hits == nil {
		hits = []Hit{}
	}
	return hits, nil
}

// Update runs `qmd update <collection>`.
func Update(ctx context.Context, collection string) error {
	args := qmdArgs("update")
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

// Embed runs `qmd embed -c <collection>`. Exit 0 is not enough:
// installed QMD can skip remaining batches or leave failed chunks and
// still return success. We require a completion banner and reject the
// known incomplete banners.
func Embed(ctx context.Context, collection string) error {
	args := qmdArgs("embed")
	if collection != "" {
		args = append(args, "-c", collection)
	}
	cmd := exec.CommandContext(ctx, "qmd", args...)
	cmd.Env = sanitizeEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qmd embed failed: %s %s: %w", strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()), err)
	}
	out := stdout.String() + "\n" + stderr.String()
	if strings.Contains(out, "chunks still failed after retries") || strings.Contains(out, "Session expired") {
		return fmt.Errorf("qmd embed incomplete: %s", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "Done!") && !strings.Contains(out, "No non-empty documents to embed") {
		return fmt.Errorf("qmd embed incomplete: missing completion banner")
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
	cmd := exec.CommandContext(ctx, "qmd", qmdArgs("collection", "show", name)...)
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
