# Exocortex DESIGN.md

This file is the product's public-site brand contract. Keep it short and exact:
agents and humans should be able to update `site/` from this file without
inventing a second design system.

## Brand Voice

- Plain-spoken, concrete, and operator-facing.
- Lead with the user outcome, then the proof.
- Avoid marketing fog, mascot language, and decorative claims.
- Exocortex is experimentation-open and productization-gated: say that plainly.
  There is no working `exo` binary yet. The pitch is the evidence contract and
  the proof-of-shape fixture, not a shipped product surface.

## Pitch One-Liner

`Exocortex is the local-first evidence core that lets an agent ask for context and a human audit exactly what it saw, skipped, and can fetch again.`

## Lucide Mark

- Icon: `search-check`
- Reason: chosen for Exocortex because the differentiator is verified
  retrieval — citations you can re-fetch and check, not just a search result.
- Rule: the mark is an inline Lucide SVG inside `.ae-app-mark`. No bespoke
  marks, logo images, emoji marks, or colored wordmarks.

## Palette Hooks

Pin `data-ae-theme="ultramarine"` — a plain, technical register (blue accent)
that reads as infrastructure, not a product with a "brand."

```css
:root {
  --ae-accent: #2643d0;
  --ae-accent-dark: #8c9eff;
}
```

No extra categorical hues needed; the site has no status pills or charts.

## Screenshot Inventory

| File | Surface | State | Caption |
| --- | --- | --- | --- |
| `site/assets/screenshots/01-verify-gate.png` | `./scripts/verify.sh` | Real terminal output, current `master` | The repo-owned gate: public-boundary grep plus fixture regeneration and idempotence check. |
| `site/assets/screenshots/02-evidence-packet.png` | `tests/fixtures/first-packet/evidence-packet.md` | Real generated fixture, current `master` | The proof-of-shape artifact: a cited answer with per-claim evidence and checksummed anchors. |
| `site/assets/screenshots/03-source-registry.png` | `tests/fixtures/first-packet/source-registry.json` | Real generated fixture, current `master` | The source-decision receipt behind the packet — what was declared, why it was included, and what freshness label it carries. |

There is no product UI to screenshot. All three captures are real command
output and real generated fixtures, not mockups.

## Footer Links

- Misty Step: `https://mistystep.io`
- GitHub: `https://github.com/misty-step/exocortex` (public)
- No Weave link — Exocortex is a standalone evidence core, not a
  Weave-family execution-plane product.

## Release Notes Rule

`site/changelog.html` is user-facing. Exocortex has no Landmark-exported
release notes yet (no tagged product release to summarize). The changelog
page says so honestly instead of inventing entries, and points at the real
commit history.
