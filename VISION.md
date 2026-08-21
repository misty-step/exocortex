# Vision

Memory is the biggest remaining gap in agent work. Agents forget how to do
things, lose context between sessions, and do not share what they learn. The
proven fix is unglamorous: agents write things down, then find them later —
in a versioned, structured wiki.

We already own that substrate. Daybook is a git-versioned markdown wiki. QMD
indexes it and every raw agent session. What the fleet lacks is the product
surface: one deep module that makes writing correct, finding reliable, and
lineage automatic — on every harness, without trusting prose conventions.

Exocortex is that surface. It is private-first and boring by design:

- Cortices stay plain markdown in git. Humans keep full control, diffing, and
  portability. The kernel is an interface, not a store.
- Feeds bring the outside in (Notion, Drive, session logs) without letting
  the inside leak out.
- One binary, two faces: CLI for everything that can exec a command, MCP for
  harnesses that register it.

Not a vector database, not a hosted service, not a chat app. The earlier
"evidence core" concept this repo once hosted (auditable context packets)
lives on in the `evidence-packet` skill, not here.
