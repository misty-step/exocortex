// Package okf validates the optional OKF v0.2 metadata families —
// trust/lifecycle events, stale_after, and provenance — against the
// structural rules of the Open Knowledge Format v0.2 spec (SPEC §5, §10).
// Violations are neutral data; level policy (error vs warning) is the
// calling profile's decision.
package okf

import (
	"fmt"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Violation is one structural rule breach in OKF v0.2 metadata.
type Violation struct {
	Rule    string
	Message string
}

// Validate checks the optional OKF v0.2 fields of a parsed frontmatter
// document node. Absent fields are meaningful (SPEC §5) and never
// flagged; malformed present fields each yield one violation.
func Validate(root *yaml.Node) []Violation {
	m := topMapping(root)
	if m == nil {
		return nil
	}
	var vs []Violation
	vs = append(vs, validateGenerated(m)...)
	vs = append(vs, validateVerified(m)...)
	vs = append(vs, validateStaleAfter(m)...)
	vs = append(vs, validateProvenance(m)...)
	return vs
}

// validTimestamp reports whether value parses as RFC3339. The RFC3339
// layout enforces the explicit UTC offset OKF §5 requires for every
// timestamp key; the offset itself may be any valid offset, not just
// zero.
func validTimestamp(value string) bool {
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func violationf(rule, format string, args ...any) Violation {
	return Violation{Rule: rule, Message: fmt.Sprintf(format, args...)}
}

func validateGenerated(root *yaml.Node) []Violation {
	generated, ok := mappingValue(root, "generated")
	if !ok {
		return nil
	}
	return validateEvent("generated", generated, false)
}

func validateVerified(root *yaml.Node) []Violation {
	verified, ok := mappingValue(root, "verified")
	if !ok {
		return nil
	}
	verified = unwrapNode(verified)
	if verified == nil {
		return []Violation{{Rule: "verified_format", Message: "verified must be a mapping or list of mappings"}}
	}
	switch verified.Kind {
	case yaml.MappingNode:
		return validateEventFields("verified", "verified", verified, true)
	case yaml.SequenceNode:
		var vs []Violation
		for i, event := range verified.Content {
			vs = append(vs, validateEventFields(
				fmt.Sprintf("verified[%d]", i), "verified", unwrapNode(event), true,
			)...)
		}
		return vs
	default:
		return []Violation{{Rule: "verified_format", Message: "verified must be a mapping or list of mappings"}}
	}
}

func validateStaleAfter(root *yaml.Node) []Violation {
	staleAfter, ok := mappingValue(root, "stale_after")
	if !ok {
		return nil
	}
	value, scalar := nodeScalar(staleAfter)
	if scalar && validTimestamp(value) {
		return nil
	}
	return []Violation{violationf("stale_after_format",
		"stale_after %q must be an RFC3339 datetime with an explicit UTC offset", value)}
}

func validateProvenance(root *yaml.Node) []Violation {
	provenance, ok := mappingValue(root, "provenance")
	if !ok {
		return nil
	}
	provenance = unwrapNode(provenance)
	if provenance == nil || provenance.Kind != yaml.MappingNode {
		return []Violation{{Rule: "provenance_format", Message: "provenance must be a mapping"}}
	}
	at, present := mappingValue(provenance, "at")
	if !present {
		return nil
	}
	value, scalar := nodeScalar(at)
	if scalar && validTimestamp(value) {
		return nil
	}
	return []Violation{violationf("provenance_at_format",
		"provenance.at %q must be an RFC3339 datetime with an explicit UTC offset", value)}
}

func validateEvent(field string, event *yaml.Node, requireAt bool) []Violation {
	event = unwrapNode(event)
	if event == nil || event.Kind != yaml.MappingNode {
		return []Violation{violationf(field+"_format", "%s must be a mapping", field)}
	}
	// generated.at is optional when the production time is unknown.
	return validateEventFields(field, field, event, requireAt)
}

func validateEventFields(field, ruleBase string, event *yaml.Node, requireAt bool) []Violation {
	var vs []Violation
	if event == nil || event.Kind != yaml.MappingNode {
		return []Violation{violationf(ruleBase+"_format", "%s must be a mapping", field)}
	}
	by, ok := mappingValue(event, "by")
	if !ok {
		vs = append(vs, violationf(ruleBase+"_by_missing", "%s.by is required", field))
	} else if value, scalar := nodeScalar(by); !scalar || !validActor(value) {
		value, _ := nodeScalar(by)
		vs = append(vs, violationf(ruleBase+"_by_format",
			"%s.by %q must be <producer>/<version>, human:<id>, or process:<id>", field, value))
	}
	at, ok := mappingValue(event, "at")
	if !ok {
		if requireAt {
			vs = append(vs, violationf(ruleBase+"_at_missing", "%s.at is required", field))
		}
	} else if value, scalar := nodeScalar(at); !scalar || !validTimestamp(value) {
		value, _ := nodeScalar(at)
		vs = append(vs, violationf(ruleBase+"_at_format",
			"%s.at %q must be an RFC3339 datetime with an explicit UTC offset", field, value))
	}
	return vs
}

func validActor(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(strings.Fields(value)) != 1 {
		return false
	}
	for _, prefix := range []string{"human:", "process:"} {
		if strings.HasPrefix(value, prefix) {
			return len(value) > len(prefix)
		}
	}
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func topMapping(root *yaml.Node) *yaml.Node {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	m := unwrapNode(root.Content[0])
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	return m
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	return mappingValueWithMerges(unwrapNode(mapping), key)
}

// mappingValueWithMerges resolves key with yaml.v3 merge-key semantics:
// plain "<<", the long-form "!!merge" tag, and the explicit non-specific
// tag all merge; quoted and aliased keys are ordinary keys.
func mappingValueWithMerges(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	var merges []*yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		rawName := mapping.Content[i]
		name := unwrapNode(rawName)
		if name == nil || name.Kind != yaml.ScalarNode {
			continue
		}
		if name.Value == key {
			return mapping.Content[i+1], true
		}
		if isMergeKey(rawName) {
			merges = append(merges, mapping.Content[i+1])
		}
	}
	for _, merged := range merges {
		if value, ok := mergedMappingValue(merged, key); ok {
			return value, true
		}
	}
	return nil, false
}

func mergedMappingValue(merged *yaml.Node, key string) (*yaml.Node, bool) {
	merged = unwrapNode(merged)
	if merged == nil {
		return nil, false
	}
	if merged.Kind == yaml.MappingNode {
		return mappingValueWithMerges(merged, key)
	}
	if merged.Kind != yaml.SequenceNode {
		return nil, false
	}
	for _, item := range merged.Content {
		if value, ok := mergedMappingValue(item, key); ok {
			return value, true
		}
	}
	return nil, false
}

func isMergeKey(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Value == "<<" &&
		(n.Tag == "" || n.Tag == "!" || n.ShortTag() == "!!merge")
}

func unwrapNode(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func nodeScalar(n *yaml.Node) (string, bool) {
	n = unwrapNode(n)
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		return "", false
	}
	return n.Value, true
}
