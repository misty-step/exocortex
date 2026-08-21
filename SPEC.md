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

Single Go binary (CR-01; decided at scaffold 2026-08-21 over Rust), two faces:

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

- Validation: the payload is validated under the cortex's profile BEFORE
  any no-op comparison or write; an invalid payload always fails, even when
  byte-identical to stored content.
- Provenance stamping: agent identity, timestamp, source — non-noop writes
  only.
- Payload separation: `<path>` is only ever the destination; the payload
  arrives via `--from`/`content`. Concurrency stays structural for EVERY
  cortex: the CAS lock lives in the generic put pipeline, not in any VCS
  driver — `caller` and `none` cortices promise the same expected-revision
  guarantees as `daybook`.
- Idempotence: a validated payload byte-equal to stored content exits
  success as a NO-OP before provenance stamping — no write, no commit — so
  identical retries are free. Conflicts return as data: create finding an
  existing path → `exists`; revision mismatch → `revision_conflict` (CR-04).

### The put pipeline (one critical section, all cortices)

Every `put` runs this sequence under one exclusive per-cortex lock
(`flock` on `${XDG_CONFIG_HOME:-~/.config}/exocortex/locks/<name>.lock`),
acquired BEFORE any state is read and released only at the end. The VCS
policy fills steps 2, 3, and 8; the CAS core (4–7) is identical everywhere:

1. lock;
2. pre-flight — `daybook` only, all three aborts conflict-as-data, run
   BEFORE refresh: with the tree already clean, the mandated
   `pull --rebase --autostash` has no in-flight work to cycle through a
   stash/pop conflict window;
   - staged paths outside this operation's touched set
     (`foreign_staged_state`) — never commit, stash away, or discard
     another worker's staged state;
   - destination staged or unstaged-dirty vs HEAD (updates only; a
     create overwrites nothing, so CAS existence answers it) →
     `dirty_destination`;
   - unstaged modifications outside the touched set
     (`foreign_unstaged_state`) — untracked files are allowed; modified
     or deleted tracked files belong to another worker, and the step-8
     unwind must never be able to reach them;
3. refresh — `daybook`: `git pull --rebase --autostash` (per the
   operator decision record; normally inert behind step 2's clean-scan),
   then REPEAT step 2's scan against post-refresh state (the pull window
   may have changed things); `caller`/`none`: nothing;
4. CAS: re-read the stored destination revision; evaluate `--expects` or
   create-absence against fresh state;
5. validate the payload under the cortex profile — invalid payloads fail
   here, before any comparison or write, even when byte-identical to
   stored content;
6. no-op short-circuit: identical retries are free. Two forms qualify —
   the payload byte-equals the stored bytes (the get → put-unchanged
   round trip), or the payload byte-equals the stored bytes with the
   kernel-owned `provenance` block stripped (a raw-draft retry). Either
   match releases the lock and exits success — no stamp, no write, no
   commit;
7. stamp provenance, atomic write (temp file + rename);
8. VCS tail — `daybook`: record `base` = HEAD sha (post-refresh), stage
   touched paths, path-limited commit (`git commit -- <touched paths>`),
   then push. On push rejection (non-fast-forward or any failure): NO
   retry, NO rebase, NO force. Unwind path-scoped, so nothing outside this
   operation can be destroyed even under a pre-flight race:
   `git reset --soft <base>` — HEAD moves back, the index is NEVER swept
   repo-wide, so even a foreign path staged after pre-flight keeps its
   index entry. Restore only the touched path by its mode: an UPDATED
   path (present in base) via `git restore --source=<base> --staged
   --worktree -- <path>`; a CREATED path does not exist in base and is
   dropped via `git rm --cached` plus file removal — a leftover would
   block the ff merge. Then converge on the winner:
   `git fetch` + `git merge --ff-only @{u}` — a lost race must not leave
   stale bytes that make `get` lie and retries spin; if the ff-only merge
   fails (rare race), stay at base with hint "refresh failed; next put's
   refresh heals". Payload preserved (CLI `--from` file is the caller's;
   MCP/stdin payloads are written to a temp file whose path is returned
   in the conflict body). Exit code: a losing UPDATE exits
   `revision_conflict` ("remote moved; re-read with get and retry"); a
   losing CREATE exits `exists` — the winner's note now occupies the
   path (operator decision 2026-08-21: existing path → `exists`).
   `caller`/`none`: nothing;
9. release lock.

`index.lock` never covers this sequence; the cortex lock does. A racing
stager cannot enter the path-limited commit even if the lock is bypassed.
Cross-host: per-host locks cannot see each other, so the CAS guarantee on
git cortices is enforced by push — a rejected push IS the cross-host
`revision_conflict` for updates and `exists` for creates; the failed put
leaves ZERO trace of itself (its own commit unwound, touched bytes
restored to base or created file removed) and the clone converges to the
remote tip via the ff-only restore. `none` and `caller`
cortices are single-host by contract.


### VCS lifecycle (per-cortex policy)

