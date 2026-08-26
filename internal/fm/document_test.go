package fm

import (
	"bytes"
	"testing"
	"time"
)

// One parsed document owns split bytes, decoded map, and lexical
// scalars. Validate and Scalar reuse it; splice stays line-based.
func TestDocumentEquivalence(t *testing.T) {
	at := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		raw     string
		profile string
	}{
		{"type-only", "---\ntype: decision\n---\nbody\n", "daybook"},
		{"unquoted-created", "---\ntype: note\ncreated: 2026-08-21T00:00:00Z\n---\nbody\n", "daybook"},
		{"date-only-created", "---\ntype: x\ncreated: 2026-08-21\n---\n", "daybook"},
		{"date-only-strict", "---\ntype: a\nstatus: b\ncreated: 2026-08-21\ndescription: d\ntags: [x]\n---\n", "strict"},
		{"strict-full", "---\ntype: a\nstatus: b\ncreated: 2026-08-21T00:00:00Z\ndescription: d\ntags: [x]\n---\n", "strict"},
		{"rich", richNote, "daybook"},
		{"memo-quiet", "---\ntype: memo\n---\none line\n", "daybook"},
		{"missing-type", "---\nstatus: active\n---\nbody\n", "daybook"},
		{"plain", "plain body\n", "daybook"},
		{"unparseable", "---\n: [unclosed\n---\n", "daybook"},
		{"empty-fm", "---\n---\nbody\n", "daybook"},
		{"sequence-fm", "---\n- a\n- b\n---\n", "daybook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.raw)
			n := Split(raw)
			doc := ParseDocument(raw)

			if doc.Note.HasFM != n.HasFM || doc.Note.FMText != n.FMText || doc.Note.Body != n.Body {
				t.Fatalf("split diverge: doc=%+v note=%+v", doc.Note, n)
			}
			if !bytes.Equal(doc.Note.Raw, raw) {
				t.Fatal("document must own the original bytes")
			}

			if !n.HasFM {
				if doc.Err() != nil {
					t.Fatalf("missing frontmatter is not a parse error: %v", doc.Err())
				}
				if doc.Map != nil {
					t.Fatalf("missing frontmatter leaked map: %v", doc.Map)
				}
			} else if doc.Err() != nil && doc.Map != nil {
				t.Fatalf("unparseable document leaked map: %v", doc.Map)
			}

			wantF, wantErr := Validate(tc.profile, doc)
			gotF, gotErr := Validate(tc.profile, doc)
			switch {
			case wantErr == nil && gotErr != nil:
				t.Fatalf("reused Validate err %v, first nil", gotErr)
			case wantErr != nil && gotErr == nil:
				t.Fatalf("reused Validate nil, first err %v", wantErr)
			case wantErr != nil && gotErr != nil && wantErr.Error() != gotErr.Error():
				t.Fatalf("reused Validate errors diverge: %q vs %q", wantErr, gotErr)
			}
			if !findingsEqual(wantF, gotF) {
				t.Fatalf("reused Validate findings diverge:\n first=%+v\n second=%+v", wantF, gotF)
			}

			spliced := SpliceProvenance(doc.Note.Raw, Provenance{Agent: "ox-alpha", At: at, Via: "mcp"})
			if tc.name == "rich" {
				for _, keep := range []string{
					"status: active # keep this comment",
					"created: 2020-01-01T00:00:00Z",
					"  nested: true",
					"Body text with ---",
				} {
					if !bytes.Contains(spliced, []byte(keep)) {
						t.Fatalf("splice after ParseDocument lost %q:\n%s", keep, spliced)
					}
				}
			}
			if n.HasFM && doc.Err() == nil {
				if nonProvenanceLines(string(StripProvenance(spliced))) != nonProvenanceLines(string(StripProvenance(raw))) {
					t.Fatalf("strip after splice must restore non-provenance bytes")
				}
			}
		})
	}
}

func findingsEqual(a, b []Finding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDocumentLexicalCreatedVsDecodedTime(t *testing.T) {
	raw := []byte("---\ntype: x\ncreated: 2026-08-21T00:00:00Z\n---\n")
	doc := ParseDocument(raw)
	if doc.Err() != nil {
		t.Fatal(doc.Err())
	}
	got, ok := doc.Scalar("created")
	if !ok || got != "2026-08-21T00:00:00Z" {
		t.Fatalf("lexical created = (%q,%v)", got, ok)
	}
	if _, isTime := doc.Map["created"].(time.Time); !isTime {
		t.Fatalf("decoded created type %T, want time.Time", doc.Map["created"])
	}

	dateOnly := ParseDocument([]byte("---\ntype: x\ncreated: 2026-08-21\n---\n"))
	lex, ok := dateOnly.Scalar("created")
	if !ok || lex != "2026-08-21" {
		t.Fatalf("date-only lexical = (%q,%v)", lex, ok)
	}
}

func TestDocumentReuseDoesNotNeedResplit(t *testing.T) {
	raw := []byte(richNote)
	doc := ParseDocument(raw)
	fs, err := Validate("daybook", doc)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(fs, "unknown_keys") {
		t.Fatalf("expected unknown_keys warning: %+v", fs)
	}
	created, ok := doc.Scalar("created")
	if !ok || created != "2020-01-01T00:00:00Z" {
		t.Fatalf("reused document lost created: (%q,%v)", created, ok)
	}
	if doc.Map["type"] != "decision" {
		t.Fatalf("reused document lost type: %v", doc.Map["type"])
	}
}
