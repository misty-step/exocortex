# Context Packet: Public Composable Context System

## Goal

Shape Exocortex as a public, reusable context engine that private systems such
as Daybook can consume without turning their private content, commands, or
personal workflows into public product assumptions.

## Non-Goals

- Do not scaffold the Rust workspace yet.
- Do not create a GitHub remote or publish anything.
- Do not migrate Daybook in this session.
- Do not copy private Daybook notes into this repo.
- Do not build a vector database, scheduler, agent runner, or hosted SaaS.
- Do not design personal Daybook rituals as public Exocortex features.

## Constraints And Invariants

- Public/private separation is load-bearing: Exocortex is generic machinery;
  Daybook is a private consumer and implementation.
- The future durable implementation should be Rust-first unless a surface
  boundary earns a small exception.
- One core must feed many faces: API, CLI, MCP, SDK, skill, and possibly a thin
  status UI.
- MCP tools should be intent-shaped, not REST endpoints auto-wrapped 1:1.
- Context returned to an LLM can stay rich and prose-like; deterministic code
  needs rigid fields only for source identity, privacy, trust, freshness,
  capabilities, citations, and branching policy.
- Retrieval is read-only by default. Ingest/sync/writeback are separate
  capabilities.

## Current State Read

The `exocortex` checkout was empty. I initialized a local-only git repository on
`shape/public-context-system` to satisfy the lane guardrail to work on a branch;
no remote was created.

Daybook evidence read:

- `AGENTS.md` / `CLAUDE.md`: Daybook is a private command center with QMD-first
  semantic search, conversation traces, MOCs, Todoist/task surfaces, clippings,
  Monologue ingest, and vault-maintenance conventions.
- `docs/daybook-as-context-source-2026-05-22.md`: the prior best framing already
  separates source adapters, normalized evidence, indexes/views, retrieval API,
  context packets, and cited reports.
- `scripts/MONOLOGUE-SYNC.md`: Monologue is a source adapter problem: read a
  local JSON store, write normalized Markdown into a private inbox, preserve
  source metadata.
- `backlog.d/009`, `011`, `012`: Daybook has firehose pressure, stale navigation
  artifacts, and a need for bounded maintenance loops.

Factory evidence read:

- The Factory synthesis makes "one core, many faces" the standing pattern:
  functional core, thin API/CLI/MCP/SDK/skill/UI faces, no duplicated business
  logic.
- The recurring fleet gap is MCP plus SDK.
- `bastion/apps/cairn` demonstrates the local exemplar: one Rust app with web,
  JSON API, CLI, and MCP surfaces around shared models and storage.

## Chosen Shape

Exocortex should be a context kernel with three layers:

```text
sources -> adapters -> evidence log -> retrieval policy -> context packet
                                      -> API / CLI / MCP / SDK / skill
```

The core domain is not "notes" or "Daybook." The core domain is evidence:
source identity, source capabilities, source trust, freshness, privacy, chunks,
citations, and packet assembly.

### Public Exocortex Core

Own these concepts:

- `Source`: declared context provider with capabilities, trust, privacy,
  freshness, roots, auth mode, and read/write flags.
- `SourceRef`: stable URI or path that can be cited and re-fetched.
- `EvidenceChunk`: retrieved context with content, score, offsets when
  available, metadata, freshness, privacy, and trust labels.
- `EvidenceDocument`: hydrated full source behind a chunk.
- `RetrievalProfile`: source-selection policy for intents such as
  `repo_grounded`, `personal_context_heavy`, `external_validation_heavy`,
  `compliance_safe`, and `fast_scan`.
- `ContextPacket`: compact agent-ready bundle containing the question, profile,
  evidence chunks, source decisions, citations, and residual gaps.
- `EvidenceLog`: append-only JSONL receipt for what was searched, fetched,
  included, excluded, and why.

### Adapter Boundary

Adapters are imperative ports behind a small interface:

```text
search(intent, constraints) -> EvidenceChunk[]
fetch(source_ref) -> EvidenceDocument
ingest(input_ref) -> IngestReceipt
health() -> SourceHealth
```

Adapters know how QMD, ripgrep, filesystem roots, Markdown frontmatter,
Monologue exports, MCP resources, or web search work. The core does not.

### First Surfaces

- CLI: `exo sources`, `exo search`, `exo fetch`, `exo packet`, `exo ingest`,
  `exo explain`.
- MCP: intent-level tools such as `build_context_packet`, `search_sources`,
  `fetch_citation`, and `explain_source_decisions`.
- SDK: thin Rust crate first; generated or tiny TypeScript/Python clients only
  after the HTTP contract stabilizes.
