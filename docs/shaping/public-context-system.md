# Context Packet: Public Exocortex Core

## Goal

Shape Exocortex as a thin, public, self-hosted context core that private command
centers such as Daybook can consume without publishing their data, rituals,
ranking policy, or writeback behavior.

The user outcome is simple: a human or agent can initialize a repo, point it at
sources, ingest or index those sources, and ask for a cited context packet
through CLI, MCP, API, SDK, skills, or a thin UI without rebuilding a bespoke
Markdown/RAG stack each time.

## Non-Goals

- Do not scaffold the full Rust workspace in this PR.
- Do not create `.exocortex/` runtime state in this repo yet.
- Do not migrate Daybook in this PR.
- Do not copy private Daybook content, config, secrets, traces, or QMD
  collection names into the public repo.
- Do not build a hosted SaaS, chat app, Obsidian clone, workflow engine,
  scheduler, agent runner, or vector database product.
- Do not make Daybook rituals, voice, personal ranking preferences, or writeback
  behavior public defaults.

## Constraints And Invariants

- Public/private separation is load-bearing: Exocortex is reusable machinery;
  Daybook is a private consumer and implementation.
- The product should stay thinner than ad hoc Markdown in ceremony. A first user
  should get value from `exo init`, `exo ingest`, `exo search`, and
  `exo packet`.
- The substrate should be batteries-included: local embedded metadata, full-text,
  vector, and relationship indexes; local embeddings where possible; QMD as an
  optional command adapter.
- One core must feed many faces: CLI, MCP, HTTP API, SDK, bundled skills, and
  any UI.
- Surface parity matters. If a user can do it through CLI, an agent should be
  able to do it through MCP, and an SDK/API caller should hit the same core
  behavior.
- MCP tools should be intent-shaped, not a 1:1 REST wrapper.
- Context returned to an LLM can stay rich and prose-like. Deterministic code
  needs rigid fields only for source identity, privacy, trust, freshness,
  capabilities, citations, and branching policy.
- Retrieval and packet building are read-only by default. Ingest, sync,
  organization, writeback, deletion, and outward publication are separate
  capabilities.

## Repo Anchors

- `VISION.md`: root north star for the public core.
- `docs/shaping/public-context-system.md`: this buildable context packet.
- `docs/shaping/public-context-system.html`: rendered planning artifact for
  review.
- `/Users/phaedrus/.factory-lanes/_brief.md`: Factory doctrine, especially one
  core with API/CLI/MCP/SDK/skill and thin faces.
- `/Users/phaedrus/.factory-lanes/exocortex.md`: lane contract and
  public/private split.
- `/Users/phaedrus/artifacts/public/a/factory/index.html`: Factory synthesis
  report with the repeated MCP/SDK gap and one-core-many-faces pattern.
- `/Users/phaedrus/Documents/daybook/AGENTS.md`: Daybook as private command
  center with QMD-first context, traces, Monologue, and vault stewardship.
- `/Users/phaedrus/Documents/daybook/docs/daybook-as-context-source-2026-05-22.md`:
  prior Daybook context-source research and evidence contract.

## Current State Read

The repository currently contains only shaping artifacts on
`shape/public-context-system`. No implementation scaffold exists. The first
commit framed Exocortex as public evidence machinery consumed by Daybook. The
operator then refined the target: keep the product very thin at the surface, but
ship a useful local substrate by default rather than asking users to assemble
Markdown conventions, embeddings, vector storage, QMD glue, MCP tools, and
skills on their own.

Daybook evidence confirms the consumer pressure:

- Daybook is a private command center, not a public product.
- QMD is the primary semantic context path today.
- Conversation traces, Monologue sync, clippings, MOCs, project notes, and vault
  maintenance already behave like sources and firehoses.
- Daybook's value comes from private policy and context. Exocortex should
  supply the public context engine underneath it.

Factory evidence confirms the surface pattern:

- One functional core should project into API, CLI, MCP, SDK, skill, and UI.
- MCP plus SDK is the repeated fleet gap.
- MCP tools should represent agent intent, not auto-wrapped endpoints.

