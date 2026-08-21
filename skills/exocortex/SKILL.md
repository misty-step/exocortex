---
name: exocortex
description: Read, search, write, and lint registered knowledge cortices (daybook first) via the exocortex CLI — use when orienting before work or writing durable results back to fleet memory.
---

# Exocortex

Exocortex is the fleet memory kernel: one binary over registered knowledge
corpora (**cortices**). Daybook is cortex `daybook`. Foreign sources (Notion,
Drive, harness session logs) enter through **feeds** as provenance-stamped
notes — never by editing raw sources.

Status: this file is the binding v0 interface contract. The binary ships from
this repo; until it is installed (omp-config slice 2), fall back to
`qmd search "<query>" -c daybook` and direct git per the daybook AGENTS.md.

## When to use

- **Before work (orient):** search the cortex for prior decisions, project
  context, and recurring patterns instead of inferring from memory.
- **After work (write-back):** durable decisions, corrected facts, and
  reusable syntheses go into the cortex; raw session logs stay on disk.

## Command contract (v0)

```sh
exocortex search "who owns powder credentials" --json   # shells qmd; hits with paths + scores
exocortex get areas/work-philosophy.md                  # read one note
exocortex put misty-step/new-decision.md --from draft.md              # create-only: fails if it exists
exocortex put misty-step/decision.md --from draft.md --expects <sha>  # update: stored-revision hash REQUIRED
exocortex log misty-step/new-decision.md                # lineage
exocortex lint misty-step/new-decision.md               # frontmatter floor gate
```

Write rules enforced by `put` (do not pre-satisfy by hand):

- Frontmatter: the cortex's validation profile decides. Daybook cortex
  follows the `/wiki` floor — parseable YAML with non-empty `type` required;
  `description` strongly recommended; everything else warned, never failed.
  `created` is never modified; unknown keys and `type` values are tolerated.
- Provenance stamped automatically (agent, time, source); never fake it.
- Payload and destination are separate: `--from <file|->` supplies the
  content; `<path>` is where it lands in the cortex. Concurrency is
  structural: bare `put` creates only — on ANY existing path it fails
  `exists` with a hint; updating REQUIRES `--expects <sha>` naming the
  STORED revision (`get` reports it). A stale or malformed hash fails
  `revision_conflict`. There is no way to overwrite without the hash.
  On mismatch, re-read with `get`, re-apply your change on top, retry.
  Never overwrite a conflict.

## VCS policy is per-cortex

`put` does not universally commit. The `daybook` cortex runs
`pull --rebase --autostash`, stages only touched paths, commits, pushes.
Other cortices may leave commits to the caller. Never run `git add -A`, never
force-push, never amend another worker's commit.

## Naming and linking

Notes are claims, not topics ("distribution is the moat", not "thoughts on
distribution"). Link densely with full paths
(`[[misty-step/exocortex-kernel]]`). Full conventions: the `/wiki` skill.