Generic `put` never hard-codes version control. The registry entry's `vcs`
policy selects steps 2, 3, and 8 above: `daybook` (git, full tail),
`caller` (kernel writes; caller commits), `none` (plain directory writes).

### Pinned interfaces

- **Revision** — lowercase hex sha256 of the note's exact file bytes, read
  inside the critical section after refresh. `get` reports it as
  `revision`; `--expects` and MCP `expectedRevision` carry the same string.
- **`get` output** (`--json`, default): `{"cortex", "path", "revision",
  "frontmatter", "content"}` — `content` is the full file text including
  frontmatter.
- **Provenance stamp** — appended to frontmatter on every successful
  WRITE; a byte-identical retry short-circuits as a successful no-op and
  stamps nothing:

  ```yaml
  provenance:
    agent: <id from --agent or EXOCORTEX_AGENT, else "unknown">
    at: <RFC3339 UTC timestamp of the write>
    via: cli | mcp
  ```

- **Cortex registry** — `${XDG_CONFIG_HOME:-~/.config}/exocortex/cortices.json`:
  `[{"name","path","vcs":"daybook"|"caller"|"none"}]`; `register` is the only
  writer.
- **Conflict payloads** — nonzero exit + JSON body:

  ```json
  {"error":"exists","operation":"create","path":"…"}
  {"error":"revision_conflict","operation":"update","path":"…","expected":"…","actual":"…"}
  {"error":"dirty_destination","path":"…","state":"staged"|"unstaged"}
  {"error":"foreign_staged_state","paths":["…"]}
  {"error":"foreign_unstaged_state","paths":["…"]}
  ```

  Every body ends with `"hint"` naming the recovery (re-read with `get`,
  re-apply, retry; or commit/unstage your own staged work).
- **Validation profiles** — each cortex declares a named profile in its
  registry entry (`profile`, default `daybook`). Built-ins:
  - `daybook` — mirrors the canonical `/wiki` contract exactly: FAIL only
    when frontmatter is missing/unparseable or `type` is absent/empty
    (`type` is free-form; tolerate every value). Everything else — missing
    `status`/`description`/`tags`/`created`, unknown keys, unknown `type`
    vocabulary — is a WARNING, never an error. `put` never strips keys,
    never modifies existing frontmatter values except adding/updating its
    own `provenance` block, and treats `created` as immutable.
  - `strict` — the five-key floor (`type`, `status`, `created`,
    `description`, `tags` all present and non-empty; RFC3339 `created`);
    opt-in for future cortices that want it.
  `lint` reports failures and warnings tiered; consumers of `daybook`
  cortices MUST NOT hard-fail on warnings.

### v0 acceptance proofs

1. Bare put on any existing path — tracked, staged, or untracked — exits
   nonzero with `exists` and a hint directing get → `--expects`. There is
   no intent inference and no `missing_expects` code (operator decision
   2026-08-21): overwriting without a stored-revision hash is impossible
   in every mode, and a stale or malformed hash falls out as an ordinary
   `revision_conflict`.
2. Bare put on existing path: `exists`. Create race under concurrency:
   exactly one creator wins; the loser gets `exists` and leaves no trace.
3. `--expects` mismatch: `revision_conflict` carrying actual revision.
4. Two concurrent updates carrying the same expected revision: exactly one
   commits; the loser aborts `revision_conflict` with zero filesystem and
   git effect. A foreign staged path makes put ABORT `foreign_staged_state`
   before any write; afterward that path remains staged and byte-identical.
5. Dirty destination: with an unstaged local edit to the target file, put
   aborts `dirty_destination` and the edit survives byte-for-byte; same for
   a staged-only change.
6. Daybook driver put: exactly one new commit touching only the target path,
   pushed; second run is a clean no-op only when content and revision match.
7. MCP face round-trip: get → put(expectedRevision) → get bumps revision and
   preserves payload byte-for-byte apart from the provenance stamp.
8. Profile conformance: a note whose frontmatter is parseable YAML with a
   non-empty `type` and no other keys passes `daybook`-profile put and lint
   (warnings permitted); `created` in an updated note survives unchanged.
9. Cross-host race, two clones of one repo simulating two hosts: both clone
   at revision R1; put via clone A succeeds and pushes; put via clone B with
   the same `--expects` commits locally, gets its push rejected, and must
   exit `revision_conflict` with the payload preserved — after which clone
   B's branch and target-file bytes are identical to the remote (A's
   content), B's commit is gone, and no rebase, merge, or force-push
   occurred at any point.
10. Unwind safety: a foreign unstaged edit appearing after pre-flight
    (constructed directly before the tail runs) survives push rejection
    byte-for-byte while the touched path is restored; afterward the losing
    clone's branch equals the remote tip and `get` returns the winner's
    bytes. Pre-flight alone: an unrelated unstaged tracked edit present up
    front aborts `foreign_unstaged_state` with the edit untouched.
11. Cross-host CREATE race: two clones create the same new path; clone A
    pushes; clone B's push is rejected, exits `exists`, B's own file is
    gone from disk, and B's branch and target bytes equal the remote
    (A's content).

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