## Recommended Shape

Build the smallest credible command-center context kernel:

```text
sources -> ingest adapters -> object store -> indexes -> retrieval profiles
        -> context packet builder -> CLI / MCP / API / SDK / skills / thin UI
```

The product surface is intentionally small:

```sh
exo init
exo ingest <path|url|stdin> [--source <id>]
exo search "query" [--profile <profile>]
exo packet "question" [--profile <profile>] [--out <dir>]
```

Everything else exists to make those commands trustworthy, inspectable, and
available to agents.

### Conceptual Repo-Local State

This PR does not create runtime state, but the future default should be easy to
understand:

```text
.exocortex/
  config.yaml            # sources, profiles, overlays, index settings
  objects/               # normalized source refs, receipts, fetched metadata
  index/                 # embedded metadata, FTS, vector, and graph indexes
  packets/               # generated packets when --out is not supplied
  logs/evidence.jsonl    # append-only retrieval and ingest receipts
  views/                 # generated MOCs or source maps, never hand-authored truth
```

The user should be able to crack the hood here without depending on a hosted
dashboard or private Daybook scripts.

## Public Core Versus Private Daybook

| Concern | Exocortex public core | Daybook private consumer |
|---|---|---|
| Identity | Self-hosted context core | Personal command center and vault |
| Data | Fixtures, schemas, generic adapter code, generated receipts | Journal, people, finance, projects, voice, traces, clippings |
| Config | Source/profile schema, overlay model, capability boundaries | Actual source roots, QMD collection, secrets, private policies |
| Search | Embedded full-text/vector search, adapter traits, QMD command adapter | `qmd -c daybook`, private ranking preferences, vault-specific semantics |
| Ingest | Files, Markdown, URL, stdin, transcript/event JSONL, command adapters | Monologue, Super Whisper, clippings, conversation traces, private inboxes |
| Organization | Suggested tags, source maps, generated views, approval boundary | Real MOCs, vault gardening, personal taxonomy, writeback decisions |
| Skills | Generic "build/fetch/inspect context" skills | Daybook-specific rituals, voice, kickoff/debrief behavior |
| MCP | Intent tools over the public core | Registered private server/profile config and source permissions |
| UI | Thin source/evidence/packet inspector | Any personal dashboard or command-center experience |
| Logs | Generic JSONL receipts and packet provenance | Private evidence archives and conversation-trace links |

The boundary rule: public Exocortex may know that a `qmd_command` adapter
exists. It must not know Daybook's private collection, folders, or policies.

## v0 Surface

### CLI Surface

- Command tree:
  - `exo init`
  - `exo sources list|add|doctor`
  - `exo ingest <path|url|-> --source <id> [--profile <profile>]`
  - `exo search "query" --profile <profile> [--json|--plain]`
  - `exo packet "question" --profile <profile> [--out <dir>] [--json]`
  - `exo fetch <source-ref> [--json|--plain]`
  - `exo explain <packet-or-ref>`
  - `exo organize suggest [--profile <profile>]`
- Primary users: humans at a terminal, scripts, and agents shelling out.
- Inputs: args, stdin, files, URLs, repo-local config, env for provider keys, and
  optional adapter commands such as QMD.
- Outputs: human summaries by default, `--json` for machines, packet files when
  `--out` is supplied, diagnostics/progress to stderr.
- Interactivity: prompts allowed only on TTY; `--no-input` disables prompts.
- Safety: no destructive organization or writeback in v0. `organize suggest`
  emits a plan; applying it is explicitly out of this PR and should require a
  future approval flag/capability.
- Config precedence: flags > env > repo `.exocortex/config.yaml` > user config >
  defaults.
- Platform/runtime: Rust single binary should be the default target. Non-Rust
  clients are thin generated or handwritten SDK surfaces after the contract is
  stable.
- Exit code map:
  - `0`: command succeeded.
  - `1`: invalid input, missing source, or no usable config.
  - `2`: partial packet with gaps or stale sources when `--strict` is set.
  - `3`: privacy/capability violation blocked the request.

