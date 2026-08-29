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
    retrieval. Default (omitted) mode is hybrid (`qmd query`) with one
    fallback to deterministic BM25 (`qmd search`) if `qmd query` errors
    while the context is still active. Empty mode has that meaning on CLI and MCP. `--mode
    bm25|hybrid|vector` selects explicitly. Callers that need
    deterministic BM25 must pass `--mode bm25` / `mode: "bm25"`. The
    kernel strips `CI`/`CI_` from the QMD environment, so `CI=true`
    does not by itself disable hybrid; if QMD still fails, fallback is
    the truthful path. `--type
    decision|memo|session|note|scratch` filters hits after retrieval.
    Memo matching uses the resolved cortex `journal_prefix`. Decision
    and `brief` share one exclusion list and one `liveStatus` helper.
    `--type decision` matches kind (`type: decision`, or live
    `type: note`, or a live untyped orientation path after a successful
    Get). A Get conflict does not fail open through path heuristics.
    `brief` applies `liveStatus` to every candidate and keeps a looser
    type policy (any live non-noise note, including archived decisions
    excluded).
  - `brief "<topic>"` — orientation packet of live canonical notes.
    CLI-only. MCP search accepts the same optional `type` filter.
  - `note "<thought>"` — journal micro-memory as an IMMUTABLE file
    (`<journal-prefix>/YYYY-MM-DD/<ulid>-<agent>.md`, ULID = ms
    timestamp + crypto randomness; prefix is a per-cortex registry
    field — daybook sets `meta/agents-board/memo` per its namespace
    doctrine, human `journal/` is off-limits). Create-only through the same pipeline; unique
    paths make cross-host push races benign, so `Note` retries race
    outcomes against converged state (bounded) and preserves the
    payload on every terminal conflict. Memo notes are silent under the
    daybook profile. Journal files are
    append-only: generic `put` updates under the journal prefix abort
    `journal_immutable` (ADR-0002/0003). For `vcs=daybook`, the kernel
    never writes, scans, stashes, or commits the registered human
    checkout. The sole publisher is a persistent clone under
    `<config>/exocortex/writers/<name>`, provisioned from the checkout's
    origin and fail-closed (`writer_unavailable`) if origin or clone
    setup fails. Preflight, refresh, CAS, and the VCS tail all run on
    that clone. The writer tree is the authoritative indexed root; the
    QMD collection named for the cortex must point at that tree (or, for
    `caller`/`none`, at the registered path). `sync` owns index and
    embed freshness. `get`/`log`/`lint` take the same per-cortex lock as
    `put`, provision the writer if needed, require it clean, fast-forward
    to `@{u}` when upstream exists, then read (`git show HEAD:<path>` for
    daybook get). `ensureWriter` does not refresh existing clones and
    does not lock. A failed refresh is `cortex_unavailable`. Human
    commits on origin become visible; uncommitted human checkout bytes
    stay invisible.
  - `sync [--cortex <name>]` — acquire the same per-cortex write lock
    as `put`, snapshot dirty markers, require `qmd collection show`
    Path to equal `effectiveRoot` (fail-closed:
    `index_root_unverified` / `index_root_mismatch`, markers retained),
    run `qmd update` then `qmd embed -c`, persist `synced.json`, and
    delete only the snapshotted markers. Missing `--cortex` walks every
    registered cortex and continues after a per-cortex failure.
  - `status [--cortex <name>]` — report dirty markers, last synced
    identity, and last sync error without creating state or clones.
  - `log <path>` — git lineage.
- **MCP stdio server** — same operations. `put` is
  `{path, content, expectedRevision?}`; `sync` and `status` are
  `{cortex?}`. Content is supplied in the call.
  Preconditions are evaluated inside the cortex critical section (see VCS
  lifecycle), after refresh and immediately before the atomic write.

Write path mechanics:

- Validation: the payload is validated under the cortex's profile BEFORE
  any no-op comparison or write; an invalid payload always fails, even when
  byte-identical to stored content.
- Provenance stamping: agent identity, timestamp, source — non-noop writes
  only.
- Dirty marker: every successful non-noop put records an immutable marker
  under `<config>/exocortex/state/<cortex>/dirty/`. Identity is the git
  commit when the VCS tail produced one, otherwise the content revision.
  Marker persistence failure is a `dirty_marker_failed` warning, not a
  put failure. No-op puts do not mark.
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
2. pre-flight — `daybook` only, on the writer clone, all three aborts
   conflict-as-data, run BEFORE refresh. There is no
   `--autostash`: the writer is kernel-owned and must stay clean, so
   refresh never rebases an unpublished candidate. The create-mode
   existence check is NOT a working-tree stat; CAS later answers
   existence from `HEAD:<path>` only. Preflight still runs under the
   lock (step 1), never before it:
   - staged paths outside this operation's touched set
     (`foreign_staged_state`) — never commit, stash away, or discard
     another worker's staged state;
   - destination staged or unstaged-dirty vs HEAD (updates only; a
     create overwrites nothing in HEAD, so CAS existence answers it) →
     `dirty_destination`;
   - unstaged modifications outside the touched set
     (`foreign_unstaged_state`) — untracked files are allowed; modified
     or deleted tracked files belong to another worker, and the step-8
     unwind must never be able to reach them;
