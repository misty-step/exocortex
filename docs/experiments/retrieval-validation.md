# Retrieval Validation Experiment

## Goal

Define the private-data retrieval experiment that decides whether Exocortex can
leave frozen-experimental status. The experiment measures whether Exocortex's
evidence contract improves agent-ready context over the incumbent private
retrieval path without leaking private corpus details into this public repo.

## Scope

This document defines the experiment. It does not build `exo`, select the
private questions, copy private data, or run the eval.

## Corpus And Questions

- Use a real private corpus owned by the operator or consumer workspace.
- Select 15-20 recent, real questions from traces or work sessions where context
  quality changes the answer.
- Keep raw question text private unless explicitly redacted for publication.
- Each question gets an internal ID, short redacted theme, and expected citation
  requirements.
- Freeze the question set before running any candidate arm.

## Retrieval Arms

Run every question through three paired arms:

1. **Incumbent private retrieval path:** the current private command or habit
   used to gather context.
2. **`exo search`:** iterative search over the same allowed private sources
   through the future generic adapter/profile boundary.
3. **`exo packet`:** evidence-session packet compilation over the same allowed
   private sources.

Normalize each arm into a common result record:

```text
question_id
arm
retrieved_items[]
citations[]
exclusions[]
gaps[]
receipt_ref
elapsed_ms
operator_notes
```

## Grading

Use Crucible to grade two layers:

1. **Deterministic citation checks**
   - Every citation has a fetchable source reference.
   - Every source reference has a checksum or equivalent integrity marker.
   - The cited source belongs to the allowed profile for the question.
   - Exclusions are present when a disallowed source class exists.

2. **Relevance and completeness jury**
   - Blind the jury to arm labels.
   - Score each answer for relevance, completeness, harmful omission, and
     unnecessary private exposure.
   - Require a short rationale tied to cited evidence.
   - Adjudicate ties or disagreements with a human review pass.

## Statistics

- Score paired by question, not as three independent batches.
- Report win/loss/tie counts for `exo search` and `exo packet` against the
  incumbent baseline.
- Report a Wilson confidence interval for the paired win rate.
- Treat any eval delta inside the interval's noise floor as inconclusive.

## Implementation-Unfreeze Threshold

Implementation may resume only if the experiment report shows all of the
following:

- Deterministic citation-fetchability pass rate for the Exocortex arms is at
  least 95%. The incumbent baseline is reported separately and cannot block
  unfreeze solely for lacking Exocortex citation structure.
- No critical private-profile violation occurs in either Exocortex arm.
- `exo packet` beats the incumbent baseline on relevance/completeness for a
  majority of paired questions, with the Wilson lower bound above 0.50.

If the result is inconclusive or failed, implementation remains frozen until a
revised experiment or backlog change is recorded.

## Evidence Packet

The private experiment run should produce:

- frozen question manifest with private text or redacted summaries
- per-arm result records
- Crucible deterministic check output
- blinded jury scores and rationales
- paired statistical report
- privacy incident log, even if empty
- operator decision: unfreeze, revise, or stop

Public reporting may include only redacted summaries, aggregate scores, and
non-sensitive artifacts.

## Privacy Handling

- Do not commit private question text, source snippets, receipts, command names,
  roots, or corpus metadata to this repo.
- Store private receipts in the consumer workspace.
- Redact source IDs before publishing examples.
- If a public example is useful, create a synthetic fixture that preserves the
  failure mode without preserving private content.

## Stop Conditions

- A private-profile violation appears.
- The incumbent baseline cannot be run or normalized.
- The question set is changed after seeing candidate outputs.
- Crucible cannot produce both deterministic checks and a blinded jury report.
- The result is inconclusive and no revised experiment is specified.
