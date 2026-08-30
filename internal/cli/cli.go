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
	"github.com/misty-step/exocortex/internal/orient"
	"github.com/misty-step/exocortex/internal/qmd"
)

// Main runs one command and returns the process exit code.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		return emit(stdout, nil, inputErr("", "no command given",
			"run `exocortex help` for the command list"))
	}
	cmd, rest := argv[0], argv[1:]
	if code, handled := dispatchSpecial(cmd, stdin, stdout, stderr); handled {
		return code
	}
	payload, conf, err := dispatch(cmd, rest, stdin)
	if err != nil {
		conf = &kernel.Conflict{
			Code:      "internal_error",
			Operation: cmd,
			Hint:      "retry; if persistent, inspect the exocortex installation",
			Detail:    map[string]any{"detail": err.Error()},
		}
	}
	return emit(stdout, payload, conf)
}

func dispatchSpecial(cmd string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	switch cmd {
	case "mcp":
		return mcp.Run(stdin, stdout, stderr), true
	case "help", "-h", "--help":
		usage(stdout)
		return 0, true
	default:
		return 0, false
	}
}

func dispatch(cmd string, rest []string, stdin io.Reader) (any, *kernel.Conflict, error) {
	switch cmd {
	case "register":
		return cmdRegister(rest)
	case "put":
		return cmdPut(rest, stdin)
	case "get":
		return cmdGet(rest)
	case "search":
		return cmdSearch(rest)
	case "brief":
		return cmdBrief(rest)
	case "log":
		return cmdLog(rest)
	case "note":
		return cmdNote(rest, stdin)
	case "lint":
		return cmdLint(rest)
	case "sync":
		return cmdSync(rest)
	case "status":
		return cmdStatus(rest)
	default:
		return nil, &kernel.Conflict{
			Code:      "unknown_command",
			Operation: cmd,
			Hint:      "run `exocortex help` for the command list",
		}, nil
	}
}

// emit writes the JSON document and returns the exit code: 0 success,
// 1 operation conflict, 2 invalid input / internal failure.
func emit(stdout io.Writer, payload any, conf *kernel.Conflict) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if conf != nil {
		_ = enc.Encode(conf.Body())
		return exitFor(conf.Class())
	}
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "exocortex: encoding output: %v\n", err)
		return 2
	}
	return 0
}

func exitFor(c kernel.Class) int {
	switch c {
	case kernel.ClassInput, kernel.ClassInternal:
		return 2
	case kernel.ClassOperation, kernel.ClassUnavailable:
		return 1
	default:
		return 1
	}
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
	_, _ = fmt.Fprint(w, `exocortex — fleet memory kernel over registered cortices

Usage:
  exocortex register <name> <path> [--vcs daybook|caller|none] [--profile daybook|strict|okf]
  exocortex put <path> --from <file|-> [--expects <sha>] [--cortex <name>] [--agent <id>]
  exocortex get <path> [--cortex <name>]
  exocortex search "<query>" [--cortex <name>] [--mode hybrid|bm25|vector] [--type <kind>] [--limit <n>]  # omitted mode is hybrid
  exocortex brief "<topic>" [--cortex <name>] [--limit <n>]
  exocortex note "<thought>" [--cortex <name>] [--agent <id>]
  exocortex sync [--cortex <name>]
  exocortex status [--cortex <name>]
  exocortex log <path> [--cortex <name>] [--limit <n>]
  exocortex lint [<path>] [--cortex <name>]
  exocortex mcp

Every command returns a JSON document on stdout. Failures speak JSON (CR-04)
naming the error, operation, path, and recovery hint.
`)
}

// splitArgs separates flag tokens from positional arguments regardless
// of order. Value-taking flags come from fs.VisitAll (non-bool Values).
// Unknown dash-tokens are left for flag.Parse to reject.
func splitArgs(args []string, fs *flag.FlagSet) (flags, positional []string) {
	valueFlags := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			return
		}
		valueFlags[f.Name] = true
	})
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
	profile := fs.String("profile", "", "validation profile: daybook | strict | okf (default daybook)")
	jprefix := fs.String("journal-prefix", "", "where note files land inside the cortex (default journal)")
	flags, pos := splitArgs(args, fs)
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("register", err.Error(), "run `exocortex help` for register usage"), nil
	}
	if len(pos) != 2 {
		return nil, inputErr("register", "register requires <name> <path>",
			"exocortex register <name> <path> [--vcs daybook|caller|none]"), nil
	}
	c, err := kernel.Register(pos[0], pos[1], *vcs, *profile, *jprefix)
	if err != nil {
		if conf, ok := err.(*kernel.Conflict); ok {
			return nil, conf, nil
		}
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
	flags, pos := splitArgs(args, fs)
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
	flags, pos := splitArgs(args, fs)
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
	limit := fs.Int("limit", 20, "max hits (default 20, max 100)")
	mode := fs.String("mode", qmd.DefaultMode, "retrieval mode: hybrid (default) | bm25 | vector")
	typeFilter := fs.String("type", "", "filter by content kind: decision | memo | session | note | scratch")
	flags, pos := splitArgs(args, fs)
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("search", err.Error(), "run `exocortex help` for search usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("search", `search requires one "<query>"`, `exocortex search "<query>"`), nil
	}
	collections := searchCollections(*cortex, *typeFilter)
	hits, err := qmd.Search(context.Background(), pos[0], collections, *mode, searchFetchLimit(*limit, *typeFilter))
	if err != nil {
		return nil, &kernel.Conflict{
			Code:      "search_unavailable",
			Operation: "search",
			Path:      pos[0],
			Hint:      "check that qmd is installed and the cortex has an indexed qmd collection of the same name",
			Detail:    map[string]any{"detail": err.Error()},
		}, nil
	}
	cs, err := loadRegistry()
	if err != nil && *typeFilter != "" {
		return nil, nil, err
	}
	projected, conf := projectSearchHits(hits, cs, *typeFilter, *limit)
	return projected, conf, nil

}