### MCP Tools

MCP should expose agent-intent tools, each backed by the same core path as the
CLI:

- `build_context_packet(question, profile, constraints)`: returns packet
  content, citations, gaps, and receipt path.
- `search_context(query, profile, constraints)`: returns ranked evidence chunks
  with source decisions.
- `fetch_source(source_ref)`: hydrates a cited source or chunk.
- `ingest_source(input_ref, source_id, options)`: imports or indexes a source
  when the configured capability allows it.
- `organize_context(scope, profile, mode="suggest")`: proposes tags, source
  maps, stale-source fixes, or view updates. v0 is suggest-only.

Do not auto-wrap every CLI/API command. Keep read tools separate from ingest or
mutation tools so risk levels are visible to agents and clients.

### API And SDK

The HTTP API and SDK should mirror core verbs, not become a second design:

- `init_workspace`
- `list_sources`
- `ingest_source`
- `search_context`
- `build_context_packet`
- `fetch_source`
- `explain_packet`
- `suggest_organization`

SDKs should make common flows concise, but they should not add behavior that the
CLI/MCP cannot exercise.

### Thin UI

A UI is allowed only as an inspector over the proven core:

- source status and freshness
- ingest receipts
- packet explorer
- citation fetch/preview
- profile and privacy diagnostics
- organization suggestions awaiting approval

It should not become a notebook app, personal dashboard, or chat front-end in
v0.

## Built-In Substrate

Exocortex should be thin, but not hollow. v0 should ship a local stack that works
without asking the user to design retrieval infrastructure:

- source registry with capabilities, trust, privacy, freshness, auth mode, and
  roots
- local object metadata and fetch receipts
- embedded full-text search
- embedded vector search with local embeddings by default
- lightweight graph/relationship edges from links, paths, source refs, and
  generated views
- local file and Markdown ingest
- URL and stdin ingest
- transcript/event JSONL ingest for firehoses
- optional QMD command adapter
- packet builder with citations, source decisions, freshness, and gaps
- JSONL evidence log

Backend flexibility comes later through adapters: Qdrant, LanceDB, hosted file
search, or provider embeddings can be swapped in, but a useful local default is
part of the product promise.

## Core Domain Model

```text
Workspace
  id, root, config_version, default_profile, inherits[]

Source
  id, kind, display_name, capabilities[], roots[], auth_scope,
  privacy_level, trust_level, freshness_policy, read_only, adapter_config

SourceRef
  source_id, uri, path, fragment, checksum, observed_at, fetch_hint

EvidenceChunk
  source_ref, title, content, score, rank, offsets,
  metadata{created_at, updated_at, retrieved_at, tags, privacy, trust}

ContextPacket
  id, question, profile, constraints, generated_at,
  source_decisions[], evidence[], citations[], gaps[], receipt_ref

IngestReceipt
  source_id, input_ref, documents_seen, documents_changed,
  index_updated, warnings[], receipt_ref

RetrievalProfile
  id, include_sources[], exclude_sources[], privacy_floor,
  trust_policy, freshness_policy, ranking_policy, max_chunks
```

Keep this model as stable as possible at the boundaries, but do not freeze every
internal index shape before the first Daybook migration teaches us what matters.

## First Profiles

Start with four profiles:

- `repo_grounded`: current repo, docs, backlog, local files, exact search, and
  only explicitly configured external/private sources.
- `public_safe`: excludes private sources and secrets by default; suitable for
  public docs, PRs, and external sharing.
- `personal_context`: allows private command-center sources such as Daybook when
  configured by the consumer.
- `deep_research`: combines local configured context with external retrieval
  adapters and subagent-ready packet receipts. The core builds the packets; a
  research skill or downstream agent does the orchestration.

Avoid a large taxonomy in v0. Profiles should be a small policy layer, not a
workflow language.

## Daybook To Exocortex Migration Path

1. **Preserve Daybook as private.** No data move. No public config. No private
   notes, collection names, or secrets land in Exocortex.