3. refresh — `daybook`: `git fetch`, then require `HEAD` to be an
   ancestor of `@{u}` (equal or behind), then `git merge --ff-only @{u}`.
   Ahead or diverged (unpublished candidate after `publish_unknown`)
   is `refresh_failed`; nothing is written and the candidate is not
   rebased. Then REPEAT step 2's scan against post-refresh state;
   `caller`/`none`: nothing;
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
   then push. Force-push, merge commits, and rebase of the candidate
   commit are forbidden. A push failure is classified before any data
   conflict is reported:
   - Non-fast-forward: fetch and re-evaluate the original create-absence
     or `--expects` predicate against `@{u}:<path>`. Path absence and
     path-observation failure are distinct; an observation failure never
     becomes a data conflict or replay. If the target is unchanged,
     unwind path-scoped, ff-only onto `@{u}`, and replay the immutable,
     already-stamped candidate held by the operation (no worktree re-read,
     no re-stamp) with a bound of two replays. If the target is observed
     to have changed, unwind path-scoped, converge, then `exists` (create)
     or `revision_conflict` (update) with `actual` from the converged
     path. `actual` equal to `expected` is not a publication conflict.
     Fetch or observation failure is `writer_unavailable` with
     `remote: rejected`; exhaustion after two unchanged-path replays is
     `publish_rejected` with `reason: contention_exhausted`. Both are
     known-not-landed results, never `publish_unknown`.
   - Auth, hook, permission, policy, and explicit `remote rejected`
     refusals are `publish_rejected` (not a data conflict; do not retry
     as a lost race). Unwind path-scoped; payload preserved.
   - Transport, timeout, and accepted-but-response-lost outcomes fetch
     and compare candidate bytes to `@{u}:<path>`: a match is proved
     success only if the writer HEAD equals the fetched tip. Otherwise
     `publish_unknown`. `note` must not mint a second path. If fetch or
     path observation fails, do not unwind the local candidate.
   - If unwind or ff-only onto the evaluated tip fails, the outcome is
     `writer_unavailable` with `remote: landed` (bytes matched; repair
     the publisher clone to `proved_commit`/`proved_revision`; do not
     retry the landed write), `remote: rejected` (the push was rejected),
     or `remote: unknown` (landing is still ambiguous). The latter two
     expose only `observed_tip`, never a proved write identity.
   Unwind stays path-scoped, so nothing outside this operation can be
   destroyed even under a pre-flight race: `git reset --soft <base>` —
   HEAD moves back, the index is NEVER swept repo-wide, so even a
   foreign path staged after pre-flight keeps its index entry. Restore
   only the touched path by its mode: an UPDATED path (present in base)
   via `git restore --source=<base> --staged --worktree -- <path>`; a
   CREATED path does not exist in base and is dropped via `git rm
   --cached` plus file removal — a leftover would block the ff merge.
   Then converge: `git fetch` + `git merge --ff-only @{u}`. If the
   ff-only merge fails (rare race), stay at base with hint "refresh
   failed; next put's refresh heals". Payload preserved (CLI `--from`
   file is the caller's; MCP/stdin payloads are written to a temp file
   whose path is returned in the conflict body). `caller`/`none`:
   nothing;
9. release lock.

`index.lock` never covers this sequence; the cortex lock does. A racing
stager cannot enter the path-limited commit even if the lock is bypassed.
Cross-host: per-host locks cannot see each other, so the CAS guarantee on
git cortices is enforced by push. After a non-fast-forward, the kernel
re-evaluates the path predicate; `exists`/`revision_conflict` fire only
when that path changed. Unrelated ref movement is replayed. Auth/hook
refusals are `publish_rejected`. A lost push acknowledgment is proved
success or `publish_unknown`. `none` and `caller` cortices are
single-host by contract.


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

- **Cortex registry** — the user registry is
  `${XDG_CONFIG_HOME:-~/.config}/exocortex/cortices.json`:
  `[{"name","path","vcs":"daybook"|"caller"|"none",
  "profile":"…","journal_prefix":"…"}]`. `register` writes only this
  registry. For directory-scoped cortices, `LoadRegistry` also reads every
  `.exocortex/cortices.json` from the filesystem root through the current
  working directory. Those files augment the user registry. Every registry
  entry receives the same name, directory, VCS, and profile validation and
  defaults as `register`; unknown JSON fields fail closed. Duplicate names or
  canonical cortex roots within one file or across scopes fail closed so one
  corpus retains one writer and lock identity. Relative `path` values resolve
  from the directory that owns `.exocortex`. `journal_prefix` (optional,
  default `journal`) is where `note` files land.
