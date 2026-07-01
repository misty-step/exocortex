# Exocortex Vision

Exocortex is the public, local-first evidence core for agent-readable context.
It exists so an agent can ask for context without rummaging through a private
world, and so a human can audit exactly what the agent saw, did not see, and
could fetch again.

The competing baseline is not another vector database. The baseline is a folder
of Markdown, scattered scripts, and whatever an agent happens to read this time.
Exocortex wins only if the better path is nearly as easy: initialize a workspace,
declare sources, search, fetch, and compile a cited packet through the same core
from CLI, MCP, API, SDK, and a shipped skill.

The repo is frozen-experimental until the retrieval experiment proves the
premise on real private data. Before runtime code lands, the public boundary
must be clean, the shape decisions must be closed, and the validation contract
must be concrete enough for Crucible to grade.

## What It Is For

Agents need smaller, safer context than "search everything and hope." Humans
need evidence they can inspect without reading a chat transcript or trusting a
prompt convention.

Exocortex answers a few durable questions:

- What sources exist for this workspace?
- Which sources are allowed for this profile and question?
- What evidence was retrieved, and can it be fetched again?
- Is the evidence fresh, trusted, private, or stale?
- What did the system exclude, and why?
- What packet should an agent read before acting?

The differentiated asset is the evidence contract: fetchable citations,
source-decision receipts, exclusions, gaps, freshness labels, and enough
deterministic policy to prove that a private profile stayed private. Retrieval
quality matters, but the product is not a memory extraction layer and not a
hosted agent framework.

## Product Shape

Exocortex should be thin at the surface and serious in the substrate.

- **One core, many faces.** The core owns source registry, adapters, indexes,
  retrieval, packet assembly, receipts, and policy. CLI, MCP, HTTP API, SDKs,
  skills, and any later inspector are projections over that same core.
- **Local-first by default.** Workspace state lives under repo-local
  `.exocortex/` unless a user explicitly chooses a service deployment. Hosted
  storage is not the premise.
- **Batteries included, not platform swollen.** v0 chooses one embedded store
  rather than hiding behind a speculative storage trait. LanceDB is the default
  for full-text, vector search, filtering, and versioned local index state.
- **Generic adapters over private glue.** Public adapters handle files,
  Markdown, URLs, stdin, and configured commands that emit evidence chunks.
  Private consumers own the actual commands, roots, collections, policies, and
  secrets.
- **Profiles over workflow DSLs.** Deterministic code owns privacy, trust,
  freshness, capability checks, citations, and branching policy. LLMs receive
  rich evidence, not a maze of rigid schemas.
- **Read-only agent surface first.** v0 MCP has three tools:
  `build_context_packet`, `search_context`, and `fetch_source`. Ingest,
  organization, and writeback stay CLI or future capability-gated flows until a
  real consumer proves the need.
- **Evidence sessions, not only one-shot packets.** `exo packet` can assemble a
  packet for a simple question, but it should also compile receipts from an
  interactive search/fetch session into a citable artifact.
- **Inspectable without a product UI.** Packet files, JSONL receipts, and
  `exo explain --html` are the v0 inspector. A live UI waits for usage to reveal
  a gap those artifacts cannot cover.

## Public Core Versus Private Consumers

Public Exocortex owns:

- Workspace initialization and repo-local context state.
- Source declarations, retrieval profiles, privacy/trust/freshness machinery,
  and safe defaults.
- Generic adapters for files, Markdown, URLs, stdin, and command output.
- LanceDB-backed local full-text/vector index state plus local embeddings where
  practical.
- A stable `citation/v1` source reference contract with checksums, observation
  time, and fetch hints.
- Versioned context packets, source decisions, gaps, exclusions, and JSONL
  receipts.
- CLI/MCP/API/SDK parity fixtures and privacy fixtures proving public-safe
  profiles do not leak excluded sources.

Private consumers own:

- Real personal, business, client, relationship, and project data.
- Source roots, command strings, collection names, secrets, and credentials.
- Ranking preferences, rituals, voice, writeback policy, and outbound actions.
- Domain dashboards, automations, and private evidence archives.

Subsets are profiles owned by the source workspace. Exocortex should not grow a
cross-workspace inheritance language until a concrete consumer proves that a
profile is insufficient.

## What Exocortex Refuses

Exocortex is not a chat app, an Obsidian replacement, a hosted SaaS premise, a
workflow engine, a scheduler, an agent runner, or a vector database product. It
can ship a local index, but the product is not "a vector DB." It can feed
research or ideation agents, but the core is not a research agent.

It also refuses private-consumer overfit. A public feature must make configured
context easier to ingest, search, fetch, assemble, cite, inspect, or safely
reuse. If it mainly exposes one private workspace's topology, it belongs outside
this repo.

## v0 Excellence

v0 is excellent only after the retrieval experiment says the premise is worth
building. The target loop is:

```sh
exo init
exo ingest ./notes --source notes
exo search "what matters for this task?" --profile repo_grounded
exo packet "prepare an implementation plan" --profile repo_grounded --out evidence/
exo explain --html evidence/packet.json
```

The same search, packet, and fetch behavior must be available through MCP. The
packet must include fetchable citations, freshness metadata, privacy/trust
labels, source decisions, exclusions, and gaps. The repo must stay simple enough
that a user can open the config and receipts to understand what happened.

The first implementation should prove:

- `exo init`, `exo ingest`, `exo search`, `exo packet`, `exo fetch`, and
  `exo explain`.
- A frozen `citation/v1` source reference contract while packet internals remain
  versioned and allowed to churn.
- One LanceDB-backed local index with full-text first, then local semantic
  retrieval when the eval justifies it.
- Three read-only MCP tools over the same core path as the CLI.
- A generic command adapter with private configuration kept out of the public
  repo.
- A public-safe privacy fixture that proves excluded sources stay excluded.
- JSONL receipts that record both inclusions and exclusions.

## Long-Term Bet

The long-term bet is that every serious agent-run workspace will need a local
context system: not a memory dump, not a chat transcript pile, but a composable
evidence layer that both humans and agents can operate.

The public promise stays simple: bring your sources, keep your private world
private, and get cited, replayable context through whatever interface your agent
or workflow needs.
