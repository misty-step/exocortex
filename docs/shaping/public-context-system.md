# Context Packet: Public Exocortex Core

## Goal

Shape Exocortex as a public, local-first evidence core for agent-readable
context. The public repo owns reusable machinery: source registry, generic
adapters, local index state, profiles, citations, receipts, and agent-facing
surfaces. Private consumers own their roots, commands, policies, rituals,
secrets, and evidence archives.

The user outcome is simple: a human or agent can initialize a workspace, point
it at allowed sources, search, fetch, and compile a cited packet through CLI,
MCP, API, SDK, and a shipped skill without publishing private topology or
rebuilding a bespoke retrieval stack each time.

## Non-Goals

- Do not scaffold the Rust workspace in this packet.
- Do not create `.exocortex/` runtime state in this repo yet.
- Do not migrate a private consumer in this repo.
- Do not commit private roots, command names, collection names, secrets,
  personal paths, private policies, or evidence archives.
- Do not build a hosted SaaS, chat app, Obsidian clone, workflow engine,
  scheduler, agent runner, or vector database product.
- Do not add implementation beyond the public-boundary scrub, question closure,
  and retrieval-experiment definition while the repo is frozen-experimental.

## Constraints And Invariants

- Public/private separation is load-bearing: Exocortex is reusable evidence
  machinery; private consumers are proof surfaces, not public product
  boundaries.
- The product should stay thinner than ad hoc Markdown in ceremony. A first
  user should get value from `exo init`, `exo ingest`, `exo search`,
  `exo packet`, `exo fetch`, and `exo explain`.
- The substrate should be batteries-included without becoming a platform:
  source registry, generic adapters, embedded full-text/vector search, local
  embeddings where practical, profiles, packets, receipts, and fetchable
  citations.
- One core must feed many faces: CLI, MCP, HTTP API, SDK, bundled skill, and any
  later inspector.
- Surface parity matters. If a user can search, fetch, or build a packet through
  CLI, an agent should be able to do the same through MCP against the same core
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
- `backlog.d/`: house-format execution queue and frozen-experimental sequence.
- `docs/shaping/public-context-system.md`: this buildable context packet.
- `docs/shaping/public-context-system.html`: rendered planning artifact for
  review.
- `docs/experiments/retrieval-validation.md`: validation experiment definition
  that must be satisfied before implementation resumes.

## Current State Read

The repository is pre-code. It has a vision, shaping docs, release-intelligence
workflow metadata, a license, and a groomed backlog. It has no runtime scaffold,
no build system, no named repo gate, no fixtures, and no executable `exo` binary.

The repo is therefore frozen-experimental. The first useful work is not code:
it is public-boundary hygiene, decision closure, and a concrete validation
experiment that can decide whether the evidence contract is worth implementing.

Factory evidence confirms the surface pattern:

- One functional core should project into API, CLI, MCP, SDK, skill, and any
  human face.
- MCP plus SDK is the repeated fleet gap.
- MCP tools should represent agent intent, not auto-wrapped endpoints.

## Recommended Shape

Build the smallest credible evidence kernel:

```text
sources -> adapters -> local store/index -> retrieval profiles
        -> evidence session -> context packet -> CLI / MCP / API / SDK / skill
```

The product surface is intentionally small:

```sh
exo init
exo ingest <path|url|stdin> [--source <id>]
exo search "query" [--profile <profile>]
exo fetch <source-ref>
exo packet "question" [--profile <profile>] [--out <dir>]
exo explain --html <packet>
```

Everything else exists to make those commands trustworthy, inspectable, and
available to agents.

## Conceptual Repo-Local State

This packet does not create runtime state, but the future default should be easy
to inspect:

```text
.exocortex/
  config.yaml            # sources, profiles, index settings
  objects/               # normalized source refs, receipts, fetched metadata
  index/                 # embedded full-text and vector index state
  packets/               # generated packets when --out is not supplied
  logs/evidence.jsonl    # append-only retrieval and ingest receipts
```

The user should be able to crack the hood without a hosted dashboard or private
scripts.

## Public Core Versus Private Consumers

