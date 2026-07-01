# Integrate first consumers without leaking them

Priority: P2 · Status: blocked · Estimate: L

## Goal
Prove Exocortex in private consumer workflows while keeping all private topology,
commands, policies, and evidence archives out of the public repo.

## Oracle
- [ ] A private consumer can configure a command adapter without committing command names, roots, collections, or secrets here.
- [ ] A one-week side-by-side read path compares incumbent context gathering against `exo packet` without writeback.
- [ ] A packet-in-PR convention exists for factory repos and at least one reviewer consumes the packet evidence.
- [ ] `exo explain --html` is sufficient for the first inspection workflow, or a specific UI gap is documented.

## Verification System
- Claim: the first consumer proves real utility without turning Exocortex into a public copy of private workflow policy.
- Falsifier: private roots appear in this repo, writeback lands before read-path proof, or packets are produced but no downstream reviewer uses them.
- Driver: private-consumer config review, side-by-side run receipts, PR packet artifact, and reviewer receipt.
- Grader: operator review of private receipts plus public repo grep for private topology.
- Evidence packet: private receipts and public PR artifact links where redacted.
- Cadence: after the kernel and semantic slices are proven.

## Children
1. Add private consumer config outside this repo.
2. Run a read-only side-by-side context-gathering week.
3. Define the packet-in-PR artifact convention.
4. Add a reviewer rule that reads packet evidence before accepting claims.
5. Extend `exo explain --html` only where the first inspection gap proves it.

## Notes
- Private consumers are proof surfaces, not public product boundaries.
