# Exocortex

Foundation-shaped memory kernel for the Misty Step agent fleet: one binary
that gives every agent a uniform read/write/search interface over registered
knowledge corpora.

**Status: v0.2 on master.** Built in Go with sole-publisher isolation,
fail-closed reads, compare-and-swap concurrency, hybrid semantic retrieval,
and executive orientation briefs (`brief`). The CLI and bundled skill
(`skill://exocortex`) is the universal fleet baseline.
## Model

- **Cortex** — a writable markdown+git corpus registered with the kernel.
  Daybook is the first. Full read/write/search/lint/lineage over plain files
  humans can still read and diff.
- **Feed** — an adapter that ingests a foreign source (Notion, Google Drive,
  harness session logs) into a cortex as compiled, provenance-stamped notes
  with `source:` lineage. Raw sources stay where they are.

## Surface

```sh
exocortex register daybook ~/Development/misty-step/daybook
exocortex brief "<topic>"                               # executive orientation briefing
exocortex search "<query>" [--type decision|memo|session] # hybrid semantic search (BM25 + Qwen rerank)
exocortex get <path>                                    # read from committed Git HEAD snapshot
exocortex note "<thought>"                              # atomic memo capture (~2s)
exocortex put <path> --from draft.md                    # create-only (fails if path exists)
exocortex put <path> --from draft.md --expects <sha>    # update: stored-revision hash REQUIRED
exocortex log <path>                                   # git lineage
exocortex lint [<path>]                                # frontmatter floor gate
```

## Docs

- [SPEC.md](SPEC.md) — full design: decisions, contracts, delivery plan
- [VISION.md](VISION.md) — why this exists
- [ADR-0001](docs/adr/0001-exocortex-kernel.md) — accepted kernel decision
- [skills/exocortex/SKILL.md](skills/exocortex/SKILL.md) — bundled agent skill
  (canonical copy installed into omp-config)

## Provenance

This repo previously hosted an earlier, unrelated "evidence core" concept
(citable context packets, 2026-07). On 2026-08-21 the operator repurposed the
repository for the memory kernel. The prior tree remains recoverable from git
history; `LICENSE` was retained.
