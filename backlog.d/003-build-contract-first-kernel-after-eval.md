# Build the contract-first kernel after the eval clears the gate

Priority: P1 · Status: blocked · Estimate: L

## Goal
Implement the smallest useful Exocortex core only after the retrieval validation
experiment shows that the evidence contract is worth building.

## Oracle
- [ ] The retrieval validation experiment has a recorded result that satisfies its implementation-unfreeze threshold.
- [ ] A fixture repo can run `exo init`, ingest local files, run `exo search`, build a packet, fetch a cited source, and render `exo explain --html`.
- [ ] The first public schema freezes `citation/v1` source references while `packet_version: 0` remains explicitly allowed to churn.
- [ ] JSONL receipts record included sources, excluded sources, freshness, gaps, and fetch hints.
- [ ] CLI and MCP packet/search/fetch paths are backed by the same core and pass a parity fixture.

## Verification System
- Claim: the first implementation proves the evidence contract before expanding into semantic retrieval or consumer migration.
- Falsifier: implementation starts without an eval result, citations cannot be fetched, receipts omit exclusions, or CLI/MCP behavior diverges.
- Driver: repo fixture, CLI replay, MCP replay, privacy fixture, and `exo explain --html` artifact inspection.
- Grader: tests plus rendered evidence packet inspection from the fixture repo.
- Evidence packet: fixture output directory committed or attached to the PR.
- Cadence: every kernel PR.

## Children
1. Add the Rust workspace and `exo` CLI with `init`, local ingest, `search`, `packet`, `fetch`, and `explain`.
2. Add the frozen `citation/v1` source reference contract with checksum, observation time, and fetch hint.
3. Add LanceDB-backed local full-text search as the first embedded index.
4. Add generic file, Markdown, URL, stdin, and command-output adapters.
5. Add profiles `repo_grounded` and `public_safe` with a privacy fixture.
6. Add JSONL evidence receipts for inclusions, exclusions, gaps, and freshness.
7. Add the read-only MCP server with `build_context_packet`, `search_context`, and `fetch_source`.
8. Add the repo gate that runs the fixture and parity checks.

## Notes
- Blocked until `002-define-retrieval-validation-experiment.md` has produced and reported an accepted result.
- Do not add a storage trait before a second backend earns it.