func searchCollections(cortex, typeFilter string) []string {
	if cortex != "" {
		return []string{cortex}
	}
	if strings.EqualFold(typeFilter, "session") {
		return sessionCollections
	}
	return nil
}

func searchFetchLimit(limit int, typeFilter string) int {
	if typeFilter == "" || limit >= 100 {
		return limit
	}
	fetchLimit := limit * 5
	if fetchLimit > 100 {
		return 100
	}
	return fetchLimit
}

func projectSearchHits(hits []qmd.Hit, cs []kernel.Cortex, typeFilter string, limit int) ([]map[string]any, *kernel.Conflict) {
	fetched, conf := hydrateHits(hits, cs)
	if conf != nil {
		return nil, conf
	}
	out := make([]map[string]any, 0)
	for i, h := range hits {
		entry, keep := projectSearchHit(h, cs, typeFilter, fetched[i])
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

// hydrateHits translates registered qmd URIs to one batched committed read.
// Non-URI and unregistered hits remain search-only projections.
func hydrateHits(hits []qmd.Hit, cs []kernel.Cortex) ([]*kernel.GetResult, *kernel.Conflict) {
	results := make([]*kernel.GetResult, len(hits))
	requests := make([]kernel.GetRequest, 0, len(hits))
	indexes := make([]int, 0, len(hits))
	for i, hit := range hits {
		collection, rel, ok := qmd.SplitURI(hit.File)
		if !ok || rel == "" || kernel.CortexNamed(cs, collection) == nil {
			continue
		}
		requests = append(requests, kernel.GetRequest{CortexName: collection, Path: rel})
		indexes = append(indexes, i)
	}
	outcomes := kernel.GetMany(cs, requests)
	for i, outcome := range outcomes {
		if outcome.Conflict != nil {
			return nil, outcome.Conflict
		}
		results[indexes[i]] = outcome.Result
	}
	return results, nil
}

func projectSearchHit(h qmd.Hit, cs []kernel.Cortex, typeFilter string, res *kernel.GetResult) (map[string]any, bool) {
	collection, rel, isURI := qmd.SplitURI(h.File)
	var (
		c  *kernel.Cortex
		fm map[string]any
	)
	if isURI {
		c = kernel.CortexNamed(cs, collection)
		if res != nil {
			fm = res.Frontmatter
		}
	}
	if typeFilter != "" && !orient.MatchType(kernel.JournalPrefix(c), rel, h.File, typeFilter, fm, res != nil) {
		return nil, false
	}
	entry := map[string]any{
		"docid":   h.DocID,
		"score":   h.Score,
		"file":    h.File,
		"title":   h.Title,
		"line":    h.Line,
		"context": h.Context,
		"snippet": h.Snippet,
	}
	if isURI {
		entry["cortex"] = collection
		entry["path"] = rel
		enrichSearchHit(entry, res)
	}
	return entry, true
}

func enrichSearchHit(entry map[string]any, res *kernel.GetResult) {
	if res == nil {
		return
	}
	if d, ok := res.Frontmatter["description"].(string); ok && d != "" && entry["context"] == "" {
		entry["context"] = d
	}
	if entry["title"] == "" {
		if t := extractTitle(res.Content); t != "" {
			entry["title"] = t
		}
	}
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

// sessionCollections is the search-only product policy for --type session
// when no --cortex is given. Classification of a hit as session lives in
// orient.MatchType.
var sessionCollections = []string{"omp-sessions", "claude-sessions", "pi-sessions"}

func cmdBrief(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	limit := fs.Int("limit", 3, "max canonical notes to summarize")
	flags, pos := splitArgs(args, fs)
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("brief", err.Error(), "run `exocortex help` for brief usage"), nil
	}
	if len(pos) != 1 {
		return nil, inputErr("brief", `brief requires one "<topic>"`, `exocortex brief "<topic>"`), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	topic := pos[0]
	var collections []string
	if *cortex != "" {
		collections = []string{*cortex}
	}
	hits, err := qmd.Search(context.Background(), topic, collections, qmd.DefaultMode, 15)
	if err != nil {
		return nil, &kernel.Conflict{
			Code:      "search_unavailable",
			Operation: "brief",
			Path:      topic,
			Hint:      "check that qmd is installed and indexing the cortex",
			Detail:    map[string]any{"detail": err.Error()},
		}, nil
	}
	notes, conf := collectBriefNotes(cs, hits, *limit)
	if conf != nil {
		return nil, conf, nil
	}
	return map[string]any{
		"topic":           topic,
		"canonical_notes": notes,
		"count":           len(notes),
	}, nil, nil
}

func collectBriefNotes(cs []kernel.Cortex, hits []qmd.Hit, limit int) ([]map[string]any, *kernel.Conflict) {
	fetched, conf := hydrateHits(hits, cs)
	if conf != nil {
		return nil, conf
	}
	var notes []map[string]any
	seen := map[string]bool{}
	for i, h := range hits {
		note, ok := briefNoteFromHit(cs, h, fetched[i], seen)
		if !ok {
			continue
		}
		notes = append(notes, note)
		if len(notes) >= limit {
			break
		}
	}
	return notes, nil
}

func briefNoteFromHit(cs []kernel.Cortex, h qmd.Hit, res *kernel.GetResult, seen map[string]bool) (map[string]any, bool) {
	cName, rel, ok := qmd.SplitURI(h.File)
	key := cName + "\x00" + rel
	if !ok || seen[key] {
		return nil, false
	}
	c := kernel.CortexNamed(cs, cName)
	if res == nil {
		return nil, false
	}
	if !orient.BriefOK(kernel.JournalPrefix(c), rel, h.File, res.Frontmatter) {
		return nil, false
	}
	seen[key] = true
	status, _ := res.Frontmatter["status"].(string)
	desc, _ := res.Frontmatter["description"].(string)
	tags, _ := res.Frontmatter["tags"].([]any)
	return map[string]any{
		"title":       h.Title,
		"path":        res.Path,
		"cortex":      res.Cortex,
		"status":      status,
		"description": desc,
		"takeaways":   extractTakeaways(res.Content),
		"tags":        tags,
		"revision":    res.Revision,
	}, true
}

func extractTakeaways(content string) []string {
	lines := strings.Split(content, "\n")
	if takeaways := takeawaysFromDecisionSection(lines); len(takeaways) > 0 {
		return takeaways
	}
	return takeawaysFallback(lines)
}

func takeawaysFromDecisionSection(lines []string) []string {
	var takeaways []string
	state := scanSkip
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "---") {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case isDecisionHeading(lower):
			state = scanDecision
			continue
		case state == scanDecision && strings.HasPrefix(trimmed, "## "):
			state = scanSkip
		}
		if state != scanDecision {
			continue
		}
		item, ok := bulletItem(trimmed)
		if !ok {
			continue
		}
		takeaways = append(takeaways, item)
		if len(takeaways) >= 4 {
			return takeaways
		}
	}
	return takeaways
}

type takeawayScan int

const (
	scanSkip takeawayScan = iota
	scanDecision
)

func isDecisionHeading(lower string) bool {
	return strings.HasPrefix(lower, "## decision") || strings.HasPrefix(lower, "## verdict") ||
		strings.HasPrefix(lower, "## model") || strings.HasPrefix(lower, "## architecture")
}

func takeawaysFallback(lines []string) []string {
	var takeaways []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if item, ok := bulletItem(trimmed); ok {
			takeaways = append(takeaways, item)
			if len(takeaways) >= 3 {
				break
			}
		}
	}
	return takeaways
}