| Concern | Public Exocortex core | Private consumer |
|---|---|---|
| Identity | Local-first context evidence core | Domain command center or corpus owner |
| Data | Fixtures, schemas, generic adapter code, generated receipts | Real personal, business, client, project, or relationship data |
| Config | Source/profile schema, capability boundaries, safe defaults | Actual roots, commands, credentials, policies, and ranking preferences |
| Search | Embedded full-text/vector search, generic command adapter contract | Private retrieval commands and corpus-specific ranking |
| Ingest | Files, Markdown, URLs, stdin, and generic command output | Private firehoses, inboxes, archives, and writeback rules |
| Skills | One generic context skill | Domain-specific rituals, voice, review, or outbound behavior |
| MCP | Three read-only intent tools over the public core | Registered private server/profile config and source permissions |
| Inspection | Packet files, receipts, and `exo explain --html` | Private evidence archives and redacted reporting |

The boundary rule: public Exocortex may know that a generic command adapter
exists. It must not know a private consumer's collection names, roots, folders,
or policies.

## Closed Decisions

1. **Vector backend:** commit to LanceDB as the single v0 backend. No storage
   trait until a second backend earns the abstraction.
2. **Private retrieval tool boundary:** public v0 ships a generic command
   adapter that parses configured command output into evidence chunks. Private
   consumers keep command names and collection config outside this repo.
3. **Packet schema:** freeze the citation core as `citation/v1`; keep the
   packet body versioned as `packet_version: 0` and allowed to churn until real
   migration pressure stabilizes it.
4. **Subsets:** cut cross-workspace inheritance from v0. Subsets are profiles
   owned by the source workspace.
5. **MCP:** v0 MCP is read-only with three tools:
   `build_context_packet`, `search_context`, and `fetch_source`.
6. **Evidence destinations:** public Exocortex defines receipt format and
   fetchability. Private consumers decide where private receipts live.
7. **Skill pack:** ship one generic skill: build a cited context packet before
   acting. Domain skills stay with consumers.
8. **UI:** no v0 product UI. Packet files, JSONL receipts, and
   `exo explain --html` are the inspector until a real gap proves otherwise.
9. **Packet premise:** `exo packet` is an evidence-session compiler as well as a
   one-shot packet assembler. Iterative search/fetch remains first-class.

## v0 Surface

### CLI Surface

- `exo init`
- `exo sources list|add|doctor`
- `exo ingest <path|url|-> --source <id> [--profile <profile>]`
- `exo search "query" --profile <profile> [--json|--plain]`
- `exo fetch <source-ref> [--json|--plain]`
- `exo packet "question" --profile <profile> [--out <dir>] [--json]`
- `exo explain <packet-or-ref> [--html]`

Primary users are humans at a terminal, scripts, and agents shelling out.
Outputs default to human-readable summaries, with `--json` for machines and
packet directories when `--out` is supplied. Prompts are allowed only on a TTY;
`--no-input` disables prompts.

### MCP Tools

MCP exposes agent-intent tools backed by the same core path as the CLI:

- `build_context_packet(question, profile, constraints)`: returns packet
  content, citations, gaps, exclusions, and receipt path.
- `search_context(query, profile, constraints)`: returns ranked evidence chunks
  with source decisions.
- `fetch_source(source_ref)`: hydrates a cited source or chunk.

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

SDKs should make common flows concise, but they should not add behavior that the
CLI/MCP cannot exercise.

## Built-In Substrate

Exocortex should be thin, but not hollow. v0 should ship a local stack that works
without asking the user to design retrieval infrastructure:

- source registry with capabilities, trust, privacy, freshness, auth mode, and
  roots
- local object metadata and fetch receipts
- LanceDB-backed embedded full-text search
- LanceDB-backed vector search with local embeddings after the eval clears the
  gate
- local file and Markdown ingest
- URL and stdin ingest
- generic command-output adapter
- packet builder with citations, source decisions, exclusions, freshness, and
  gaps
- JSONL evidence log
- source doctor and `exo explain --html`

Backend flexibility comes later through adapters only after a second backend
proves it is worth the surface.

## Core Domain Model

