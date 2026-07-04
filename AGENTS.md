# AGENTS.md — Exocortex

Exocortex is the public, local-first evidence core for agent-readable
context: source registry, adapters, indexes, retrieval, packet assembly,
receipts, and policy behind one core, projected through CLI, MCP, HTTP API,
SDKs, and a skill. See `VISION.md` for the full product shape and the
evidence-contract thesis (fetchable citations, source-decision receipts,
exclusions, freshness labels).

Status: **experimentation-open, productization-gated**. Bounded prototypes,
fixtures, and private-data experiments may land when tied to an explicit
evidence oracle. Product runtime, stable public APIs, SDK/MCP commitments, and
broad scaffolding remain gated until the retrieval experiment validates the
premise on real private data — see `backlog.d/002` for the validation contract.

## Build / verify

- No build or test tooling exists yet (no package manifest, no
  `scripts/verify.sh`). Prototype scripts are allowed only as experiment
  drivers and must stay out of product-runtime shape.
- The standing gate for this phase is the public-boundary grep oracle in
  `backlog.d/001-public-boundary-and-frozen-shape-closure.md`: no private
  paths, hostnames, or tool names may leak into public docs.
- CI: `.github/workflows/landmark-release.yml` only synthesizes release
  notes on GitHub release publish — it is not a code gate.

## Layout

- `VISION.md` — product shape and closed/open shape decisions.
- `backlog.d/` — numbered backlog items; current front is retrieval
  validation before any product runtime scaffold.

No repo-local `.agents/skills/` present.
