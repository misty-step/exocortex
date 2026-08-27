# AGENTS.md — Exocortex kernel

One binary (`exocortex`) giving every Misty Step agent a uniform
read/write/search interface over registered knowledge corpora. Built in Go
(v0.2 on master) with sole-publisher isolation, fail-closed reads, and
compare-and-swap concurrency.

## Operator-set decisions (2026-08-21)

Source artifact: daybook `misty-step/exocortex-kernel.md`.

- Small CLI that carries its own agent skill; two faces from one binary:
  CLI (`--json`) and stdio MCP server.
- Pluggable at the edge, never in the store. **Cortices** are writable
  markdown+git corpora with full read/write; **feeds** are adapters that
  ingest foreign sources into a cortex as compiled, provenance-stamped notes
  with `source:` lineage. No content port; no storage backends.
- Daybook is cortex #1. Core operations: `register/put/get/search/brief/note/sync/status/log/lint`.
- Sole-publisher architecture: for Daybook cortices, the kernel-owned clone in
  `~/.config/exocortex/writers/<cortex>` is the exclusive write and read authority.
  The kernel runs `pull --ff-only`, stages touched paths only, commits, and pushes
  from the isolated clone. Registered human workspaces are never preflighted,
  stashed, committed to, or mutated.
- Fail-closed operations: a missing origin remote or provisioning error fails
  closed (`writer_unavailable` / `cortex_unavailable`) with zero fallback to
  reading or writing uncommitted human bytes.
- Fleet delivery: the CLI (`exocortex`) and bundled skill (`skill://exocortex`)
  is the official, universal fleet interface. The fleet runs on OMP; per-harness
  MCP fleet installations (Slice 2) were retired by operator decision (2026-08-24)
  as unnecessary complexity. Skill source is `skills/exocortex/SKILL.md`.
  `scripts/install-skill.sh` generates the omp-config committed copy.
  omp-config `./install` deploys live. Do not hand-edit generated copies.
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
- Clone-to-green is `./scripts/check.sh`. CI runs the same command.

- Org defaults apply: Go or Rust (CR-01), minimal docs surface (DC-01), ADRs
  immutable once accepted (DC-02).
- History note: this repo previously hosted an "evidence core" concept
  (replaced 2026-08-21 by operator decision). Old tree is recoverable from
  git history; do not restore it without operator instruction.
