package fm

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	headingRE = regexp.MustCompile(`^#{1,6}(?:[ \t]|$)`)
	catalogRE = regexp.MustCompile(`^\* \[[^]]+\]\([^)]+\)(?: (?:-|—) .+)?$`)
)

// ValidatePath validates one parsed document and reports whether it is an OKF catalog.
func ValidatePath(profile, rel string, d Document) (reserved bool, findings []Finding, err error) {
	if profile != "okf" {
		findings, err = Validate(profile, d)
		return false, findings, err
	}
	clean := path.Clean(rel)
	switch path.Base(clean) {
	case "index.md":
		return true, nil, validateIndex(d, clean == "index.md")
	case "log.md":
		return true, nil, validateLog(d)
	default:
		findings, err = validateDaybook(d)
		return false, findings, err
	}
}

func validateIndex(d Document, root bool) error {
	if d.Note.HasFM {
		if !root {
			return contract(errf("reserved_frontmatter", "nested index.md forbids frontmatter"))
		}
		if d.err != nil {
			return contract(errf("fm_unparseable", "invalid frontmatter YAML: %v", d.err))
		}
		var extra []string
		for key := range d.Map {
			if key != "okf_version" {
				extra = append(extra, key)
			}
		}
		if len(extra) != 0 {
			sort.Strings(extra)
			return contract(errf("unknown_keys", "root index extra keys: %s", strings.Join(extra, ", ")))
		}
	}
	body := string(d.Note.Raw)
	if d.Note.HasFM {
		body = d.Note.Body
	}
	var heading, catalog bool
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if headingRE.MatchString(line) {
			heading = true
		}
		if strings.HasPrefix(line, "* ") {
			if !catalogRE.MatchString(line) {
				return contract(errf("index_format", "catalog item must match * [Title](url)"))
			}
			catalog = true
		}
	}
	if !heading {
		return contract(errf("index_format", "index.md needs a heading"))
	}
	if !catalog {
		return contract(errf("index_format", "index.md needs * [Title](url)"))
	}
	return nil
}

func validateLog(d Document) error {
	if d.Note.HasFM {
		return contract(errf("reserved_frontmatter", "log.md must not have frontmatter"))
	}
	var previous string
	for _, line := range strings.Split(string(d.Note.Raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "##") || len(line) > 2 && line[2] != ' ' && line[2] != '\t' {
			continue
		}
		date := strings.TrimSpace(line[2:])
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return contract(errf("log_heading", "log.md date must be YYYY-MM-DD, got %q", date))
		}
		if previous != "" && previous < date {
			return contract(errf("log_order", "log.md dates must be newest-first"))
		}
		previous = date
	}
	if previous == "" {
		return contract(errf("log_dates", "log.md needs a dated heading"))
	}
	return nil
}
