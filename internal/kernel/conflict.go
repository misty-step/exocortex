package kernel

import "fmt"

// Class is the transport-neutral semantic class of a Conflict.
// Faces map class; they do not switch on code names.
type Class int

const (
	// ClassOperation is a data conflict against current cortex state
	// (exists, revision_conflict, not_found, …).
	ClassOperation Class = iota + 1
	// ClassInput is a caller usage or registration input failure.
	ClassInput
	// ClassInternal is an unexpected kernel failure (internal_error).
	ClassInternal
	// ClassUnavailable is an expected dependency or publisher outage
	// (search_unavailable, writer_unavailable, cortex_unavailable,
	// log_unavailable).
	ClassUnavailable
)

// SPEC input-class and internal_error keep their named classes.
// Named availability codes are ClassUnavailable. Every other current
// code stays ClassOperation so CLI exits remain 1 unless SPEC lists
// them as exit 2.
var codeClass = map[string]Class{
	"invalid_input":       ClassInput,
	"unknown_command":     ClassInput,
	"registration_failed": ClassInput,
	"payload_unreadable":  ClassInput,
	"internal_error":      ClassInternal,
	"search_unavailable":  ClassUnavailable,
	"writer_unavailable":  ClassUnavailable,
	"cortex_unavailable":  ClassUnavailable,
	"log_unavailable":     ClassUnavailable,
}

// Conflict is an operation failure returned as data (CR-04): a stable
// machine-readable code, the operation and input it names, structured
// detail, and a recovery hint.
type Conflict struct {
	Code      string
	Operation string
	Path      string
	Detail    map[string]any
	Hint      string
}

func (c *Conflict) Error() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s (%s op=%s path=%s)", c.Code, c.Code, c.Operation, c.Path)
}

// Class reports the semantic class owned by this conflict. Unknown
// codes default to ClassOperation, matching the historical CLI default
// of exit 1.
func (c *Conflict) Class() Class {
	if c == nil {
		return ClassInternal
	}
	if class, ok := codeClass[c.Code]; ok {
		return class
	}
	return ClassOperation
}

// Body is the pinned CR-04 JSON object. Empty operation/path are
// omitted; Detail keys flatten into the object. This is the only
// serializer: CLI and MCP must not rebuild the map.
func (c *Conflict) Body() map[string]any {
	if c == nil {
		return map[string]any{"error": "internal_error", "hint": "retry"}
	}
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

func conflict(code, op, path, hint string, detail map[string]any) *Conflict {
	return &Conflict{Code: code, Operation: op, Path: path, Hint: hint, Detail: detail}
}
