package fm

import (
	"bufio"
	"bytes"
	"path"
	"sort"
	"strings"
	"time"
)

// ValidatePath applies the named profile to raw at cortex-relative rel.
// Profile okf uses reserved index.md / log.md formats; other profiles
// apply the note type floor to every path.
func ValidatePath(profile, rel string, raw []byte) ([]Finding, error) {
	if profile == "okf" {
		kind, rootIndex := ReservedMarkdown(rel)
		switch kind {
		case "index":
			return ValidateIndex(raw, rootIndex)
		case "log":
			return ValidateLog(raw)
		}
	}
	return Validate(profile, ParseDocument(raw))
}

// ValidateIndex checks an OKF reserved index.md.
// Nested indexes must have no frontmatter. The bundle-root index.md MAY
// carry only okf_version. Body requires at least one ATX heading and at
// least one "* [Title](url)" catalog bullet.
func ValidateIndex(raw []byte, root bool) ([]Finding, error) {
	d := ParseDocument(raw)
	if d.Note.HasFM {
		if !root {
			return nil, contract(errf("reserved_frontmatter", "nested index.md must not have frontmatter"))
		}
		if d.err != nil {
			return nil, contract(errf("fm_unparseable", "frontmatter is not parseable YAML: %v", d.err))
		}
	}
	if err := checkIndexBody(markdownBody(d, raw)); err != nil {
		return nil, err
	}
	if !d.Note.HasFM {
		return nil, nil
	}
	var unknown []string
	for k := range d.Map {
		if k != "okf_version" {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil, nil
	}
	sort.Strings(unknown)
	return []Finding{warnf("unknown_keys", "root index.md extra keys: %s", strings.Join(unknown, ", "))}, nil
}

func checkIndexBody(body string) error {
	var heading, link bool
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if isATXHeading(line) {
			heading = true
			continue
		}
		ok, bullet := isLinkBullet(line)
		if !bullet {
			continue
		}
		if !ok {
			return contract(errf("index_format", "index.md list items must be * [Title](url) - description"))
		}
		link = true
	}
	if !heading {
		return contract(errf("index_format", "index.md needs at least one markdown heading"))
	}
	if !link {
		return contract(errf("index_format", "index.md needs at least one * [Title](url) bullet"))
	}
	return nil
}

// ValidateLog checks an OKF reserved log.md: no frontmatter, every ##
// heading is an ISO date, headings newest-first, at least one date heading.
func ValidateLog(raw []byte) ([]Finding, error) {
	d := ParseDocument(raw)
	if d.Note.HasFM {
		return nil, contract(errf("reserved_frontmatter", "log.md must not have frontmatter"))
	}
	var dates []string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		if _, err := time.Parse("2006-01-02", rest); err != nil {
			return nil, contract(errf("log_heading", "log.md ## headings must be ISO YYYY-MM-DD, got %q", rest))
		}
		dates = append(dates, rest)
	}
	if len(dates) == 0 {
		return nil, contract(errf("log_dates", "log.md needs at least one ## YYYY-MM-DD heading"))
	}
	for i := 1; i < len(dates); i++ {
		if dates[i-1] < dates[i] {
			return nil, contract(errf("log_order", "log.md date headings must be newest-first"))
		}
	}
	return nil, nil
}

func markdownBody(d Document, raw []byte) string {
	if d.Note.HasFM {
		return d.Note.Body
	}
	return string(raw)
}

func isATXHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return false
	}
	return n == len(line) || line[n] == ' ' || line[n] == '\t'
}

func isLinkBullet(line string) (ok, bullet bool) {
	var item string
	switch {
	case strings.HasPrefix(line, "* "):
		item = strings.TrimSpace(line[2:])
	case strings.HasPrefix(line, "- "):
		item = strings.TrimSpace(line[2:])
	default:
		return false, false
	}
	if len(item) < 5 || item[0] != '[' {
		return false, true
	}
	rb := strings.IndexByte(item, ']')
	if rb < 2 || rb+1 >= len(item) || item[rb+1] != '(' {
		return false, true
	}
	closeParen := strings.IndexByte(item[rb+2:], ')')
	if closeParen < 0 {
		return false, true
	}
	url := strings.TrimSpace(item[rb+2 : rb+2+closeParen])
	if url == "" {
		return false, true
	}
	rest := strings.TrimSpace(item[rb+2+closeParen+1:])
	if rest == "" || strings.HasPrefix(rest, "- ") || strings.HasPrefix(rest, "— ") {
		return true, true
	}
	return false, true
}

// ReservedMarkdown reports whether rel is an OKF reserved basename.
func ReservedMarkdown(rel string) (kind string, rootIndex bool) {
	base := path.Base(strings.ReplaceAll(rel, "\\", "/"))
	switch base {
	case "index.md":
		slash := path.Clean(strings.ReplaceAll(rel, "\\", "/"))
		return "index", slash == "index.md"
	case "log.md":
		return "log", false
	default:
		return "", false
	}
}