2. **Add private Daybook config.** Create Daybook-local `.exocortex/config.yaml`
   mapping QMD, vault roots, conversation traces, Monologue inbox, clippings,
   project notes, MOCs, and evidence-log destination.
3. **Read path first.** Replace selected Daybook context-gathering calls with
   `exo packet --profile personal_context` while leaving QMD and existing
   scripts intact.
4. **Record evidence privately.** Write receipts under a Daybook-owned location,
   likely `meta/evidence/` or linked conversation traces, so future agents can
   audit source decisions.
5. **Normalize firehoses.** Route Monologue, clippings, and conversation traces
   through the generic transcript/event JSONL adapter; Daybook keeps source
   policy and raw transcript conventions.
6. **Register MCP after CLI parity.** Daybook should use the Exocortex MCP server
   only once the same packet can be produced through the CLI and citations are
   fetchable.
7. **Move skills by boundary.** Exocortex ships generic context skills. Daybook
   skills call Exocortex but keep personal voice, kickoff/debrief behavior, and
   writeback rules private.
8. **Introduce overlays for subsets.** Adminifi can inherit a narrow Daybook
   profile without copying Daybook data:

   ```yaml
   workspace: adminifi
   inherits:
     - ../daybook/.exocortex
   include_profiles:
     - adminifi_context
   exclude:
     - personal
     - finance_private
     - relationship_private
   ```

   This overlay/inheritance design is promising, but it should remain an open
   design question until Daybook proves the first read-only migration.

## Alternatives Considered

| Option | Why it helps | Why it fails | Verdict |
|---|---|---|---|
| Ad hoc Markdown conventions | Lowest setup cost and already familiar | No shared API/MCP/SDK surface, no evidence receipts, weak privacy/freshness policy | Reject as the baseline to beat |
| Rename Daybook into Exocortex | Fast path from a working private system | Public repo inherits private assumptions and risks leaking personal boundaries | Reject |
| Full RAG/vector platform | Familiar category, connector-rich, easy to explain | Competes on commodity infrastructure and bloats past the command-center core | Reject |
| MCP wrapper over QMD/files | Quick agent access | Shallow pass-through with no source governance, surface parity, or packet contract | Reject |
| Hosted context SaaS | Easy onboarding story | Violates self-hosted/local-first premise and private-consumer trust | Reject for now |
| Thin public context core with baked-in local index | Beats Markdown while staying composable; supports Daybook, Adminifi, and future repos | Requires careful v0 scope and parity discipline | Choose |

## Oracle

This PR is a shaping PR, not the implementation PR. It is done when:

- `VISION.md` states the public Exocortex core, private Daybook consumer
  boundary, thin surface, embedded index promise, and v0 excellence loop.
- `docs/shaping/public-context-system.md` covers the boundary, v0
  `exo init/ingest/search/packet` surface, MCP tools, API/SDK parity, built-in
  substrate, Daybook migration path, alternatives, and open questions.
- `docs/shaping/public-context-system.html` is updated as the rendered planning
  artifact and opened for review.
- The branch is pushed to `origin/shape/public-context-system`.
- A GitHub PR is open.
- No full build scaffold is introduced.

First implementation PR oracle, for later:

- Fixture repo runs `exo init`.
- Fixture Markdown/URL/stdin sources run through `exo ingest`.
- `exo search "query" --profile repo_grounded --json` returns cited evidence
  chunks.
- `exo packet "question" --profile repo_grounded --out evidence/` writes
  `packet.json` and `evidence.jsonl`.
- MCP `build_context_packet` produces an equivalent packet from the same core.
- `public_safe` fixture excludes private-only sources.
- `exo explain` shows included/excluded source decisions and freshness gaps.

## Verification System

- Claim: the docs now define a buildable public Exocortex context core without
  scaffolding the implementation or blurring the Daybook private boundary.
- Falsifier: docs omit the v0 CLI/MCP surface, treat Daybook as public product,
  describe Exocortex as only a vector DB/RAG app, or introduce runtime scaffold.
