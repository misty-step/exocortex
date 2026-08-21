# Exocortex

Foundation-shaped memory kernel for the Misty Step agent fleet: one binary
that gives every agent a uniform read/write/search interface over registered
knowledge corpora.

**Status: spec-seeded, unbuilt.** The v0 build is Powder card `exocortex-v0`.
Everything below describes the agreed contract, not shipped code.

## Model

- **Cortex** — a writable markdown+git corpus registered with the kernel.
  Daybook is the first. Full read/write/search/lint/lineage over plain files
  humans can still read and diff.
- **Feed** — an adapter that ingests a foreign source (Notion, Google Drive,
  harness session logs) into a cortex as compiled, provenance-stamped notes
  with `source:` lineage. Raw sources stay where they are.

## Planned surface (v0)

```sh
exocortex register daybook ~/Development/misty-step/daybook
exocortex put <file> [--expects <sha>]   # validates, stamps, applies cortex VCS policy
exocortex get <path>
exocortex search "<query>" --json        # shells qmd
exocortex log <path>                     # git lineage
exocortex lint [<path>]                  # frontmatter floor gate
exocortex mcp                            # stdio MCP server, same operations
```

## Docs

- [SPEC.md](SPEC.md) — full design: decisions, contracts, delivery plan
- [VISION.md](VISION.md) — why this exists
- [ADR-0001](docs/adr/0001-exocortex-kernel.md) — accepted kernel decision
- [skills/exocortex/SKILL.md](skills/exocortex/SKILL.md) — bundled agent skill
  (canonical copy; omp-config installs it in slice 2)

## Provenance

This repo previously hosted an earlier, unrelated "evidence core" concept
(citable context packets, 2026-07). On 2026-08-21 the operator repurposed the
repository for the memory kernel. The prior tree remains recoverable from git
history; `LICENSE` was retained.
