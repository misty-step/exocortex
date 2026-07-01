# Exocortex Vision

Exocortex is the public, self-hosted context core for agent-first command
centers.

It competes with the default solution people already use: scattered Markdown,
ad hoc folders, half-remembered scripts, and whatever context an agent happens
to read this time. The win is not to become a giant app. The win is to make the
better path just as easy: initialize a repo, declare sources, ingest or index
them, and ask for a cited context packet through the same core from CLI, MCP,
API, SDK, skills, or a thin UI.

Daybook is the first demanding consumer, not the product boundary. Daybook stays
private: the vault, QMD collection, personal policies, traces, people notes,
finance context, rituals, and writeback rules remain there. Exocortex is the
reusable program Daybook consumes: source registry, adapters, local indexes,
embeddings, evidence logs, profiles, packets, and agent-facing tools.

## What It Is For

Agents need a smaller, more trustworthy way to get the right context than
"search everything and hope." Humans need to inspect, correct, and extend the
same system without learning an agent's private prompt rituals.

Exocortex answers a few durable questions:

- What sources exist for this workspace?
- Which sources are allowed for this question and profile?
- What exact evidence was retrieved, and can it be fetched again?
- Is the evidence fresh, private, trusted, or stale?
- What compact packet should an agent read before acting?
- What did the system exclude, and why?

The user is the first-class consumer. Agents are delegated consumers. Every
surface should expose the same capability: use context, manage context, inspect
context, and cite context.

## Product Shape

Exocortex should be thin at the product surface and serious in the substrate.

- **One core, many faces.** The core owns source registry, ingest, indexes,
  retrieval, packet assembly, evidence logs, and policy. CLI, MCP, HTTP API,
  SDKs, skills, and any UI are projections over that same core.
- **Local-first and self-hosted.** A single repo can carry its own
  `.exocortex/` state. Service mode, hosted vector stores, or remote workers are
  optional deployment choices, not the default premise.
- **Batteries included, not platform swollen.** v0 should include local file and
  Markdown ingest, URL/stdin ingest, embedded full-text and vector search,
  local embeddings, QMD integration where available, and packet generation. It
  should not require a user to design a retrieval stack before the first useful
  result.
- **Profiles over workflows.** Deterministic code owns privacy, trust,
  freshness, capability checks, and citations. LLMs receive rich evidence, not a
  maze of rigid schemas. Use profiles such as `repo_grounded`, `public_safe`,
  `personal_context`, and `deep_research` instead of a workflow DSL.
- **Agent-native, not agent-magical.** MCP tools should be intent-shaped:
  `build_context_packet`, `search_context`, `fetch_source`, `ingest_source`,
  and `organize_context`. They should call the same core as `exo packet`,
  `exo search`, and `exo ingest`.
- **Inspectable by default.** Every packet carries citations, source decisions,
  freshness, gaps, and an evidence receipt. A future agent should not need chat
  history to audit a decision.
- **Manual and automatic.** Users can upload, ingest, tag, exclude, rebuild, and
  organize by hand. Agents can suggest organization and run approved ingest
  paths. Automated organization starts as suggestions; mutation requires an
  explicit capability boundary.

## Public Core Versus Private Consumers

Public Exocortex owns:

- Workspace initialization and repo-local context state.
- Source declarations, profiles, privacy/trust/freshness policy, and overlays.
- Ingest adapters for files, Markdown, URLs, stdin, transcript/event JSONL, and
  command-backed semantic search such as QMD.
- Embedded metadata, full-text, vector, and relationship indexes.
- Evidence chunks, source refs, context packets, citations, and JSONL receipts.
- Surface parity across CLI, MCP, API, SDK, bundled skills, and an optional thin
  evidence inspector UI.
- Fixtures and gates proving citations are fetchable, private profiles do not
  leak, and CLI/MCP/API behavior agrees.

Private consumers own:

- Real personal, business, client, and relationship data.
- Secrets, source roots, collection names, and auth config.
- Personal ranking preferences, rituals, voice, writeback policy, and command
  center behavior.
- Domain-specific dashboards, automations, and outbound actions.

Daybook uses Exocortex. Adminifi may use a narrower overlay of Daybook or its
own workspace. Neither should force private assumptions into the public core.

## What Exocortex Refuses

Exocortex is not a chat app, an Obsidian replacement, a hosted SaaS premise, a
workflow engine, a scheduler, an agent runner, or a vector database product.
It can ship an embedded vector index, but the product is not "a vector DB." It
can support research and ideation skills, but the core is not a research agent.
It can expose a UI, but the UI is not the command center.

If a feature cannot be explained as making configured context easier to ingest,
search, assemble, cite, inspect, or safely reuse, it probably belongs in a
consumer repo.

## v0 Excellence

v0 is excellent when a new repo can run this loop:

```sh
exo init
exo ingest ./notes --source notes
exo ingest https://example.com/spec --source web
exo search "what matters for this task?" --profile repo_grounded
exo packet "prepare an implementation plan" --profile repo_grounded --out evidence/
```

The same packet must be available through MCP with `build_context_packet`.
The packet must include fetchable citations, freshness metadata, privacy/trust
labels, source decisions, and gaps. The repo must remain understandable enough
that a user can crack open the config or index receipts and fix what happened.

The first implementation should prove:

- `exo init`, `exo ingest`, `exo search`, and `exo packet`.
- MCP parity for packet building, search, fetch, ingest, and organization
  suggestions.
- Local embedded search with full-text plus vector search by default.
- QMD as an optional command adapter, with Daybook keeping private collection
  config outside the public repo.
- A privacy fixture where `public_safe` excludes private-only sources.
- A Daybook migration path that replaces context-gathering calls before moving
  or rewriting any private data.

## Long-Term Bet

The long-term bet is that every serious agent-run workspace will need a local
context system: not a memory dump, not a chat transcript pile, but a composable
evidence layer that both humans and agents can operate.

The public promise stays simple: bring your sources, keep your private world
private, and get cited context packets through whatever interface your agent or
workflow needs.
