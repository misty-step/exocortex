# ADR-0003: Journal prefix is a per-cortex field; daybook notes land on the agent board

Status: Accepted (2026-08-22)
Deciders: operator (phaedrus) with ox-alpha implementation session
Supersedes: the journal-path clause of ADR-0002 decision 1 (only that clause)

## Context

ADR-0002 placed `note` files at `journal/YYYY-MM-DD/<ulid>-<agent>.md`.
Daybook's binding namespace doctrine (daybook `AGENTS.md`) reserves
`journal/YYYY/MM/DD.md` for human-authored entries and designates
`meta/agents-board/` as THE agent message board for fleet knowledge —
a charter written explicitly in response to the OpenAI/HuggingFace
swarm-coordination incident. Hardcoding `journal/` in the kernel would
make every fleet micro-memory violate daybook's namespace and the
"no new top-level folders" rule.

## Decision

The note destination directory becomes a per-cortex registry field,
`journal_prefix` (optional, default `journal`). Daybook registers with
`journal_prefix: meta/agents-board/memo`, so fleet micro-memories land
inside the existing agent-board subtree; reflection folds notable memos
into indexed board threads per the board charter. Other cortices keep
the plain default. SPEC registry shape and SKILL.md updated accordingly.

## Consequences

- Human journal space is structurally unreachable from `note`.
- The kernel stays venue-agnostic; namespace policy lives in cortex
  registration, where it belongs.
- ADR-0002 stands unmodified as accepted; this ADR records the delta.