- HTTP API: useful once CLI/MCP behavior is proven; should mirror the core
  verbs, not become the design authority.
- Skill: describes when agents should use Exocortex and gives deterministic
  commands for common packet-building flows.
- Thin UI: deferred; if it exists, it should inspect evidence logs, source
  freshness, and citations, not become a notebook app.

## Public Core Versus Private Daybook

| Concern | Exocortex public core | Daybook private consumer |
|---|---|---|
| Identity | Context engine | Personal command center |
| Data | Fixtures, schemas, generic adapters | Journal, people, finance, projects, voice, traces |
| Search | Adapter traits, QMD-capable adapter, rg/filesystem adapter | `qmd -c daybook`, vault paths, private ranking preferences |
| Firehoses | Generic append-only ingest schema | Monologue, clippings, conversation traces, private inboxes |
| Navigation | MOC/link graph adapter primitives | Actual MOCs and vault maps |
| Policy | Trust/freshness/privacy fields and profile engine | Which personal sources are allowed for which workflows |
| Writeback | Capability boundary only | Notes, Todoist, traces, vault gardening, private approvals |
| Surfaces | CLI/API/MCP/SDK/skill over evidence | Daybook commands that call Exocortex |

## Minimal v0

The v0 should prove the contract with fixtures before touching live private
data.

### Data Model

```text
Source
  id, kind, display_name, capabilities[], roots[], auth_scope,
  privacy_level, trust_level, freshness_policy, read_only

SourceRef
  source_id, uri, path, fragment, checksum, observed_at

EvidenceChunk
  source_ref, title, content, score, rank, offsets,
  metadata{created_at, updated_at, retrieved_at, tags, privacy, trust}

ContextPacket
  id, question, profile, constraints, generated_at,
  source_decisions[], evidence[], citations[], gaps[]

IngestReceipt
  source_id, input_ref, documents_seen, documents_changed,
  index_updated, warnings[]
```

### First Adapters

1. `local_files`: read-only filesystem roots, frontmatter, exact fetch, checksum.
2. `markdown_vault`: Markdown notes, wikilinks, MOC/index discovery, basic
   frontmatter filtering.
3. `repo_context`: `AGENTS.md`, `VISION.md`, README, backlog, docs, and rg
   search in the current repo.
4. `firehose_jsonl`: append-only transcript/event imports with stable source
   refs and timestamps.
5. `qmd_command`: optional command adapter for QMD-like local semantic search.
   It ships as generic command integration; Daybook supplies the private
   collection name and roots.

Defer Gmail, Slack, GitHub, hosted vector stores, and full web search until the
local evidence contract is proven.

### First Public Commands

```sh
exo sources list
exo search "query" --profile repo-grounded --source current_repo
exo fetch qmd://daybook/meta/example.md
exo packet "what context matters for this task?" --profile repo-grounded --out evidence/
exo explain evidence/packet.json
```

The CLI should emit machine-readable JSON by default when asked and human-readable
summaries otherwise. The MCP server should call the same core path as `exo
packet`.

## Migration Path: Daybook On Exocortex

1. **Configure, do not move.** Add a private Daybook `sources.yaml` that points
   to QMD, vault roots, Monologue inbox, clippings, conversation traces, and MOC
   files. No public repo receives private content.
2. **Read path first.** Replace selected Daybook command context gathering with
   `exo packet` calls while leaving existing QMD and scripts intact.
3. **Evidence logs.** Write packet receipts next to Daybook conversation traces
   or private `meta/briefs/` so future agents can audit sources without chat
   context.
4. **Firehose normalization.** Route Monologue/clippings/conversation traces
   through a generic firehose adapter; Daybook keeps processing policy.
5. **Agent-native access.** Register the Exocortex MCP server for Daybook once
   CLI parity is verified.
6. **Skill boundary.** Daybook skills say when to ask Exocortex; Exocortex skill
   says how to build packets. Do not duplicate private voice or rituals into
   the public skill.

## Alternatives Considered

| Option | Why it helps | Why it fails | Verdict |
|---|---|---|---|
| Rename Daybook into Exocortex | Fast path from working private system | Public repo would inherit private assumptions and personal data boundaries | Reject |
| Build a full RAG/vector platform | Familiar category, broad connector story | Rebuilds commodity infrastructure and hides the real value: provenance, trust, and profiles | Reject |
| MCP wrapper over QMD and files | Quick agent access | Becomes another shallow adapter; no evidence log, source policy, privacy, or citation discipline | Reject |
| Public evidence kernel consumed by Daybook | Clean boundary, reusable, aligns with Factory pattern | Requires careful v0 scope and fixtures before it feels useful | Choose |
| Keep this as Daybook-only scripts | Lowest immediate effort | Cannot become a public factory component or reusable SDK/MCP surface | Reject |

