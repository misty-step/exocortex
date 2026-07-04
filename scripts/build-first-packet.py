#!/usr/bin/env python3
"""Build the first evidence-contract packet fixture.

This is intentionally a prototype script, not product runtime. It assembles a
single markdown packet from a source registry and validates file-line anchors
that are available from the local checkout.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_REGISTRY = REPO_ROOT / "tests/fixtures/first-packet/source-registry.json"
DEFAULT_OUTPUT = REPO_ROOT / "tests/fixtures/first-packet/evidence-packet.md"


def load_json(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def resolve_repo_path(path_text: str) -> Path:
    candidate = (REPO_ROOT / path_text).resolve()
    return candidate


def parse_line_range(line_range: str) -> tuple[int, int]:
    start_text, end_text = line_range.split("-", 1)
    return int(start_text), int(end_text)


def line_range_bytes(path: Path, start: int, end: int) -> tuple[bytes, int]:
    lines = path.read_bytes().splitlines(keepends=True)
    line_count = len(lines)
    if start < 1 or end < start or end > line_count:
        return b"", line_count
    return b"".join(lines[start - 1 : end]), line_count


def checksum_bytes(content: bytes) -> str:
    return "sha256:" + hashlib.sha256(content).hexdigest()


def checksum_fingerprint(checksum: str) -> str:
    if ":" not in checksum:
        return checksum[:12]
    algorithm, digest = checksum.split(":", 1)
    return f"{algorithm}:{digest[:12]}"


def frontmatter_fields(path: Path) -> dict[str, str]:
    fields: dict[str, str] = {}
    if not path.exists():
        return fields

    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0].strip() != "---":
        return fields

    for line in lines[1:]:
        if line.strip() == "---":
            break
        if ":" in line and not line.startswith(" "):
            key, value = line.split(":", 1)
            fields[key.strip()] = value.strip().strip('"')
    return fields


def check_file_anchor(anchor: dict[str, str]) -> str:
    path_text = anchor.get("path", "")
    if not path_text.startswith("../"):
        return ""

    path = resolve_repo_path(path_text)
    if not path.exists():
        return f"missing local file anchor: {path_text}"

    start, end = parse_line_range(anchor["lines"])
    content, line_count = line_range_bytes(path, start, end)
    if start < 1 or end < start or end > line_count:
        return f"invalid line range for {path_text}: {anchor['lines']}"

    expected_checksum = anchor.get("checksum", "")
    if not expected_checksum:
        return f"missing checksum for local file anchor: {path_text}:{anchor['lines']}"

    actual_checksum = checksum_bytes(content)
    if actual_checksum != expected_checksum:
        return (
            f"checksum mismatch for {path_text}:{anchor['lines']}: "
            f"expected {expected_checksum}, actual {actual_checksum}"
        )
    return ""


def anchor_label(anchor: dict[str, str]) -> str:
    checksum = anchor.get("checksum", "")
    checksum_text = ""
    if checksum:
        checksum_text = f" checksum `{checksum_fingerprint(checksum)}`"

    if "docid" in anchor:
        return (
            f"docid `{anchor['docid']}` "
            f"`{anchor['path']}:{anchor['lines']}`"
            f"{checksum_text}"
        )
    return f"`{anchor['path']}:{anchor['lines']}`{checksum_text}"


def bullet_list(items: list[str]) -> list[str]:
    return [f"- {item}" for item in items]


def source_lines(registry: dict[str, Any]) -> list[str]:
    lines = [
        "## Declared Sources",
        "",
        "| Source | Kind | Freshness | Fetch Contract | Decision |",
        "|---|---|---|---|---|",
    ]
    for source in registry["sources"]:
        lines.append(
            "| {id} | {kind} | {freshness} | {fetch_contract} | {decision_reason} |".format(
                **source
            )
        )
    return lines


def evidence_lines(registry: dict[str, Any]) -> list[str]:
    lines = ["## Evidence", ""]
    for item in registry["evidence"]:
        citations = "; ".join(anchor_label(anchor) for anchor in item["anchors"])
        lines.extend(
            [
                f"### {item['id']} — {item['source_id']} ({item['freshness']})",
                "",
                item["claim"],
                "",
                f"Citations: {citations}.",
                "",
            ]
        )
    return lines


def build_packet(registry: dict[str, Any]) -> str:
    learning_path = resolve_repo_path(
        "../harness-kit/docs/solutions/web-serving/artifact-shelf-slashless-directory-urls.md"
    )
    learning_frontmatter = frontmatter_fields(learning_path)

    lines: list[str] = [
        f"# Evidence Packet: {registry['question']}",
        "",
        f"- Packet ID: `{registry['packet_id']}`",
        f"- Observed at: `{registry['observed_at']}`",
        "- Status: prototype fixture, not product runtime",
        "- Promotion boundary: answers the contract-shape question; does not relax the retrieval validation bar",
        "",
    ]

    if learning_frontmatter:
        lines.extend(
            [
                "## Grep-First Source Metadata",
                "",
                f"- Learning title: {learning_frontmatter.get('title', 'unknown')}",
                f"- Learning date: `{learning_frontmatter.get('date', 'unknown')}`",
                f"- Learning tags: `{learning_frontmatter.get('tags', 'unknown')}`",
                "- Learning provenance: redacted downstream repository handle; use the learning source line anchors below",
                "",
            ]
        )

    lines.extend(source_lines(registry))
    lines.extend(["", "## Answer", ""])
    lines.extend(bullet_list(registry["answer_points"]))
    lines.extend([""])
    lines.extend(evidence_lines(registry))

    lines.extend(["## Exclusions", ""])
    for exclusion in registry["exclusions"]:
        lines.append(f"- `{exclusion['source_id']}`: {exclusion['reason']}")

    lines.extend(["", "## Gaps", ""])
    lines.extend(bullet_list(registry["gaps"]))
    lines.extend(["", "## Fixture Integrity", ""])
    lines.append(
        "The builder validates local file-line anchors for the learning source. "
        "Private trace anchors are intentionally represented as docid/path/line handles "
        "because the private collection is declared outside this public repo."
    )
    lines.append("")
    return "\n".join(lines)


def validate(registry: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    for item in registry["evidence"]:
        for anchor in item["anchors"]:
            error = check_file_anchor(anchor)
            if error:
                errors.append(error)
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--registry", type=Path, default=DEFAULT_REGISTRY)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    registry = load_json(args.registry)
    errors = validate(registry)
    if errors:
        for error in errors:
            print(error)
        return 1

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(build_packet(registry), encoding="utf-8")
    print(args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