- **Conflict payloads** — nonzero exit + JSON body:

  ```json
  {"error":"exists","operation":"create","path":"…"}
  {"error":"revision_conflict","operation":"update","path":"…","expected":"…","actual":"…"}
  {"error":"publish_rejected","operation":"create","path":"…"}
  {"error":"publish_unknown","operation":"update","path":"…"}
  {"error":"writer_unavailable","operation":"create","path":"…","remote":"landed","proved_commit":"…","proved_revision":"…","converged":false,"unwind":[],"push_stderr":"…"}
  {"error":"writer_unavailable","operation":"update","path":"…","remote":"rejected","observed_tip":"…","converged":false,"unwind":[],"push_stderr":"…"}
  {"error":"writer_unavailable","operation":"update","path":"…","remote":"unknown","observed_tip":"…","converged":false,"unwind":[],"push_stderr":"…"}
  {"error":"dirty_destination","path":"…","state":"staged"|"unstaged"}
  {"error":"foreign_staged_state","paths":["…"]}
  {"error":"foreign_unstaged_state","paths":["…"]}
  {"error":"created_immutable","operation":"update","path":"…","stored":"…","submitted":"…"}
  {"error":"journal_immutable","operation":"update","path":"…"}
  {"error":"duplicate_cortex","operation":"register","path":"…"}
  {"error":"duplicate_path","operation":"register","path":"…","name":"…"}

  ```

  Input-class failures (`invalid_input`, `unknown_command`,
  `registration_failed`, `payload_unreadable`, `internal_error`) emit the
  same JSON shape on stdout for every COMMAND EXECUTION path — flag
  errors, missing arguments, bare invocation included — and exit 2;
  operation conflicts exit 1. The single exception is the human-facing
  `help` / `-h` / `--help` form, which prints usage text to stdout and
  exits 0; it is documentation, not command output.

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
    own `provenance` block. `created` is IMMUTABLE: an update whose
    payload changes or drops an existing non-empty `created` aborts
    `created_immutable` (stored and submitted values returned in the
    body; compared as lexical scalar text, so unquoted timestamps are
    covered); filling a missing `created` is legal gap-fill.
  - `strict` — the five-key floor (`type`, `status`, `created`,
    `description`, `tags` all present and non-empty; RFC3339 `created`);
    opt-in for future cortices that want it.
  `lint` reports failures and warnings tiered; consumers of `daybook`
  cortices MUST NOT hard-fail on warnings.

### v0 acceptance proofs

1. Bare put on a path that exists in HEAD (the committed snapshot)
   exits nonzero with `exists` and a hint directing get → `--expects`.
   Existence is `git show HEAD:<path>` for daybook, not a working-tree
   stat: an untracked leftover in the writer clone is crash residue and
   create overwrites it. There is no intent inference and no
   `missing_expects` code (operator decision 2026-08-21): overwriting a
   committed note without a stored-revision hash is impossible, and a
   stale or malformed hash falls out as an ordinary `revision_conflict`.
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
   Immutability probes: an update CHANGING or DROPPING an existing
   non-empty `created` aborts `created_immutable` with zero file effect —
   including the unquoted-RFC3339 form yaml.v3 decodes to time.Time —
   while filling a missing `created` succeeds.
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
12. Two journal writers racing across clones both land: the loser's
    push is non-fast-forward, Put replays the unchanged unique path on
    the converged tip, and BOTH notes exist afterward. Every terminal
    `note` conflict carries a preserved-payload path. `publish_unknown`
    does not mint a second journal path.
13. Concurrent registration from two PROCESSES survives: both entries
    exist afterward (registry transaction lock + unique save temp).
14. Generic different-path creates and updates both land after unrelated
    ref movement. `exists`/`revision_conflict` fire only after the
    target path is observed to have changed.
15. A pre-receive hook or auth refusal exits `publish_rejected` with
    payload preserved, zero remote effect, and the local candidate
    unwound. It is not `exists` or `revision_conflict`.
16. Accepted-but-response-lost: when `@{u}:<path>` bytes match the
    candidate, the put is proved success. Otherwise `publish_unknown`.

## Fleet delivery

The CLI is the universal baseline: every harness execs shell commands, and
the bundled skill (`skill://exocortex`) teaches agents the interface. The
fleet runs on OMP; per-harness MCP fleet registration (Slice 2) was retired
by operator decision (2026-08-24) as unnecessary complexity. Skill source
is `skills/exocortex/SKILL.md` in this repository.
`scripts/install-skill.sh` writes the committed omp-config copy
(`omp-config/skills/exocortex/SKILL.md`). omp-config `./install` deploys
that copy to the live harness (`PI_CODING_AGENT_DIR`, default
`~/.omp/agent`). Do not run `install-skill.sh` against the live agent
dir. omp-config `skills/exocortex/check-source.sh` fails if the committed
copy drifts. SPEC and `exocortex help` own binding product semantics.
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
- Historical embedding gap (35% of daybook docs lacked embeddings as of
  2026-08-21) is closed by `sync` + the dirty-marker write path, not by
  a separate backfill owner.

## References

- Daybook artifact: `misty-step/exocortex-kernel.md` (daybook commit
  `4502bc43`)
- TBPN interview with Jeff Huber, 2026-08-12 (Foundation announcement)
- chroma-core/chroma PRs #6999, #7005, #7007 (foundation-cli)
- omp-config `CANON.md`: RS-05, DE-01–DE-08, CR-01–CR-04, TH-01–TH-04
