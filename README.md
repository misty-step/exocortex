# Exocortex

Exocortex is a public, local-first evidence core for agents that need smaller,
safer context than "search everything and hope": it is meant to compile
policy-filtered, replayable context packets with fetchable citations,
freshness labels, source decisions, exclusions, and gaps. It is not another
vector database, not a chat app, and not a hosted agent framework; the product
is the auditable evidence contract around what context was included, excluded,
and can be fetched again.

## Current Status

This repository is experimentation-open and productization-gated. Small
fixtures and bounded prototypes may land when they prove the evidence contract,
but product runtime, stable public APIs, SDK/MCP commitments, and broad
scaffolding are blocked on the retrieval-validation experiment in
[docs/experiments/retrieval-validation.md](docs/experiments/retrieval-validation.md).

The concrete proof-of-shape artifact today is
[tests/fixtures/first-packet/](tests/fixtures/first-packet/):

- [source-registry.json](tests/fixtures/first-packet/source-registry.json)
  declares the sources, evidence, exclusions, gaps, and citation anchors.
- [evidence-packet.md](tests/fixtures/first-packet/evidence-packet.md) is the
  generated packet.
- [scripts/build-first-packet.py](scripts/build-first-packet.py) regenerates
  the packet and validates local file-line anchors plus citation checksums.

Run the current repository gate with
[scripts/verify.sh](scripts/verify.sh):

```sh
./scripts/verify.sh
```

## Aspirational v0

The target product loop below is not implemented yet. There is no working
`exo` binary in this repository. This shape is tracked in
[backlog.d/003-build-contract-first-kernel-after-eval.md](backlog.d/003-build-contract-first-kernel-after-eval.md)
and remains blocked on the retrieval-validation experiment.

```sh
exo init
exo ingest ./notes --source notes
exo search "what matters for this task?" --profile repo_grounded
exo packet "prepare an implementation plan" --profile repo_grounded --out evidence/
exo explain --html evidence/packet.json
```

## Where To Read Next

- [VISION.md](VISION.md) explains the product thesis, public/private boundary,
  and frozen/open shape decisions.
- [AGENTS.md](AGENTS.md) records the repo contract, CI checks, and agent
  guardrails.
- [backlog.d/](backlog.d/) holds the current backlog and blocked
  productization work.
- [docs/experiments/retrieval-validation.md](docs/experiments/retrieval-validation.md)
  defines the experiment that decides whether implementation unfreezes.
