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
		usage(stderr)
		return 2
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
	case "lint":
		payload, conf, err = cmdLint(rest)
	case "mcp":
		return mcp.Run(stdin, stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "exocortex: unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "exocortex: %v\n", err)
		return 2
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if conf != nil {
		_ = enc.Encode(conflictBody(conf))
		return 1
	}
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(stderr, "exocortex: encoding output: %v\n", err)
		return 2
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprint(w, `exocortex — fleet memory kernel over registered cortices

Usage:
  exocortex register <name> <path> [--vcs daybook|caller|none] [--profile daybook|strict]
  exocortex put <path> --from <file|-> [--expects <sha>] [--cortex <name>] [--agent <id>]
  exocortex get <path> [--cortex <name>]
  exocortex search "<query>" [--cortex <name>] [--limit N] [--mode hybrid|bm25|vector]
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
	vcs := fs.String("vcs", "", "vcs policy: daybook | caller | none (default: auto-detect)")
	profile := fs.String("profile", "", "validation profile: daybook | strict (default daybook)")
	flags, pos := splitArgs(args, map[string]bool{"vcs": true, "profile": true})
	if err := fs.Parse(flags); err != nil {
		return nil, nil, err
	}
	if len(pos) != 2 {
		return nil, nil, fmt.Errorf("register requires <name> <path>")
	}
	c, err := kernel.Register(pos[0], pos[1], *vcs, *profile)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	if len(pos) != 1 {
		return nil, nil, fmt.Errorf("put requires exactly one <path>")
	}
	if *from == "" {
		return nil, nil, fmt.Errorf("put requires --from <file|-> (payload and destination are separate)")
	}
	var payload []byte
	var err error
	if *from == "-" {
		payload, err = io.ReadAll(stdin)
	} else {
		payload, err = os.ReadFile(*from)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading payload: %w", err)
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
		return nil, nil, err
	}
	if len(pos) != 1 {
		return nil, nil, fmt.Errorf("get requires <path>")
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
	mode := fs.String("mode", "hybrid", "retrieval mode: hybrid | bm25 | vector")
	flags, pos := splitArgs(args, map[string]bool{"cortex": true, "limit": true, "mode": true, "agent": true})
	if err := fs.Parse(flags); err != nil {
		return nil, nil, err
	}
	if len(pos) != 1 {
		return nil, nil, fmt.Errorf(`search requires one "<query>"`)
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
		return nil, nil, err
	}
	if len(pos) != 1 {
		return nil, nil, fmt.Errorf("log requires <path>")
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
		return nil, nil, err
	}
	if len(pos) > 1 {
		return nil, nil, fmt.Errorf("lint takes at most one <path>")
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
