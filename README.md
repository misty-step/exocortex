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
exocortex search "<query>" [--mode hybrid|bm25|vector] [--type decision|memo|session] # omitted mode is hybrid; pass bm25 for deterministic
exocortex get <path>                                    # read from committed Git HEAD snapshot
exocortex note "<thought>"                              # atomic memo capture (~2s)
exocortex put <path> --from draft.md                    # create-only (fails if path exists)
exocortex put <path> --from draft.md --expects <sha>    # update: stored-revision hash REQUIRED
exocortex log <path>                                   # git lineage
exocortex lint [<path>]                                # frontmatter floor gate
exocortex sync [--cortex <name>]                       # refresh QMD index and embeddings
exocortex status [--cortex <name>]                     # dirty markers, last sync, last error
```

## Develop

Clone-to-green:

```sh
./scripts/check.sh
```

That is the owned gate: `gofmt`, `go vet`, `golangci-lint` cyclop ≤ 15
(failing fuse), gitleaks, govulncheck, module token budgets,
`go test -race`, and a CLI smoke (`register` → `put` → `get` → `lint`
on a temp cortex). `scripts/lint-report.sh` prints gocognit, nestif,
funlen, maintidx, staticcheck, errcheck, and dupl without failing.
Go 1.26.6 is required (`go.mod`); 1.26.5 stdlib is govulncheck-red.



Install the binary so `go version -m` names this module:

```sh
go install -trimpath ./cmd/exocortex
```

The user timer runs `~/.local/bin/exocortex`. Rebuild after merge. Roll
back by installing the previous commit the same way.

```sh
./scripts/install-hooks.sh   # core.hooksPath=.githooks
```


## Docs

- [SPEC.md](SPEC.md) — full design: decisions, contracts, delivery plan
- [VISION.md](VISION.md) — why this exists
- [ADR-0001](docs/adr/0001-exocortex-kernel.md) — accepted kernel decision
- [skills/exocortex/SKILL.md](skills/exocortex/SKILL.md) — bundled agent skill
  (source of truth; generate omp-config with `scripts/install-skill.sh`)

Clone-to-green: `go test ./...`. Generate a dest copy with `scripts/install-skill.sh <dest-dir>` only. omp-config dest drift fails `skills/exocortex/check-source.sh` there.

## Provenance

This repo previously hosted an earlier, unrelated "evidence core" concept
(citable context packets, 2026-07). On 2026-08-21 the operator repurposed the
repository for the memory kernel. The prior tree remains recoverable from git
history; `LICENSE` was retained.
