# Evidence Packet: What does the fleet know about serving generated HTML sites without breaking relative links?

- Packet ID: `first-packet-html-relative-links`
- Observed at: `2026-07-04T19:32:56Z`
- Status: prototype fixture, not product runtime
- Promotion boundary: answers the contract-shape question; does not relax the retrieval validation bar

## Grep-First Source Metadata

- Learning title: Redirect slashless directory artifact URLs before serving relative-link pages
- Learning date: `2026-07-04`
- Learning tags: `[artifact-shelf, static-serving, slashless-url, relative-links, redirect-308, dot-paths]`
- Learning provenance: redacted downstream repository handle; use the learning source line anchors below

## Declared Sources

| Source | Kind | Freshness | Fetch Contract | Decision |
|---|---|---|---|---|
| learnings-corpus | file-tree | same-day learning; source frontmatter date is 2026-07-04 | path plus line range | The frontmatter and retrieval terms directly match static serving, generated HTML, slashless directory URLs, relative links, redirects, and dot-prefixed paths. |
| operator-trace-corpus | configured-command-collection | same-day and previous-day traces fetched on 2026-07-04 | docid plus collection path plus line range | The trace corpus confirmed the live incident outcome and added prior prefix and fallback failures that the learning document alone did not fully cover. |

## Answer

- Canonicalize generated-site directory URLs to trailing-slash URLs before serving index content; use a permanent redirect for slashless directory requests.
- Preserve the external/public mount prefix when rendering links. Internal route stripping must not leak into hrefs.
- Allow normal dot-prefixed components such as .github while still rejecting traversal components.
- For legacy artifact trees without a root index, serve a deterministic fallback: single HTML redirects, multi-file trees get an auto-index, and empty directories stay 404.
- Verify by exercising slashless redirect, slash URL content, relative child paths, dot-prefixed paths, rendered href content, and route-misroute failures. Status-only sweeps are insufficient.

## Evidence

### E1 — learnings-corpus (same-day)

The learning is tagged and dated as a same-day static-serving bug track for slashless URLs, relative links, redirect handling, dot paths, generated HTML, and published sites.

Citations: `../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md:2-13` checksum `sha256:730edaa9444b`; `../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md:48-51` checksum `sha256:3232ef6999b4`.

### E2 — learnings-corpus (same-day)

Browser relative-link resolution treats a slashless directory URL as a file path, so serving index content there moves child links one level too high. The remedy is to redirect directory URLs to the trailing-slash form before serving index content, keep traversal checks strict, allow normal dot-prefixed components, and verify both slash forms plus child or dot paths.

Citations: `../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md:18-23` checksum `sha256:3c143ae2bc2c`; `../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md:27-35` checksum `sha256:b837ac6671b1`.

### E3 — learnings-corpus (same-day)

The linked fix and regressions show the concrete expected proof shape: permanent slashless-directory redirect, explicit redirect regression, and dot-prefixed path round trip.

Citations: `../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md:39-46` checksum `sha256:f3d5ccdbb1f3`.

### E4 — operator-trace-corpus (same-day)

The same-day trace says the fleet saw directory artifacts served at slashless URLs, every relative link broke, and the fix was a 308 redirect plus dot-path support, followed by a full republish with all navigation returning 200s.

Citations: docid `e1f295` `meta/conversations/2026-07-04.md:88-99`.

### E5 — operator-trace-corpus (previous-day)

The previous-day trace adds a second failure mode: internal route stripping leaked into rendered hrefs, dropping the public prefix and making links fall through to the root mount. The fix was to feed the public prefix into rendered URLs, make root fallback fail loudly, and resweep links by content rather than status code alone.

Citations: docid `c9bcb1` `meta/conversations/2026-07-03.md:1-5`.

### E6 — operator-trace-corpus (previous-day)

Earlier traces show legacy artifact trees without root index files were not lost; serving needed deterministic directory fallback: redirect a single HTML file, otherwise produce an auto-index, while empty directories remain 404s.

Citations: docid `9f3128` `meta/bridge-reimagining-2026-07-03.md:95-98`; docid `c9bcb1` `meta/conversations/2026-07-03.md:432-439`.

## Exclusions

- `operator-trace-corpus`: A broad initial query returned unrelated books and clippings. They were excluded because they did not explain generated-site serving or relative-link failures.
- `operator-trace-corpus`: Raw private snippets, source bodies, personal context strings, and collection names are not committed in this public fixture; only docid/path/line handles and paraphrased claims are included.
- `learnings-corpus`: The packet does not fetch or embed the downstream application source files referenced by the learning document; it cites the learning record as the bounded experiment source.

## Gaps

- Public file-tree anchors now carry citation/v1-style checksums; private docid trace handles remain fetch handles without content checksums because the private collection is declared outside this public repo.
- The configured-command source is represented as harvested evidence records, not as a reusable adapter.
- The packet does not replay browser navigation; it cites traces and the learning record that describe those verifications.
- The private trace index reported incomplete embeddings, so lexical and targeted fetches were required for confidence.

## Fixture Integrity

The builder validates local file-line anchors for the learning source. Private trace anchors are intentionally represented as docid/path/line handles because the private collection is declared outside this public repo.
