# SPEC — Exocortex kernel v0

Binding design for the v0 build. Source decision artifact: daybook
`misty-step/exocortex-kernel.md` (2026-08-21). Build job: Powder card
`exocortex-v0`.

## Background: Chroma Foundation

Chroma's Foundation (announced 2026-08-12) is a shared memory layer for agent
teams: a versioned, access-controlled wiki that agents write to and read
from, with concurrency control, versioning, and lineage tracking. It ingests
Notion, Google Drive, and coding-agent sessions; the team routes technical
questions through it first. Surface: alpha cloud-backed Rust CLI
(`foundation-cli` PRs #6999/#7005/#7007 in chroma-core/chroma).

## Gap analysis

The fleet already runs most of Foundation's data model:

| Foundation primitive | Current state | Gap |
| --- | --- | --- |
| Versioned wiki | daybook git repo | none |
| Retrieval | QMD over wiki + raw session collections | not exposed beyond omp; 35% of docs unembedded |
| Agent writes | prose contracts (/wiki skill, AGENTS.md) | no mechanical write path |
| Concurrency control | claim-before-work doctrine | no mechanism |
| Lineage | frontmatter conventions | unenforced, not queryable |
| Access control | single operator, private remote | N/A at current scale |
| Session ingestion | QMD collections | freshness unowned |

Conclusion: do not build a memory database. Build the thin Foundation-shaped
API over what exists. The name `foundation` is taken (omp-config baseline
skill), hence `exocortex`.

## Architecture ruling: pluggable at the edge, not in the store

Two source classes, one data model:

- **Cortices** — writable markdown+git corpora. Register one; get full
  read/write/search/lint/lineage. Daybook is cortex #1; Emma's vault
  (`~/Documents/emma-daybook`, per daybook `areas/emma-agent/architecture`)
  is a future one. No content port: files stay where they are. The kernel's
  native model IS daybook's existing model (markdown + frontmatter + git).
- **Feeds** — foreign systems (Notion, Google Drive, harness session logs,
  arbitrary repos). Adapters ingest compiled, provenance-stamped notes into a
  cortex with `source:` lineage frontmatter. Raw sources stay on disk
  (raw-vs-compiled doctrine). Write-back per adapter, never required by core.

Rejected extremes:

- Port daybook into a kernel-owned store — loses the git-markdown substrate,
  duplicates storage, big-bang migration (DE-06).
- Fully abstracted storage backends now — rule of three (DE-08): no second
  real consumer exists; complexity without rent (DE-01).

## v0 contract

Single Go or Rust binary (CR-01), two faces:

- **CLI** (`--json` everywhere, CR-02):
  - `register <name> <path>` — bind a cortex.
  - `put <path> --from <file|->` — write payload (`-` = stdin) to cortex
    destination `<path>`. Bare form is create-only: fails if `<path>`
    already exists (atomic create).
  - `put <path> --from <file|-> --expects <sha>` — update: REQUIRED
    precondition; `<sha>` must match the STORED revision of `<path>` (the
    hash `get` reports), never the payload file. Update without
    `--expects` is a hard error, never a silent overwrite.
  - `get <path>` — read a note.
  - `search "<query>"` — shells `qmd --format json`; never re-implements
    retrieval.
  - `log <path>` — git lineage.
  - `lint [<path>]` — frontmatter floor gate.
- **MCP stdio server** — same operations; `put` is
  `{path, content, expectedRevision?}` — content supplied in the call.
  Preconditions are evaluated inside the cortex critical section (see VCS
  lifecycle), after refresh and immediately before the atomic write.

Write path mechanics:

- Frontmatter floor validation (type, status, created, description, tags).
- Provenance stamping: agent identity, timestamp, source.
- Payload separation: `<path>` is only ever the destination; the payload
  arrives via `--from`/`content`. Concurrency stays structural: every update
  requires `--expects` naming the stored revision (`get` reports it; missing
  flag = hard error); bare `put` creates only. The check-then-write is one
  atomic step inside the cortex critical section — never check-before-lock,
  which lets two passers serialize into a silent replacement. Mismatch or
  create race returns a conflict as data (operation, input, expected vs
  actual state, CR-04).

### VCS lifecycle (per-cortex policy)

Generic `put` never hard-codes version control. Each cortex declares a
policy:

- `daybook` driver — one critical section per operation; the cortex lock is
  acquired BEFORE any state is read and held through push:
  1. lock `flock <cortex>/.git/exocortex.driver.lock` (`index.lock` alone
     does not cover multi-command sequences);
  2. refresh: `git pull --rebase --autostash`;
  3. pre-flight: if the index holds staged paths outside this operation's
     touched set, abort with conflict-as-data — never commit, stash away, or
     discard another worker's staged state;
  4. CAS: re-read the stored destination revision; evaluate `--expects` or
     create-absence against fresh state;
  5. validate payload, stamp provenance, atomic write (temp file + rename);
  6. stage touched paths, path-limited commit
     (`git commit -- <touched paths>`), push;
  7. release lock. A racing stager cannot enter the path-limited commit even
     if the lock is bypassed.
- `caller` policy: kernel writes files; the caller commits (default for
  non-vault repos).
- `none` policy: plain directory writes.

## Fleet delivery

The CLI is the universal baseline: every harness execs shell commands, and
the bundled skill teaches agents the interface. MCP is registered per
harness — no single config covers the fleet:

| Harness | Target | Slice |
| --- | --- | --- |
| Oh My Pi | `omp-config/mcp.json` + `install` (writes only to `${PI_CODING_AGENT_DIR:-$(omp config path)}`) | 2 |
| Claude Code | user-scope `claude mcp add` or project `.mcp.json` | 2 |
| Codex | `~/.codex/config.toml` `[mcp_servers.exocortex]` | 2 |
| opencode | `opencode.json` `mcp` block | 2 |
| goose | goose config extensions block | 2 |
| pi | its own MCP config target | 2 |

Slice 2 also owns: `skills/exocortex/` installed into omp-config from this
repo's canonical copy, and the one-line pointer in omp-config
`global/AGENTS.md`. Until then, cross-harness parity comes from the CLI.

## Rejected options

- **Standalone memory service** (daemon + own DB) — duplicates versioning and
  search already paid for; operated-service burden with no contention problem
  to justify it. Correct escalation if multi-host write storms appear.
- **Build on Chroma OSS/cloud** — dependency weight against RS-03 at this
  corpus size; alpha cloud-gated CLI; substrate churn for capability
  git+QMD already deliver.
- **Name `foundation`** — collides with the omp-config baseline skill.

## Open questions

- Language: Go recommended; Rust acceptable (CR-01). Decide at scaffold.
- Claim/lease semantics for concurrency mechanization.
- Feed priority order (harness session logs proposed first — highest tacit
  value per Huber). Coverage caveat: QMD collections cover omp, Claude Code,
  and pi raw sessions only; Hermes/Amos live in SQLite outside QMD, and
  Codex/opencode/goose session storage is unverified. Inventory every fleet
  session source before any completeness claim or feed work starts.
- QMD embedding backfill and freshness ownership (35% of daybook docs lack
  embeddings as of 2026-08-21).

## References

- Daybook artifact: `misty-step/exocortex-kernel.md` (daybook commit
  `4502bc43`)
- TBPN interview with Jeff Huber, 2026-08-12 (Foundation announcement)
- chroma-core/chroma PRs #6999, #7005, #7007 (foundation-cli)
- omp-config `CANON.md`: RS-05, DE-01–DE-08, CR-01–CR-04, TH-01–TH-04