- Driver: file inspection, targeted `rg` checks, ASCII scan, HTML parse,
  `git diff --check`, git status, push, and PR creation.
- Grader: expected terms and files are present; no unexpected runtime files are
  created; GitHub reports an open PR; branch/remote sync check reports `0 0`.
- Evidence packet: command transcript in this run plus the pushed PR URL.
- Cadence: this docs PR now; first implementation PR must add executable
  fixture commands and MCP replay.
- Gap/waiver: no live `exo` binary exists yet, so implementation behavior is not
  claimed in this PR.

## Premise Sources

- `sha256:fdbf950fca898db1eb0036fa83c291e090c4825d15a32fa8205c26d072cf79fa`
  `/Users/phaedrus/.factory-lanes/_brief.md`
- `sha256:e6c8e8583a75ee850eb9ba823040ce7f29a7b6dc836b5a2304e693a77d729548`
  `/Users/phaedrus/.factory-lanes/exocortex.md`
- `sha256:b8a981522941a7623b3310ebdb28e991f684d254261b097fb6f094723ffe7be4`
  `/Users/phaedrus/artifacts/public/a/factory/index.html`
- `sha256:bf8cfeca2376fa635167f9865b9bdf5fc3a77eac9b2ff9a53af299df1f889ad0`
  `/Users/phaedrus/Documents/daybook/AGENTS.md`
- `sha256:3324198385b6ae223aab7221f68b389417e42569e3ceab9480fa9ce7c5879923`
  `/Users/phaedrus/Documents/daybook/docs/daybook-as-context-source-2026-05-22.md`
- Waiver: the operator's 2026-07-01 chat refinement about thinness, embedded
  vectors/embeddings/QMD, skills, self-hosting, Adminifi nesting, and the PR
  pause instruction is not a stable repo file; this packet incorporates it
  directly and reports the remaining decisions below.

## HTML Plan

`docs/shaping/public-context-system.html`, opened locally after edit.

## Risks And Mitigations

- **Daybook overfit:** use public fixtures first and keep Daybook config private.
- **Too thin to beat Markdown:** ship embedded search, vector index, embeddings,
  packet receipts, and MCP parity in v0 instead of just schemas.
- **Too thick to stay composable:** keep research, ideation, writeback, and UI
  workflows as skills or consumers over the core.
- **Connector sprawl:** every adapter must enter through `Source`,
  `SourceRef`, `EvidenceChunk`, and `IngestReceipt`.
- **Shallow MCP:** design MCP tools around agent intent and keep read/ingest
  risk levels separate.
- **Over-structured AI seam:** keep chunk content rich; reserve rigid schema for
  deterministic policy and citations.
- **Stale indexes:** freshness metadata and source health must be visible before
  ranking looks credible.
- **Unsafe self-organization:** v0 organization is suggest-only; apply/writeback
  needs explicit future capability and approval design.

## Open Questions

1. Should the embedded vector layer default to a specific local backend in v0, or
   start behind a trait with a simple in-process implementation and choose the
   durable backend after fixtures?
2. Should QMD ship as a public optional command adapter in the first
   implementation slice, or should Daybook keep QMD behind private glue until
   the file/Markdown/URL path is proven?
3. How much of `ContextPacket` should be a stable public JSON schema in v0
   versus a Rust type with JSON export allowed to churn until Daybook migrates?
4. Are overlays/inheritance (`Adminifi` as a subset of Daybook) a v0 design
   requirement, or should they wait until Daybook has a working private
   `.exocortex/` config?
5. Should `ingest_source` appear in the first MCP server, or should v0 MCP be
   read-only except for packet/search/fetch until privacy and capability gates
   are battle-tested?
6. Where should Daybook store Exocortex evidence logs: beside conversation
   traces, under `meta/briefs/`, or in a new `meta/evidence/` area?
7. What is the minimum useful skill pack for v0: one generic context skill, or
   separate skills for packet building, deep research, creative ideation, and
   organization review?
8. Should the thin UI exist in v0 as an evidence inspector, or should it wait
   until CLI/MCP/API usage proves the inspection gaps?
