# AGENTS.md — Exocortex kernel

One binary (`exocortex`) giving every Misty Step agent a uniform
read/write/search interface over registered knowledge corpora. Spec-seeded
2026-08-21; nothing is implemented yet. The v0 build job is Powder card
`exocortex-v0`.

## Operator-set decisions (2026-08-21)

Source artifact: daybook `misty-step/exocortex-kernel.md`.

- Small CLI that carries its own agent skill; two faces from one binary:
  CLI (`--json`) and stdio MCP server.
- Pluggable at the edge, never in the store. **Cortices** are writable
  markdown+git corpora with full read/write; **feeds** are adapters that
  ingest foreign sources into a cortex as compiled, provenance-stamped notes
  with `source:` lineage. No content port; no storage backends.
- Daybook is cortex #1. Skeleton first: `register/put/get/search/log/lint`.
- VCS lifecycle is a per-cortex policy; generic `put` never hard-codes
  commits. The daybook driver runs `pull --rebase --autostash`, stages
  touched paths only, commits, pushes (daybook's own repo contract). Other
  cortices declare caller-owned commits or no VCS.
- Fleet delivery: the CLI is the universal baseline (any harness can exec a
  shell command). MCP is registered per harness; slice 2 owns the omp /
  Claude Code / Codex / opencode / goose / pi install targets.

## Open decisions

- Language: Go recommended (velocity, single-binary fleet distribution);
  Rust equally acceptable (canon CR-01). Decide at scaffold.
- Claim/lease semantics when concurrency mechanization lands.
- Feed priority order (harness session logs proposed first).
- Optional `exo` binary alias.

## Contracts

- Write path: frontmatter floor validation, provenance stamping
  (agent/time/source), expected-hash precondition on put, conflicts returned
  as data — never swallowed.
- Search shells out to `qmd --format json`; the kernel never re-implements
  retrieval.
- `--json` everywhere (CR-02); errors name operation, input, expected state
  (CR-04).

## Working rules

- Trunk-based on `master`; semantic commits (`type(scope): subject`).
- Never force-push. Stage only files you touched; concurrent agents work in
  this repo.
- Org defaults apply: Go or Rust (CR-01), minimal docs surface (DC-01), ADRs
  immutable once accepted (DC-02).
- History note: this repo previously hosted an "evidence core" concept
  (replaced 2026-08-21 by operator decision). Old tree is recoverable from
  git history; do not restore it without operator instruction.
