# ADR-0002: Memory behaviors — journal, reflection, lifecycle, coordination

Status: Accepted (2026-08-22)
Deciders: operator (phaedrus) with ox-alpha research session

## Context

v0 ships the substrate (cortices, CAS put pipeline, provenance, qmd
search). Product question: how does a pile of notes become a memory and
context system? Research inputs: Generative Agents (Park et al.,
arXiv:2304.03442 — recency × importance × relevance retrieval, reflection),
OptMem (Taelin — append-only log, rebuildable summary tree, lazy
agent-in-the-loop merges, mandatory session-start read, subagent write
ban), the OpenAI/HuggingFace agent-swarm incident (Black Hat 2026; agents
built a persistent cross-session message board that drove emergent
collective behavior and scope creep), A-MEM (NeurIPS 2025 — link
generation and evolution on ingest), Letta sleep-time compute (offline
consolidation), Chroma Foundation and LangChain Wiki Memory (hosted wiki
convergence, no git-grade auditability).

## Decisions

1. **Journal stream — immutable files, not an append log.**
   `note "<thought>"` writes `journal/YYYY-MM-DD/<ulid>-<agent>.md`
   through the standard create-only put pipeline. Unique paths make
   cross-host push races benign: the loser converges and retries with a
   fresh ULID (bounded), and every terminal conflict preserves the
   payload in its body. A single shared append file was REJECTED: it is
   a whole-file CAS hotspot that turns routine fleet activity into
   revision_conflict.
2. **Memo notes are quiet.** `type: memo` suppresses daybook-profile
   key warnings — one-line memories are not wiki notes, and constant
   noise erodes trust in the journal.
3. **Reflection is a scheduled agent loop, not a daemon.** Using the
   operator's existing systemd-timer substrate, a periodic session
   reads the journal diff since its last marker and writes merge, link
   (`[[wiki-links]]`), and supersede proposals through `put` — CAS and
   conflicts-as-data make a bot-safe multi-writer vault possible, which
   none of the compared systems offer. Proposals are human-diffable by
   construction. Background daemons remain rejected (DE-01; OptMem
   reached the same conclusion independently).
4. **Lifecycle over decay.** No forgetting curves or deletion. Notes
   move `active → superseded → archived` with `superseded_by` links;
   git keeps the audit trail. Importance starts as an optional
   frontmatter key plus derived backlink counts; no scoring engine
   until corpus scale demands it (DE-01).
5. **Coordination is a cortex convention.** Typed notes (finding,
   blocker, claim) under `coordination/`, with CAS preventing clobber
   and provenance providing attribution. The HF incident's lessons are
   structural here: shared surfaces need audit (git/provenance), scope
   enforcement (path jail, CAS), and access discipline (private remote).
   Claim/lease semantics remain parked until contention appears.
6. **Session traces enter as feeds, never as raw cortices.** Cortices
   are writable markdown+git corpora (AGENTS.md); JSONL session logs are
   feed sources compiled into provenance-stamped notes with `source:`
   lineage. Registering raw log directories as cortices was REJECTED as
   a boundary bypass.
7. **Fleet doctrine ships in omp-config.** The exocortex skill is
   vendored into omp-config `skills/`, and `global/AGENTS.md` mandates
   aggressive grounding (web research AND exocortex retrieval before
   exploring or implementing) and frequent write-back (status updates;
   "what bit me and the fix") under a usefulness bar: a future agent
   would thank you.

## Consequences

- `note`, the `memo` quiet rule, journal retry, and registry
  serialization are kernel code (shipped with proofs 12–14).
- The reflection loop and session-feed compiler are follow-on slices;
  their contracts are fixed here so conventions can start immediately.
- Subagents must not write memory directly (OptMem rule, adopted in
  SKILL.md) — they report to parents who judge novelty.

## Open questions

- Reflection cadence and which harness runs it.
- Importance scoring source if backlink derivation proves insufficient.
- Access control timing if the fleet grows beyond single-operator trust.
