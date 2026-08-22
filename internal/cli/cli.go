// Package cli wires the exocortex command surface. Every command emits
// a JSON document on stdout (CR-02); failures are conflict payloads
// with nonzero exit.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/misty-step/exocortex/internal/kernel"
	"github.com/misty-step/exocortex/internal/mcp"
	"github.com/misty-step/exocortex/internal/qmd"
)

// Main runs one command and returns the process exit code.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		return emit(stdout, nil, inputErr("", "no command given",
			"run `exocortex help` for the command list"))
	}
	cmd, rest := argv[0], argv[1:]
	var payload any
	var conf *kernel.Conflict
	var err error
	switch cmd {
	case "register":
		payload, conf, err = cmdRegister(rest)
	case "put":
		payload, conf, err = cmdPut(rest, stdin)
	case "get":
		payload, conf, err = cmdGet(rest)
	case "search":
		payload, conf, err = cmdSearch(rest)
	case "log":
		payload, conf, err = cmdLog(rest)
	case "note":
		payload, conf, err = cmdNote(rest, stdin)
	case "lint":
		payload, conf, err = cmdLint(rest)
	case "mcp":
		return mcp.Run(stdin, stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		conf = &kernel.Conflict{
			Code:      "unknown_command",
			Operation: cmd,
			Hint:      "run `exocortex help` for the command list",
		}
		return emit(stdout, nil, conf) // input error: exit 2
	}
	if err != nil {
		// Residual internal failures also speak JSON (CR-04).
		conf = &kernel.Conflict{
			Code:      "internal_error",
			Operation: cmd,
			Hint:      "retry; if persistent, inspect the exocortex installation",
			Detail:    map[string]any{"detail": err.Error()},
		}
	}
	return emit(stdout, payload, conf)
}

// emit writes the JSON document and returns the exit code: 0 success,
// 1 operation conflict, 2 invalid input / internal failure.
func emit(stdout io.Writer, payload any, conf *kernel.Conflict) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if conf != nil {
		code := 1
		switch conf.Code {
		case "unknown_command", "invalid_input", "registration_failed",
			"payload_unreadable", "internal_error":
			code = 2
		}
		_ = enc.Encode(conflictBody(conf))
		return code
	}
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "exocortex: encoding output: %v\n", err)
		return 2
	}
	return 0
}

// inputErr builds an invalid_input conflict for usage-level failures.
func inputErr(cmd, detail, hint string) *kernel.Conflict {
	return &kernel.Conflict{
		Code:      "invalid_input",
		Operation: cmd,
		Hint:      hint,
		Detail:    map[string]any{"detail": detail},
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `exocortex — fleet memory kernel over registered cortices

Usage:
  exocortex register <name> <path> [--vcs daybook|caller|none] [--profile daybook|strict]
  exocortex put <path> --from <file|-> [--expects <sha>] [--cortex <name>] [--agent <id>]
  exocortex get <path> [--cortex <name>]
  exocortex search "<query>" [--cortex <name>] [--limit N] [--mode bm25|hybrid|vector]
  exocortex log <path> [--cortex <name>] [--limit N]
  exocortex lint [<path>] [--cortex <name>]
  exocortex mcp

All output is JSON. Conflicts and errors exit nonzero with a JSON body
naming the error, operation, path, and recovery hint.
`)
}

// conflictBody renders a Conflict as its pinned JSON shape.
func conflictBody(c *kernel.Conflict) map[string]any {
	body := map[string]any{
		"error": c.Code,
		"hint":  c.Hint,
	}
	if c.Operation != "" {
		body["operation"] = c.Operation
	}
	if c.Path != "" {
		body["path"] = c.Path
	}
	for k, v := range c.Detail {
		body[k] = v
	}
	return body
}

// splitArgs separates flag tokens from positional arguments regardless
// of order (stdlib flag stops at the first positional). valueFlags names
// the flags that consume the following token; unknown dash-tokens are
// left for flag.Parse to reject.
func splitArgs(args []string, valueFlags map[string]bool) (flags, positional []string) {
	takesValue := func(tok string) bool {
		name := strings.TrimLeft(tok, "-")
		if name == "" {
			return false
		}
		return valueFlags[name]
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			return
		case strings.HasPrefix(a, "-") && a != "-" && !strings.HasPrefix(a, "--="):
			flags = append(flags, a)
			if !strings.Contains(a, "=") && takesValue(a) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			positional = append(positional, a)
		}
	}
	return
}

// commonFlags registers the flags every cortex-touching command shares.
func commonFlags(fs *flag.FlagSet) *string {
	fs.Bool("json", true, "JSON output (always on; accepted for compatibility)")
	agent := fs.String("agent", "", "agent id stamped into provenance (env EXOCORTEX_AGENT)")
	cortex := fs.String("cortex", "", "explicit cortex name (optional)")
	_ = agent
	return cortex
}

func agentFlag(fs *flag.FlagSet) string {
	if f := fs.Lookup("agent"); f != nil {
		return f.Value.String()
	}
	return ""
}

func loadRegistry() ([]kernel.Cortex, error) {
	return kernel.LoadRegistry()
}

