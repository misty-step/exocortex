// Package mcp exposes the kernel operations as an MCP stdio server —
// the second face of the same binary (SPEC: CLI and MCP from one
// implementation).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/misty-step/exocortex/internal/kernel"
	"github.com/misty-step/exocortex/internal/orient"
	"github.com/misty-step/exocortex/internal/qmd"
)

// Run serves MCP over stdio until the client disconnects.
func Run(stdin io.Reader, stdout, stderr io.Writer) int {
	server := mcp.NewServer(&mcp.Implementation{Name: "exocortex", Version: "v0"}, nil)
	addTools(server)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(stderr, "exocortex mcp: %v\n", err)
		return 1
	}
	return 0
}

// toolResult renders a kernel payload as JSON text content, or a
// conflict as an error result carrying the pinned conflict body.
func toolResult(payload any, conf *kernel.Conflict) (*mcp.CallToolResult, any, error) {
	if conf != nil {
		raw, err := json.Marshal(conf.Body())
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
			IsError: true,
		}, nil, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
	}, nil, nil
}

type getArgs struct {
	Path   string `json:"path" jsonschema:"note path inside the cortex (or under a registered cortex root)"`
	Cortex string `json:"cortex,omitempty" jsonschema:"explicit cortex name"`
}

type putArgs struct {
	Path             string `json:"path" jsonschema:"destination note path inside the cortex"`
	Content          string `json:"content" jsonschema:"full note payload (markdown with frontmatter)"`
	ExpectedRevision string `json:"expectedRevision,omitempty" jsonschema:"stored revision (sha256 hex) REQUIRED to update an existing note; omit to create"`
	Cortex           string `json:"cortex,omitempty" jsonschema:"explicit cortex name"`
	Agent            string `json:"agent,omitempty" jsonschema:"agent id stamped into provenance"`
}

type searchArgs struct {
	Query  string `json:"query" jsonschema:"search text"`
	Cortex string `json:"cortex,omitempty" jsonschema:"restrict to one cortex (qmd collection of the same name)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max hits (default 20, max 100)"`
	Mode   string `json:"mode,omitempty" jsonschema:"hybrid (default, BM25 fallback) | bm25 | vector"`
	Type   string `json:"type,omitempty" jsonschema:"filter by content kind: decision | memo | session | note | scratch"`
}

type noteArgs struct {
	Text   string `json:"text" jsonschema:"one thought worth remembering (a line or three)"`
	Cortex string `json:"cortex,omitempty" jsonschema:"explicit cortex name"`
	Agent  string `json:"agent,omitempty" jsonschema:"agent id stamped into provenance"`
}

type logArgs struct {
	Path   string `json:"path" jsonschema:"note path"`
	Cortex string `json:"cortex,omitempty" jsonschema:"explicit cortex name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max entries (default 50)"`
}

type lintArgs struct {
	Path   string `json:"path,omitempty" jsonschema:"note path; omit to lint the whole cortex"`
	Cortex string `json:"cortex,omitempty" jsonschema:"explicit cortex name"`
}

type syncArgs struct {
	Cortex string `json:"cortex,omitempty" jsonschema:"optional cortex name; omit to walk every registered cortex"`
}

type statusArgs struct {
	Cortex string `json:"cortex,omitempty" jsonschema:"optional cortex name; omit to report every registered cortex"`
}

type registerArgs struct {
	Name          string `json:"name" jsonschema:"cortex name (lowercase slug)"`
	Path          string `json:"path" jsonschema:"absolute path of the corpus root"`
	VCS           string `json:"vcs,omitempty" jsonschema:"daybook | caller | none (default: auto-detect)"`
	Profile       string `json:"profile,omitempty" jsonschema:"daybook | strict (default daybook)"`
	JournalPrefix string `json:"journalPrefix,omitempty" jsonschema:"where note files land (default journal)"`
}

func addTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_get", Description: "Read one note: revision (sha256), frontmatter, full content."}, get)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_put", Description: "Create or CAS-update a note. Bare put creates only; updating REQUIRES expectedRevision from a prior get. Provenance is stamped automatically; the daybook cortex commits and pushes."}, put)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_note", Description: "Record one journal micro-memory as an immutable file on the cortex's agent board. Cheap by design: use often for status updates, gotchas and fixes, decisions in flight."}, noteTool)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_search", Description: "Search cortex content via QMD; hits carry cortex, path, score, snippet."}, search)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_log", Description: "Git lineage of one note."}, logTool)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_lint", Description: "Frontmatter floor gate; tiered findings, only errors block."}, lint)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_register", Description: "Register a cortex (markdown+git corpus) for full read/write/search/lint."}, register)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_sync", Description: "Refresh the QMD index and embeddings for dirty cortices. Fail-closed if the collection does not point at the kernel indexed root."}, syncTool)
	mcp.AddTool(server, &mcp.Tool{Name: "exocortex_status", Description: "Report dirty markers, last synced identity, and last sync error without creating state or clones."}, statusTool)
}

func get(ctx context.Context, req *mcp.CallToolRequest, a getArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Get(cs, a.Cortex, a.Path)
	return toolResult(res, conf)
}

func put(ctx context.Context, req *mcp.CallToolRequest, a putArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Put(ctx, cs, kernel.PutInput{
		CortexName: a.Cortex,
		Path:       a.Path,
		Payload:    []byte(a.Content),
		Expects:    a.ExpectedRevision,
		Agent:      a.Agent,
		Via:        "mcp",
	})
	return toolResult(res, conf)
}

