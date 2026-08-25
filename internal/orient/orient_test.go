package orient

import "testing"

func TestMatchTypeAndBriefTable(t *testing.T) {
	const (
		vault  = "meta/agents-board/memo"
		emma   = "daily"
		legacy = "journal"
	)

	type row struct {
		name    string
		prefix  string
		rel     string
		file    string
		fm      map[string]any
		filter  string
		want    bool
		briefOK bool
	}

	cases := []row{
		{
			name:    "vault memo prefix",
			prefix:  vault,
			rel:     "meta/agents-board/memo/2026-08-25/x.md",
			file:    "qmd://vault/meta/agents-board/memo/2026-08-25/x.md",
			fm:      map[string]any{"type": "memo"},
			filter:  "memo",
			want:    true,
			briefOK: false,
		},
		{
			name:    "vault memo is not a decision",
			prefix:  vault,
			rel:     "meta/agents-board/memo/2026-08-25/x.md",
			file:    "qmd://vault/meta/agents-board/memo/2026-08-25/x.md",
			fm:      map[string]any{"type": "note", "status": "active"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "custom journal prefix is memo",
			prefix:  emma,
			rel:     "daily/2026-08-25/n.md",
			file:    "qmd://emma/daily/2026-08-25/n.md",
			fm:      map[string]any{"type": "memo"},
			filter:  "memo",
			want:    true,
			briefOK: false,
		},
		{
			name:    "default journal prefix on legacy cortex",
			prefix:  legacy,
			rel:     "journal/2026-08-25/n.md",
			file:    "qmd://legacy/journal/2026-08-25/n.md",
			fm:      map[string]any{"type": "memo"},
			filter:  "memo",
			want:    true,
			briefOK: false,
		},
		{
			name:    "frontmatter memo outside prefix is not --type memo",
			prefix:  emma,
			rel:     "journal/x.md",
			file:    "qmd://emma/journal/x.md",
			fm:      map[string]any{"type": "memo"},
			filter:  "memo",
			want:    false,
			briefOK: true,
		},
		{
			name:    "conversations are sessions and not brief",
			prefix:  vault,
			rel:     "meta/conversations/2026-08-25.md",
			file:    "qmd://vault/meta/conversations/2026-08-25.md",
			fm:      map[string]any{"type": "conversation"},
			filter:  "session",
			want:    true,
			briefOK: false,
		},
		{
			name:    "jsonl is session",
			prefix:  vault,
			rel:     "raw/trace.jsonl",
			file:    "qmd://vault/raw/trace.jsonl",
			filter:  "session",
			want:    true,
			briefOK: false,
		},
		{
			name:    "unregistered session collection",
			rel:     "2026/08/25/sess.jsonl",
			file:    "qmd://omp-sessions/2026/08/25/sess.jsonl",
			filter:  "session",
			want:    true,
			briefOK: false,
		},
		{
			name:    "clippings excluded from decision and brief",
			prefix:  vault,
			rel:     "Clippings/book.md",
			file:    "qmd://vault/Clippings/book.md",
			fm:      map[string]any{"type": "clipping"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "reviews excluded from decision and brief",
			prefix:  vault,
			rel:     "meta/reviews/pr.md",
			file:    "qmd://vault/meta/reviews/pr.md",
			fm:      map[string]any{"type": "note", "status": "active"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "reading texts excluded",
			prefix:  vault,
			rel:     "resources/reading/book.md",
			file:    "qmd://vault/resources/reading/book.md",
			fm:      map[string]any{"type": "note", "status": "active"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "typed decision is decision and brief",
			prefix:  vault,
			rel:     "misty-step/kernel.md",
			file:    "qmd://vault/misty-step/kernel.md",
			fm:      map[string]any{"type": "decision", "status": "active"},
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "active note is decision",
			prefix:  vault,
			rel:     "projects/plan.md",
			file:    "qmd://vault/projects/plan.md",
			fm:      map[string]any{"type": "note", "status": "active"},
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "complete note is decision",
			prefix:  vault,
			rel:     "projects/done.md",
			file:    "qmd://vault/projects/done.md",
			fm:      map[string]any{"type": "note", "status": "complete"},
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "untyped note status is decision",
			prefix:  vault,
			rel:     "projects/bare.md",
			file:    "qmd://vault/projects/bare.md",
			fm:      map[string]any{"type": "note"},
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "superseded note is not decision and not brief",
			prefix:  vault,
			rel:     "projects/old.md",
			file:    "qmd://vault/projects/old.md",
			fm:      map[string]any{"type": "note", "status": "superseded"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "archived decision is not brief",
			prefix:  vault,
			rel:     "misty-step/old.md",
			file:    "qmd://vault/misty-step/old.md",
			fm:      map[string]any{"type": "decision", "status": "archived"},
			filter:  "decision",
			want:    true,
			briefOK: false,
		},
		{
			name:    "deprecated decision is not brief",
			prefix:  emma,
			rel:     "decisions/old.md",
			file:    "qmd://emma/decisions/old.md",
			fm:      map[string]any{"type": "decision", "status": "deprecated"},
			filter:  "decision",
			want:    true,
			briefOK: false,
		},
		{
			name:    "scratch is not decision even under projects/",
			prefix:  vault,
			rel:     "projects/wip.md",
			file:    "qmd://vault/projects/wip.md",
			fm:      map[string]any{"type": "scratch", "status": "active"},
			filter:  "decision",
			want:    false,
			briefOK: true,
		},
		{
			name:    "scratch filter matches type",
			prefix:  vault,
			rel:     "inbox/wip.md",
			file:    "qmd://vault/inbox/wip.md",
			fm:      map[string]any{"type": "scratch"},
			filter:  "scratch",
			want:    true,
			briefOK: true,
		},
		{
			name:    "untyped orientation path remains decision",
			prefix:  vault,
			rel:     "docs/adr/0001.md",
			file:    "qmd://vault/docs/adr/0001.md",
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "untyped superseded orientation path is not decision",
			prefix:  vault,
			rel:     "projects/dead-untyped.md",
			file:    "qmd://vault/projects/dead-untyped.md",
			fm:      map[string]any{"status": "superseded"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "untyped archived orientation path is not decision",
			prefix:  vault,
			rel:     "misty-step/dead-untyped.md",
			file:    "qmd://vault/misty-step/dead-untyped.md",
			fm:      map[string]any{"status": "archived"},
			filter:  "decision",
			want:    false,
			briefOK: false,
		},
		{
			name:    "second cortex decision by frontmatter only",
			prefix:  emma,
			rel:     "keep.md",
			file:    "qmd://emma/keep.md",
			fm:      map[string]any{"type": "decision", "status": "active"},
			filter:  "decision",
			want:    true,
			briefOK: true,
		},
		{
			name:    "second cortex untyped file is not memo",
			prefix:  emma,
			rel:     "keep.md",
			file:    "qmd://emma/keep.md",
			fm:      map[string]any{"type": "decision", "status": "active"},
			filter:  "memo",
			want:    false,
			briefOK: true,
		},
		{
			name:    "note filter matches type note",
			prefix:  vault,
			rel:     "misty-step/kernel.md",
			file:    "qmd://vault/misty-step/kernel.md",
			fm:      map[string]any{"type": "note", "status": "active"},
			filter:  "note",
			want:    true,
			briefOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchType(tc.prefix, tc.rel, tc.file, tc.filter, tc.fm)
			if got != tc.want {
				t.Fatalf("MatchType(%s)=%v want %v", tc.filter, got, tc.want)
			}
			if got := BriefOK(tc.prefix, tc.rel, tc.file, tc.fm); got != tc.briefOK {
				t.Fatalf("BriefOK=%v want %v", got, tc.briefOK)
			}
		})
	}
}