## Oracle

For this shaping session:

- `VISION.md` exists and states the public/private boundary.
- `docs/shaping/public-context-system.md` exists and sketches boundary, v0 data
  model, API surfaces, adapters, migration path, alternatives, and open
  questions.
- `docs/shaping/public-context-system.html` exists and has been opened for
  rendered review.
- No remote exists.

For the first implementation slice:

- A fixture repo plus fixture Markdown vault can declare sources.
- `exo packet "..." --profile repo-grounded` emits a context packet and
  `evidence.jsonl` with fetchable citations.
- The same packet can be produced through MCP.
- A privacy fixture proves a `public_safe` profile excludes private-only
  sources.
- A stale-source fixture shows freshness warnings in the packet.

## Verification System

- Claim: Exocortex can produce an auditable context packet from configured
  sources without leaking private sources into profiles that exclude them.
- Falsifier: a packet cites an un-fetchable source, omits freshness metadata,
  includes a private source under a public-safe profile, or CLI/MCP disagree.
- Driver: future fixture command plus MCP replay:
  `cargo test --workspace`, `cargo run -- exo packet ...`, and an MCP
  `build_context_packet` replay.
- Grader: JSON schema checks, citation fetch checks, privacy exclusion
  assertions, and CLI/MCP golden comparison.
- Evidence packet: `evidence.jsonl`, `packet.json`, command transcript, and MCP
  replay transcript.
- Cadence: before first implementation PR, after adapter additions, and before
  registering Daybook as a live consumer.
- Current gap: no code exists yet; this session verifies only the design
  artifacts and local repo state.

## Premise Sources

- `sha256:378a96eb94605bd996953e9e37ecc50d77c678ad11025b0786ae03456bb38175`
  `/Users/phaedrus/.factory-lanes/_brief.md`
- `sha256:e6c8e8583a75ee850eb9ba823040ce7f29a7b6dc836b5a2304e693a77d729548`
  `/Users/phaedrus/.factory-lanes/exocortex.md`
- `sha256:b8a981522941a7623b3310ebdb28e991f684d254261b097fb6f094723ffe7be4`
  `/Users/phaedrus/artifacts/public/a/factory/index.html`
- `sha256:bf8cfeca2376fa635167f9865b9bdf5fc3a77eac9b2ff9a53af299df1f889ad0`
  `/Users/phaedrus/Documents/daybook/AGENTS.md`
- `sha256:3324198385b6ae223aab7221f68b389417e42569e3ceab9480fa9ce7c5879923`
  `/Users/phaedrus/Documents/daybook/docs/daybook-as-context-source-2026-05-22.md`

## HTML Plan

`docs/shaping/public-context-system.html`

## Risks And Mitigations

- **Daybook overfit:** keep Daybook config private and make public fixtures the
  first implementation target.
- **Connector sprawl:** require every adapter to enter through `Source` and the
  adapter trait; no bespoke workflow prose per connector.
- **Shallow MCP:** design MCP tools by agent intent and call the same packet
  builder as the CLI.
- **Over-structured AI seam:** keep chunk content rich; reserve rigid schema for
  deterministic policy fields.
- **Stale indexes:** freshness metadata is mandatory before any ranking looks
  credible.
- **Premature UI:** defer UI until evidence logs exist; otherwise it will be a
  decorative browser over unproven retrieval.

## Open Questions

1. Should Exocortex v0 include an optional QMD command adapter, or should it
   start with generic filesystem/Markdown/rg only and let Daybook keep QMD
   behind private glue for one slice?
2. What is the right first profile set: the five from the Daybook research note,
   or a smaller trio of `repo_grounded`, `personal_context_heavy`, and
   `public_safe`?
3. Should `ContextPacket` be a stable public JSON schema in v0, or a Rust type
   plus JSON export that is allowed to churn until the first Daybook migration?
4. Is the first MCP server read-only only, or should `ingest_firehose` appear as
   a separate write-capable tool from the start?
5. Where should Daybook store evidence logs: beside conversation traces, under
   `meta/briefs/`, or in a new private `meta/evidence/` area?
6. How much source ranking should v0 attempt beyond deterministic profile
   ordering, trust/freshness labels, and adapter scores?
7. Does the public repo eventually want a thin UI for evidence-log inspection,
   or should that remain a downstream consumer concern until API/MCP usage
   proves demand?