func cmdRegister(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool("json", true, "JSON output (always on)")
	vcs := fs.String("vcs", "", "vcs policy: daybook | caller | none (default: auto-detect)")
	profile := fs.String("profile", "", "validation profile: daybook | strict (default daybook)")
	jprefix := fs.String("journal-prefix", "", "where note files land inside the cortex (default journal)")
	flags, pos := splitArgs(args, map[string]bool{"vcs": true, "profile": true, "journal-prefix": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("register", err.Error(), "run `exocortex help` for register usage"), nil
	}
	if len(pos) != 2 {
		return nil, inputErr("register", "register requires <name> <path>",
			"exocortex register <name> <path> [--vcs daybook|caller|none]"), nil
	}
	// Duplicate detection as data, before the domain error.
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	for i := range cs {
		if cs[i].Name == pos[0] {
			return nil, &kernel.Conflict{Code: "duplicate_cortex", Operation: "register",
				Path: pos[0], Hint: "pick a new name or inspect the existing cortex with get/search",
				Detail: map[string]any{"path": cs[i].Path}}, nil
		}
	}
	c, err := kernel.Register(pos[0], pos[1], *vcs, *profile, *jprefix)
	if err != nil {
		return nil, &kernel.Conflict{Code: "registration_failed", Operation: "register",
			Path: pos[1], Hint: "fix the name (lowercase slug), path, vcs, or profile and retry",
			Detail: map[string]any{"detail": err.Error()}}, nil
	}
	return map[string]any{"registered": c}, nil, nil
}

func cmdPut(args []string, stdin io.Reader) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("put", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	from := fs.String("from", "", "payload file, or - for stdin")
	expects := fs.String("expects", "", "stored revision (sha256 hex) required for updates")
	flags, pos := splitArgs(args, map[string]bool{
		"from": true, "expects": true, "cortex": true, "agent": true, "json": false,
	})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("put", err.Error(), "run `exocortex help` for put usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("put", "put requires exactly one <path>",
			"exocortex put <path> --from <file|->"), nil
	}
	if *from == "" {
		return nil, inputErr("put", "put requires --from <file|-> (payload and destination are separate)",
			"exocortex put <path> --from <file|->"), nil
	}
	var payload []byte
	var err error
	if *from == "-" {
		payload, err = io.ReadAll(stdin)
	} else {
		payload, err = os.ReadFile(*from)
	}
	if err != nil {
		return nil, &kernel.Conflict{Code: "payload_unreadable", Operation: "put",
			Path: *from, Hint: "check the --from path; use - to read the payload from stdin",
			Detail: map[string]any{"detail": err.Error()}}, nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Put(context.Background(), cs, kernel.PutInput{
		CortexName: *cortex,
		Path:       pos[0],
		Payload:    payload,
		Expects:    *expects,
		Agent:      agentFlag(fs),
		Via:        "cli",
		OwnPayload: *from != "-",
	})
	return res, conf, nil
}

func cmdGet(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("get", err.Error(), "run `exocortex help` for get usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("get", "get requires <path>", "exocortex get <path> [--cortex <name>]"), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Get(cs, *cortex, pos[0])
	return res, conf, nil
}

func cmdSearch(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	limit := fs.Int("limit", 20, "max hits")
	mode := fs.String("mode", "bm25", "retrieval mode: bm25 (deterministic default) | hybrid | vector")
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "limit": true, "mode": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("search", err.Error(), "run `exocortex help` for search usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("search", `search requires one "<query>"`, `exocortex search "<query>"`), nil
	}
	hits, err := qmd.Search(context.Background(), pos[0], *cortex, *mode, *limit)
	if err != nil {
		return nil, &kernel.Conflict{
			Code:      "search_unavailable",
			Operation: "search",
			Path:      pos[0],
			Hint:      "check that qmd is installed and the cortex has an indexed qmd collection of the same name",
			Detail:    map[string]any{"detail": err.Error()},
		}, nil
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
	return out, nil, nil
}
func cmdLog(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	limit := fs.Int("limit", 50, "max entries")
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "limit": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("log", err.Error(), "run `exocortex help` for log usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("log", "log requires <path>", "exocortex log <path> [--cortex <name>]"), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	entries, conf := kernel.Log(cs, *cortex, pos[0], *limit)
	if conf != nil {
		return nil, conf, nil
	}
	if entries == nil {
		entries = []kernel.LogEntry{}
	}
	name := *cortex
	if name == "" {
		if c, _, rerr := kernel.Resolve(cs, "", pos[0]); rerr == nil {
			name = c.Name
		}
	}
	return map[string]any{"cortex": name, "path": pos[0], "entries": entries}, nil, nil
}

func cmdLint(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("lint", err.Error(), "run `exocortex help` for lint usage"), nil
	}
	if len(pos) > 1 {
		return nil, inputErr("lint", "lint takes at most one <path>", "exocortex lint [<path>]"), nil
	}
	path := ""
	if len(pos) == 1 {
		path = pos[0]
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Lint(cs, *cortex, path)
	return res, conf, nil
}

func cmdNote(args []string, stdin io.Reader) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("note", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("note", err.Error(), "run `exocortex help` for note usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("note", `note requires exactly one "<thought>"`,
			`exocortex note "<one thought worth remembering>"`), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Note(context.Background(), cs, kernel.NoteInput{
		CortexName: *cortex,
		Text:       pos[0],
		Agent:      agentFlag(fs),
		Via:        "cli",
	})
	return res, conf, nil
}
