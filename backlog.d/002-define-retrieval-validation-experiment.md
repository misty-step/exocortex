# Define the retrieval validation experiment before building

Priority: P0 · Status: ready · Estimate: M

## Goal
Define the retrieval-quality experiment that must produce evidence before
Exocortex leaves frozen-experimental status.

## Oracle
- [ ] `docs/experiments/retrieval-validation.md` exists and defines a 15-20 question evaluation over real private data without committing that data.
- [ ] The experiment compares the incumbent private retrieval path, `exo search`, and `exo packet` as separate arms.
- [ ] Crucible grading is specified with deterministic citation-fetchability checks plus an LLM relevance/completeness jury.
- [ ] The acceptance rule reports paired results with a Wilson confidence interval and names the threshold required before kernel implementation resumes.
- [ ] The doc states privacy handling: question text, source snippets, and receipts stay private unless explicitly redacted.

## Verification System
- Claim: the validation experiment is defined tightly enough that another agent can run it later without chat context.
- Falsifier: the doc omits arms, graders, sample size, privacy handling, or an implementation-unfreeze threshold.
- Driver: file inspection, targeted `rg` checks for the required sections, and no-code diff review.
- Grader: the experiment packet answers what data, what arms, what grader, what statistics, what evidence, and what stop rule.
- Evidence packet: the experiment doc plus PR transcript.
- Cadence: run before any runtime implementation PR is opened.

## Children
1. Specify the private-question selection rule and minimum sample size.
2. Specify the three retrieval arms and how their outputs are normalized.
3. Specify deterministic citation-fetchability checks.
4. Specify Crucible LLM jury prompts, blinding, and adjudication shape.
5. Specify paired scoring, Wilson interval reporting, and the implementation-unfreeze threshold.
6. Specify privacy redaction and where private receipts live outside this public repo.

## Notes
- This is an experiment-definition item only. It does not authorize building `exo`.
- The public repo may describe the baseline generically; private command names and collection paths belong in the consumer workspace.
