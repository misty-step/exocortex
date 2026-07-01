# Add local semantic retrieval after the contract proves value

Priority: P2 · Status: blocked · Estimate: M

## Goal
Add vector retrieval and local embeddings only after the contract-first kernel
has a fetchable-citation path and the eval gives a baseline to beat.

## Oracle
- [ ] LanceDB stores full-text and vector data in one local index directory.
- [ ] A bundled CPU-friendly embedding path works without network or GPU.
- [ ] Hybrid ranking reports freshness and source-decision data in the packet.
- [ ] The retrieval eval is rerun against the full-text baseline and reports a paired delta with confidence interval.

## Verification System
- Claim: semantic retrieval improves recall without weakening the evidence contract.
- Falsifier: the vector path cannot run offline, citations become unfetchable, or the eval delta is inside the noise floor.
- Driver: fixture corpus, offline embedding smoke test, packet inspection, and paired eval replay.
- Grader: eval result and packet evidence sections.
- Evidence packet: benchmark output plus representative packet artifacts.
- Cadence: before enabling semantic retrieval by default.

## Children
1. Add local embedding model selection and cache policy.
2. Add vector table/indexing in the same LanceDB store.
3. Add hybrid ranking with transparent score and freshness reporting.
4. Add an offline smoke fixture.
5. Rerun the retrieval validation experiment and compare against the full-text baseline.

## Notes
- This is deliberately after the contract-first slice; rebuilding commodity retrieval before receipts would miss the product premise.