```text
Workspace
  id, root, config_version, default_profile

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

Keep this model stable at the citation and policy boundaries, but do not freeze
every internal packet field before the validation experiment and first private
consumer teach the design what matters.

## First Profiles

- `repo_grounded`: current repo, docs, backlog, local files, exact search, and
  only explicitly configured external/private sources.
- `public_safe`: excludes private sources and secrets by default; suitable for
  public docs, PRs, and external sharing.
- `private_context`: allows private command-center sources when configured by
  the consumer.

Avoid a large taxonomy in v0. Profiles should be a small policy layer, not a
workflow language.

## First Consumer Boundary

The first private consumer should prove the read path without moving data or
publishing private config:

1. Keep data, source roots, collection names, credentials, and receipts private.
2. Configure private sources outside this repo through generic adapter fields.
3. Run read-only context gathering side-by-side with the incumbent path.
4. Record private receipts in the consumer workspace.
5. Register MCP only after CLI parity and fetchable citations are proven.
6. Move consumer skills by boundary: Exocortex ships the generic context skill;
   domain voice, rituals, and writeback stay private.

## Alternatives Considered

| Option | Why it helps | Why it fails | Verdict |
|---|---|---|---|
| Ad hoc Markdown conventions | Lowest setup cost and already familiar | No shared API/MCP/SDK surface, no evidence receipts, weak privacy/freshness policy | Reject as the baseline to beat |
| Publish a private workspace as the product | Fast path from a working system | Leaks private assumptions and makes personal policy look generic | Reject |
| Full RAG/vector platform | Familiar category, connector-rich, easy to explain | Competes on commodity infrastructure and bloats past the evidence core | Reject |
| MCP wrapper over files and private commands | Quick agent access | Shallow pass-through with no source governance, parity, or packet contract | Reject |
| Thin context core with baked-in local index | Beats Markdown while staying composable | Requires disciplined v0 scope and explicit private boundaries | Choose |

## Oracle

This frozen-experimental scrub is done when:

- `VISION.md` and this packet no longer publish private topology.
- The eight questions from the previous packet are closed as decisions, not left
  open.
- `docs/experiments/retrieval-validation.md` defines the validation experiment
  that must run before implementation resumes.
- `docs/shaping/public-context-system.html` carries the same scrubbed decisions
  as this markdown file.
- No build scaffold, runtime state, or sample private config is introduced.

First implementation PR oracle, for later:

- The retrieval validation experiment has reported an accepted result.
- Fixture repo runs `exo init`.
- Fixture Markdown/URL/stdin sources run through `exo ingest`.
- `exo search "query" --profile repo_grounded --json` returns cited evidence
  chunks.
- `exo packet "question" --profile repo_grounded --out evidence/` writes
  `packet.json` and `evidence.jsonl`.
- MCP `build_context_packet`, `search_context`, and `fetch_source` produce
  equivalent results from the same core.
- `public_safe` fixture excludes private-only sources.
- `exo explain --html` shows included/excluded source decisions and freshness
  gaps.

## Verification System

- Claim: the docs define a public Exocortex evidence core without leaking
  private topology, leaving questions open, or scaffolding implementation.
- Falsifier: the boundary grep finds private terms, the decisions remain open,
  or the diff introduces runtime code.
- Driver: repo-wide boundary grep, markdown/html inspection, experiment doc
  inspection, `git diff --check`, and git status.
- Grader: oracle items pass from a clean checkout.
- Evidence packet: command transcript plus the merged PR URL.
- Cadence: run before merge; keep the boundary grep as the first check for
  public docs changes.
- Gap/waiver: no live `exo` binary exists yet, so implementation behavior is not
  claimed in this PR.

## Premise Sources

- Factory shared lane brief, redacted source hash
  `sha256:fdbf950fca898db1eb0036fa83c291e090c4825d15a32fa8205c26d072cf79fa`.
- Factory synthesis report, redacted source hash
  `sha256:b8a981522941a7623b3310ebdb28e991f684d254261b097fb6f094723ffe7be4`.
- Exocortex groom teardown report and operator decisions overlay, read from the
  factory lane context for this 2026-07-01 groom cycle.

## Risks And Mitigations

- **Private-consumer overfit:** keep private roots, commands, policies, and
  receipts out of the public repo.
- **Too thin to beat Markdown:** ship receipts, exclusions, fetchable
  citations, profiles, and MCP parity; retrieval quality must pass the eval.
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
- **Unsafe self-organization:** v0 has no organization write path; mutation
  needs explicit future capability and approval design.