func search(ctx context.Context, req *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
	var collections []string
	if a.Cortex != "" {
		collections = []string{a.Cortex}
	}
	limit := a.Limit
	if limit <= 0 {
		limit = qmd.DefaultSearchLimit
	}
	hits, err := qmd.Search(ctx, a.Query, collections, a.Mode, mcpFetchLimit(limit, a.Type))
	if err != nil {
		return toolResult(nil, &kernel.Conflict{
			Code:      "search_unavailable",
			Operation: "search",
			Path:      a.Query,
			Hint:      "check that qmd is installed and the cortex has an indexed qmd collection of the same name",
			Detail:    map[string]any{"detail": err.Error()},
		})
	}
	var cs []kernel.Cortex
	if a.Type != "" {
		cs, err = kernel.LoadRegistry()
		if err != nil {
			return nil, nil, err
		}
	}
	projected, conf := projectMCPHits(hits, cs, a.Type, limit)
	return toolResult(projected, conf)
}

func mcpFetchLimit(limit int, typeFilter string) int {
	if typeFilter == "" || limit >= qmd.MaxSearchLimit {
		return limit
	}
	fetchLimit := limit * 5
	if fetchLimit > qmd.MaxSearchLimit {
		return qmd.MaxSearchLimit
	}
	return fetchLimit
}

func projectMCPHits(hits []qmd.Hit, cs []kernel.Cortex, typeFilter string, limit int) ([]map[string]any, *kernel.Conflict) {
	fetched := make([]*kernel.GetResult, len(hits))
	if typeFilter != "" {
		requests := make([]kernel.GetRequest, 0, len(hits))
		indexes := make([]int, 0, len(hits))
		for i, h := range hits {
			collection, path, ok := qmd.SplitURI(h.File)
			if !ok || path == "" || kernel.CortexNamed(cs, collection) == nil {
				continue
			}
			requests = append(requests, kernel.GetRequest{CortexName: collection, Path: path})
			indexes = append(indexes, i)
		}
		for i, outcome := range kernel.GetMany(cs, requests) {
			if outcome.Conflict != nil {
				return nil, outcome.Conflict
			}
			fetched[indexes[i]] = outcome.Result
		}
	}
	out := make([]map[string]any, 0, len(hits))
	for i, h := range hits {
		entry, keep := projectMCPHit(h, cs, typeFilter, fetched[i])
		if !keep {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func projectMCPHit(h qmd.Hit, cs []kernel.Cortex, typeFilter string, res *kernel.GetResult) (map[string]any, bool) {
	entry := map[string]any{
		"docid":   h.DocID,
		"score":   h.Score,
		"line":    h.Line,
		"title":   h.Title,
		"context": h.Context,
		"snippet": h.Snippet,
		"file":    h.File,
	}
	var (
		c   *kernel.Cortex
		rel string
		fm  map[string]any
	)
	if collection, path, ok := qmd.SplitURI(h.File); ok {
		entry["cortex"] = collection
		entry["path"] = path
		rel = path
		c = kernel.CortexNamed(cs, collection)
		if res != nil {
			fm = res.Frontmatter
		}
	}
	if typeFilter != "" && !orient.MatchType(kernel.JournalPrefix(c), rel, h.File, typeFilter, fm, res != nil) {
		return nil, false
	}
	return entry, true
}

func noteTool(ctx context.Context, req *mcp.CallToolRequest, a noteArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Note(ctx, cs, kernel.NoteInput{
		CortexName: a.Cortex,
		Text:       a.Text,
		Agent:      a.Agent,
		Via:        "mcp",
	})
	return toolResult(res, conf)
}

func logTool(ctx context.Context, req *mcp.CallToolRequest, a logArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	entries, conf := kernel.Log(cs, a.Cortex, a.Path, a.Limit)
	if conf != nil {
		return toolResult(nil, conf)
	}
	if entries == nil {
		entries = []kernel.LogEntry{}
	}
	name := a.Cortex
	if name == "" {
		if c, _, rerr := kernel.Resolve(cs, "", a.Path); rerr == nil {
			name = c.Name
		}
	}
	return toolResult(map[string]any{"cortex": name, "path": a.Path, "entries": entries}, nil)
}

func lint(ctx context.Context, req *mcp.CallToolRequest, a lintArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Lint(cs, a.Cortex, a.Path)
	return toolResult(res, conf)
}

func register(ctx context.Context, req *mcp.CallToolRequest, a registerArgs) (*mcp.CallToolResult, any, error) {
	c, err := kernel.Register(a.Name, a.Path, a.VCS, a.Profile, a.JournalPrefix)
	if err != nil {
		if conf, ok := err.(*kernel.Conflict); ok {
			return toolResult(nil, conf)
		}
		return toolResult(nil, &kernel.Conflict{
			Code:      "registration_failed",
			Operation: "register",
			Path:      a.Path,
			Hint:      "fix the name (lowercase slug), path, vcs, or profile and retry",
			Detail:    map[string]any{"detail": err.Error()},
		})
	}
	return toolResult(map[string]any{"registered": c}, nil)
}

func syncTool(ctx context.Context, req *mcp.CallToolRequest, a syncArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Sync(ctx, cs, a.Cortex)
	if conf != nil && res != nil {
		if conf.Detail == nil {
			conf.Detail = map[string]any{}
		}
		conf.Detail["results"] = res
	}
	if conf != nil {
		return toolResult(nil, conf)
	}
	return toolResult(res, nil)
}

func statusTool(ctx context.Context, req *mcp.CallToolRequest, a statusArgs) (*mcp.CallToolResult, any, error) {
	cs, err := kernel.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Status(cs, a.Cortex)
	return toolResult(res, conf)
}
