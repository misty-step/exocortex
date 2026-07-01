# Scrub the public boundary and close the frozen shape decisions

Priority: P0 · Status: done · Estimate: M

## Goal
Make the public repo safe to read and frozen-experimental by removing private
topology from public docs and recording the operator's closed shape decisions
before any runtime implementation begins.

## Oracle
- [ ] `rg -n -i "ph[a]edrus|/U[s]ers/|d[a]ybook|m[o]nologue|s[u]per whisper|finance[_]private|relationship[_]private|q[m]d" . --glob '!.git/**'` returns no matches.
- [ ] `docs/shaping/public-context-system.md` records the closed decisions: LanceDB as the single v0 backend, generic command adapter, frozen citation core, no cross-workspace inheritance, three read-only MCP tools, private evidence destinations, one generic skill, no v0 UI, and packet-as-evidence-session compiler.
- [ ] `docs/shaping/public-context-system.html` is regenerated from the markdown and carries the same decisions.
- [ ] No implementation scaffold, build system, runtime state, or sample private config is introduced.

## Verification System
- Claim: the public docs no longer leak private topology and the experimental shape is closed enough to evaluate.
- Falsifier: the grep oracle finds a private term, the open questions remain framed as open, or the diff introduces runtime code.
- Driver: targeted `rg` checks, markdown/html inspection, `git diff --check`, and git status.
- Grader: each oracle item passes from a clean checkout.
- Evidence packet: command transcript plus the merged PR.
- Cadence: run on the scrub PR before merge, then keep the grep as the first check for public docs changes.

## Children
1. Replace absolute local paths and private-source names in the shaping packet with source hashes or generic evidence labels.
2. Remove the private-consumer migration section from public docs; keep only a short first-consumer boundary note.
3. Replace private-tool-named adapter prose with a generic command adapter contract.
4. Delete the cross-workspace inheritance example; record that subsets are profiles owned by the source workspace.
5. Reduce the first MCP surface to `build_context_packet`, `search_context`, and `fetch_source`.
6. Close all eight open questions as decisions in the shaping packet.
7. Regenerate the HTML planning artifact from the scrubbed markdown.

## Notes
- The repo is public, so public docs should not teach a stranger the operator's private topology.
- Completed by the frozen-experimental boundary scrub branch; no runtime code was added.
