# Open the public front door

Priority: P1 · Status: pending · Estimate: M

## Goal
Make the public repository understandable, legally usable, and agent-operable
without relying on private lane context.

## Oracle
- [ ] `README.md` explains the stranger pitch: cited, policy-filtered, replayable context packets with exclusions.
- [ ] `AGENTS.md` points cold agents to `VISION.md`, `backlog.d/`, and the repo gate without duplicating strategy prose.
- [ ] GitHub repo description states the product category in one sentence.
- [ ] The named repo gate exists and covers public-boundary grep, docs hygiene, and any available fixture once code lands.
- [ ] License status is clear and remains MIT.

## Verification System
- Claim: a cold agent or stranger can understand what Exocortex is, what it refuses, and how to validate changes from the repo page alone.
- Falsifier: the README depends on private context, the repo has no named gate, or `AGENTS.md` conflicts with `VISION.md`.
- Driver: cold-read review, `gh repo view`, gate command, and boundary grep.
- Grader: reviewer can state product, non-goals, gate, and next backlog item without chat history.
- Evidence packet: README/AGENTS diff plus gate output.
- Cadence: after the frozen boundary scrub, before the first implementation PR.

## Children
1. Add README with a first-screen demo packet and exclusions explanation.
2. Add repo `AGENTS.md` as a small pointer contract.
3. Set the GitHub repo description.
4. Add the first named repo gate and wire it into CI.
5. Keep release intelligence workflow present and documented.

## Notes
- The MIT license is added by the 2026-07-01 groom phase and should remain the public default.
