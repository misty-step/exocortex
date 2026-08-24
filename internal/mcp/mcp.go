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
		body := map[string]any{"error": conf.Code, "hint": conf.Hint}
		if conf.Operation != "" {
			body["operation"] = conf.Operation
		}
		if conf.Path != "" {
			body["path"] = conf.Path
		}
		for k, v := range conf.Detail {
			body[k] = v
		}
		raw, _ := json.Marshal(body)
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
	Limit  int    `json:"limit,omitempty" jsonschema:"max hits (default 20)"`
	Mode   string `json:"mode,omitempty" jsonschema:"bm25 (deterministic default) | hybrid | vector"`
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
	hits, err := qmd.Search(ctx, a.Query, collections, a.Mode, a.Limit)
	if err != nil {
		return toolResult(nil, &kernel.Conflict{
			Code:      "search_unavailable",
			Operation: "search",
			Path:      a.Query,
			Hint:      "check that qmd is installed and the cortex has an indexed qmd collection of the same name",
			Detail:    map[string]any{"detail": err.Error()},
		})
	}
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		entry := map[string]any{
			"docid":   h.DocID,
			"score":   h.Score,
			"line":    h.Line,
			"title":   h.Title,
			"context": h.Context,
			"snippet": h.Snippet,
			"file":    h.File,
		}
		if collection, rel, ok := qmd.SplitURI(h.File); ok {
			entry["cortex"] = collection
			entry["path"] = rel
		}
		out = append(out, entry)
	}
	return toolResult(out, nil)
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
		return nil, nil, err
	}
	return toolResult(map[string]any{"registered": c}, nil)
}