func bulletItem(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, "- ") {
		return strings.TrimPrefix(trimmed, "- "), true
	}
	if strings.HasPrefix(trimmed, "* ") {
		return strings.TrimPrefix(trimmed, "* "), true
	}
	return "", false
}
func cmdLog(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	limit := fs.Int("limit", 50, "max entries")
	flags, pos := splitArgs(args, fs)
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
	flags, pos := splitArgs(args, fs)
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
	flags, pos := splitArgs(args, fs)
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

func cmdSync(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	flags, pos := splitArgs(args, fs)
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("sync", err.Error(), "run `exocortex help` for sync usage"), nil
	}
	if len(pos) != 0 {
		return nil, inputErr("sync", "sync takes no positional arguments",
			"exocortex sync [--cortex <name>]"), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Sync(context.Background(), cs, *cortex)
	if conf != nil && res != nil {
		if conf.Detail == nil {
			conf.Detail = map[string]any{}
		}
		conf.Detail["results"] = res
	}
	return res, conf, nil
}

func cmdStatus(args []string) (any, *kernel.Conflict, error) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cortex := commonFlags(fs)
	flags, pos := splitArgs(args, fs)
	if err := fs.Parse(flags); err != nil {
		return nil, inputErr("status", err.Error(), "run `exocortex help` for status usage"), nil
	}
	if len(pos) != 0 {
		return nil, inputErr("status", "status takes no positional arguments",
			"exocortex status [--cortex <name>]"), nil
	}
	cs, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	res, conf := kernel.Status(cs, *cortex)
	return res, conf, nil
}
