# ADR-0001: Exocortex is a Foundation-shaped memory kernel, pluggable at the edge

Status: Accepted (2026-08-21)
Deciders: operator (phaedrus) with ox-alpha research/brainstorm session

## Context

Chroma announced Foundation: a versioned, access-controlled wiki that AI
agents write to and read from across an organization, with concurrency
control, versioning, lineage tracking, and ingestion from Notion, Google
Drive, and coding-agent sessions. The operator asked how to imitate it for
the Misty Step fleet, either in daybook or as a new project, wired into
omp-config so every agent can use it.

The fleet already runs most of Foundation's data model: daybook is a
git-versioned markdown wiki; QMD provides hybrid retrieval over the wiki and
raw agent session collections; concurrent-agent discipline exists as prose
doctrine. The real gaps are a mechanical agent write path, a concurrency
mechanism, and a uniform CLI+MCP surface across harnesses. The name
`foundation` is taken by the omp-config agentic-baseline skill.

## Decision

Build `exocortex`: a small CLI carrying its own agent skill, one binary with
CLI (`--json`) and stdio MCP faces, in its own misty-step repo, skeleton
first.

Pluggable at the edge, never in the store:

- **Cortices** — writable markdown+git corpora registered for full
  read/write/search/lint/lineage. Daybook is cortex #1. No content port.
- **Feeds** — adapters that ingest foreign sources into a cortex as
  compiled, provenance-stamped notes with `source:` lineage. Raw sources
  stay on disk; write-back per adapter is optional.

VCS lifecycle is a per-cortex policy; generic `put` never hard-codes
commits. The daybook driver runs `pull --rebase --autostash`, stages touched
paths only, commits, pushes. Fleet delivery is CLI-first (universal
baseline); MCP is registered per harness in slice 2 (omp-config covers Oh My
Pi only).

## Alternatives considered

- **Port daybook content into a kernel-owned store** — rejected: abandons
  the git-markdown substrate (portable, diffable, human-owned), duplicates
  storage, big-bang migration (DE-06).
- **Fully abstracted storage backends now** — rejected: rule of three
  (DE-08); no second real consumer exists (DE-01).
- **Standalone memory service** (daemon + own DB) — rejected for v0: no
  multi-host contention to justify an operated service; correct escalation
  path if that changes.
- **Build on Chroma OSS/cloud** — rejected: dependency weight (RS-03) at
  this corpus size; alpha cloud-gated CLI; git+QMD already deliver the
  capability.

## Consequences

- The fleet gets a mechanical write contract (frontmatter floor, provenance,
  hash preconditions, conflicts as data) replacing trust in prose rules.
- Daybook's concurrent-agents doctrine becomes enforced machinery via the
  daybook VCS driver.
- Slice 2 carries per-harness MCP registration; until then non-omp harnesses
  use the CLI.
- Open items carried in SPEC.md: language choice, claim/lease semantics,
  feed priority, QMD embedding backfill.
