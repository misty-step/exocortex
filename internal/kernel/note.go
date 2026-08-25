package kernel

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// NoteInput is one journal micro-memory: a single thought worth keeping.
type NoteInput struct {
	CortexName string
	Text       string
	Agent      string
	Via        string // "cli" | "mcp"
}

// Note writes a journal micro-memory as an IMMUTABLE file,
// <journal_prefix>/YYYY-MM-DD/<ulid>-<agent>.md, through the standard
// put pipeline (create-only; the path is unique by construction). One
// file per memory keeps concurrent writers out of a whole-file CAS
// hotspot: no two agents ever touch the same revision. Reflection
// compiles the journal later; the journal itself is append-only.

func Note(ctx context.Context, cs []Cortex, in NoteInput) (*PutResult, *Conflict) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, conflict("empty_note", "note", "",
			"pass the thought worth remembering as the argument", nil)
	}
	c, err := ResolveCortex(cs, in.CortexName)
	if err != nil {
		return nil, conflict("resolve_failed", "note", in.CortexName,
			"register a cortex or pass --cortex <name>",
			map[string]any{"detail": err.Error()})
	}
	prefix := effectiveJournalPrefix(c)

	// Journal captures must never lose a memory to remote movement.
	// Paths are unique, so every cross-host push race is benign: after
	// the loser converges onto the winner, a fresh attempt (new ULID)
	// finds its path free and lands normally. Intermediate attempts
	// keep the payload in memory here; only a terminal conflict gets it
	// preserved once into the conflict body.
	var last *Conflict
	var payload []byte
	for range 3 {
		now := time.Now().UTC()
		rel := fmt.Sprintf("%s/%s/%s-%s.md", prefix, now.Format("2006-01-02"), newULID(now), slug(agentID(in.Agent)))
		payload = []byte(fmt.Sprintf("---\ntype: memo\ncreated: %s\n---\n\n%s\n", now.Format(time.RFC3339), text))
		res, conf := Put(ctx, cs, PutInput{
			CortexName: c.Name,
			Path:       rel,
			Payload:    payload,
			Agent:      in.Agent,
			Via:        in.Via,
			OwnPayload: true,
		})
		if conf == nil {
			return res, nil
		}
		switch conf.Code {
		case "exists", "revision_conflict":
			last = conf
			continue
		}
		// Non-retryable (foreign state, refresh failure): the thought
		// must survive anyway — production hit this on daybook's dirty
		// heartbeat within minutes of first use.
		preservePayload(PutInput{Payload: payload, OwnPayload: false}, conf)
		return nil, conf
	}
	if last != nil {
		preservePayload(PutInput{Payload: payload, OwnPayload: false}, last)
	}
	return nil, last
}

// newULID renders a ULID-shaped identifier: 48-bit millisecond
// timestamp + 80-bit crypto randomness, Crockford base32. Lexical sort
// is chronological.
func newULID(now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	b[0], b[1], b[2], b[3], b[4], b[5] = byte(ms>>40), byte(ms>>32), byte(ms>>24), byte(ms>>16), byte(ms>>8), byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		// Time-only fallback still unique within a millisecond window.
		ms2 := uint64(now.UnixNano())
		b[6], b[7], b[8], b[9], b[10], b[11] = byte(ms2>>40), byte(ms2>>32), byte(ms2>>24), byte(ms2>>16), byte(ms2>>8), byte(ms2)
	}
	const enc = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	out := make([]byte, 0, 26)
	var acc uint64
	var bits uint
	for _, by := range b {
		acc = acc<<8 | uint64(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out = append(out, enc[(acc>>bits)&31])
		}
	}
	if bits > 0 { // ULID pads the trailing 3 bits with zeros -> 26 symbols
		out = append(out, enc[(acc<<(5-bits))&31])
	}
	return string(out)
}

// slug reduces an agent id to filename-safe characters.
func slug(agent string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(agent) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
