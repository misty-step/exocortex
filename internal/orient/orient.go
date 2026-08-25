// Package orient classifies search hits for --type and brief.
// Journal location is cortex policy (kernel.JournalPrefix); this
// package owns the shared exclusion, session, frontmatter, and
// lifecycle mechanics.
package orient

import (
	"fmt"
	"path/filepath"
	"strings"
)

// noisePrefixes is the single exclusion list for decision search and
// brief. The cortex journal prefix is always treated as memo and is
// not listed here.
var noisePrefixes = []string{
	"meta/conversations/",
	"meta/reviews/",
	"Clippings/",
	"resources/reading/",
}

// untypedDecisionPrefixes preserve orientation search for files that
// have no frontmatter type. Typed non-decision notes do not fall through.
var untypedDecisionPrefixes = []string{
	"projects/",
	"misty-step/",
	"docs/adr/",
	"standards/",
}

// MatchType reports whether a search hit satisfies --type. Memo matching
// uses journalPrefix, not a compiled Daybook path.
func MatchType(journalPrefix, rel, fileURI, filter string, fm map[string]any) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return true
	}
	typ := fmString(fm, "type")
	status := fmString(fm, "status")
	switch f {
	case "session":
		return isSession(rel, fileURI)
	case "memo":
		return isMemo(journalPrefix, rel)
	case "decision":
		if isNoise(journalPrefix, rel, fileURI) {
			return false
		}
		if typ != "" {
			return decisionType(typ, status)
		}
		return decisionPath(rel)
	default:
		return typ != "" && strings.EqualFold(typ, f)
	}
}

// BriefOK reports whether a hit may appear in brief. Brief keeps a
// looser type policy than --type decision: any live non-noise note.
func BriefOK(journalPrefix, rel, fileURI string, fm map[string]any) bool {
	if isNoise(journalPrefix, rel, fileURI) {
		return false
	}
	return liveStatus(fmString(fm, "status"))
}

func isMemo(journalPrefix, rel string) bool {
	return pathUnder(rel, journalPrefix)
}

func isSession(rel, fileURI string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, ".jsonl") || strings.Contains(rel, "conversations/") {
		return true
	}
	return strings.Contains(fileURI, "sessions")
}

func isNoise(journalPrefix, rel, fileURI string) bool {
	if isMemo(journalPrefix, rel) || isSession(rel, fileURI) {
		return true
	}
	rel = filepath.ToSlash(rel)
	for _, p := range noisePrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

func decisionType(typ, status string) bool {
	if strings.EqualFold(typ, "decision") {
		return true
	}
	if !strings.EqualFold(typ, "note") {
		return false
	}
	switch strings.ToLower(status) {
	case "", "active", "complete":
		return true
	default:
		return false
	}
}

func decisionPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, p := range untypedDecisionPrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

func liveStatus(status string) bool {
	switch strings.ToLower(status) {
	case "deprecated", "archived", "superseded":
		return false
	default:
		return true
	}
}

func pathUnder(rel, prefix string) bool {
	rel = filepath.ToSlash(rel)
	prefix = strings.Trim(filepath.ToSlash(prefix), "/")
	if prefix == "" || rel == "" {
		return false
	}
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}

func fmString(fm map[string]any, key string) string {
	if fm == nil {
		return ""
	}
	v, ok := fm[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
