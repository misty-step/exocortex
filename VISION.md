# Exocortex Vision

Exocortex is a public context engine for agent-first work. It turns many local
and remote sources into cited, inspectable context packets that an agent can
use without guessing what matters, where it came from, or whether it is safe to
trust.

It is not "all of Phaedrus's context." It is the reusable program that a private
system such as Daybook can consume, configure, and extend. Daybook stays the
personal vault and command center. Exocortex supplies the public machinery:
source adapters, normalized evidence, retrieval policy, citation discipline,
and agent-facing surfaces.

## The Job

Agents already have access to files, tools, search, inboxes, voice transcripts,
repos, and notes. Access is no longer the scarce part. The scarce part is a
small, composable system that can answer:

- Which sources should be consulted for this kind of question?
- What exact evidence was retrieved?
- How fresh, private, and trustworthy is it?
- Can another agent fetch the same source again?
- Did the final context packet cite reality or just summarize memory?

Exocortex exists to make context an evidence product, not a prompt accident.

## Design Philosophy

Exocortex should be boring in its core and opinionated at its boundaries.

- **One core, many faces.** The retrieval and evidence model lives once. CLI,
  HTTP API, MCP server, SDK, bundled skill, and any thin UI are projections of
  that same core.
- **Rust by default.** Durable logic belongs in Rust. Non-Rust surfaces are
  allowed only where an ecosystem boundary earns it, such as generated SDKs or
  a small web UI.
- **Source adapters are ports, not product identity.** A Markdown vault, QMD,
  repo files, Monologue exports, web search, Gmail, or GitHub are replaceable
  adapters behind the same evidence contract.
- **Provenance beats clever ranking.** Every useful result carries a source
  reference, retrieval time, freshness, trust label, privacy label, and enough
  metadata to re-fetch or reject it.
- **Rich context for models, structure for code.** Do not over-normalize prose
  just because an LLM will read it. Add rigid fields only where deterministic
  code must branch: privacy, trust, freshness, source identity, citation refs,
  and capabilities.
- **Read-only by default.** Retrieval and packet generation are safe. Ingest,
  sync, writeback, deletion, and outward publication are separate capabilities
  with explicit configuration and approval boundaries.
- **Triggers over loops.** The long-term shape is event-driven ingest and
  re-indexing. Polling is acceptable only as a cheap bridge with clear
  freshness metadata.

## What Belongs Here

Public Exocortex owns:

- Source registry and capability declarations.
- Evidence and citation schemas.
- Retrieval profiles and source-selection policy.
- Adapter traits and a small set of reusable adapters.
- Packet builder that emits compact, cited context for agents.
- CLI, API, MCP, SDK, and skill surfaces over the same core.
- Verification fixtures that prove privacy boundaries, citation stability, and
  surface parity.

Private consumers own:

- Actual personal or business data.
- Source-specific configuration and secrets.
- Personal workflows, rituals, voice, relationship context, and command-center
  behavior.
- Writeback policy into private systems.
- Human-facing dashboards that are meaningful only for that private domain.

Daybook should be the first demanding consumer, not the product boundary.

## What Exocortex Refuses

Exocortex is not a universal RAG platform, a vector database project, an
Obsidian replacement, a private memory dump, a scheduler, or an agent runner. It
does not decide what a person should do. It does not hide citations behind a
chat answer. It does not ship private Daybook assumptions as public defaults.

If a feature cannot be explained as "make evidence from configured sources more
available, trustworthy, reusable, or inspectable," it probably belongs
elsewhere.

## Excellence Bar

For v0, excellence is a small contract that works end to end:

- A downstream repo can declare sources.
- A caller can ask for a context packet.
- Exocortex retrieves from at least one semantic source, one exact-search source,
  and one append-only firehose.
- The packet includes citations and freshness metadata.
- CLI and MCP expose the same behavior.
- A fixture proves private sources do not leak into public-safe profiles.

For v1, excellence is composability:

- New sources are adapter additions, not new product branches.
- SDK and MCP users can build against stable contracts.
- Agents can request context by intent without loading every connector schema.
- Consumers can inspect why a source was included, excluded, or downweighted.

For the long run, excellence is trust:

- The system is useful enough that agents ask it before guessing.
- The evidence log lets a future agent audit decisions without chat history.
- Private consumers can adopt it without publishing their lives.
- Public users can run it locally without signing up for a hosted platform.

The public promise is simple: bring your sources; get cited context packets;
keep your private world private.
