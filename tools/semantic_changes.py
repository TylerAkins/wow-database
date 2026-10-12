#!/usr/bin/env python3
"""Compare generated database products while ignoring revision-only churn."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path
from typing import Any


def normalize_records(data: bytes) -> list[dict[str, Any]]:
    rows = []
    for line_number, raw in enumerate(data.splitlines(), 1):
        if not raw.strip():
            raise ValueError(f"records.jsonl line {line_number} is blank")
        row = json.loads(raw)
        if not isinstance(row, dict):
            raise ValueError(f"records.jsonl line {line_number} is not an object")
        row.pop("sourceRevision", None)
        provenance = row.get("provenance")
        if isinstance(provenance, dict):
            for observation in provenance.values():
                if isinstance(observation, dict):
                    observation.pop("revision", None)
                    sources = observation.get("sources")
                    if isinstance(sources, list):
                        for source in sources:
                            if isinstance(source, dict):
                                source.pop("revision", None)
        rows.append(row)
    rows.sort(key=lambda row: (row.get("flavor", ""), row.get("entityType", ""), row.get("entityId", "")))
    return rows


def semantic_records_equal(left: bytes, right: bytes) -> bool:
    return normalize_records(left) == normalize_records(right)


def is_canonical_path(path: str) -> bool:
    return any(
        path.startswith(f"data/{flavor}/{kind}/")
        for flavor in ("era", "forever")
        for kind in ("sources", "compilation")
    )


def normalized_product(path: str, data: bytes) -> Any:
    if path.endswith("/records.jsonl"):
        return normalize_records(data)
    value = json.loads(data)
    if path.endswith("/manifest.json"):
        if not isinstance(value, dict):
            return value
        for key in ("sourceRevision", "recordsSha256", "generatedAt"):
            value.pop(key, None)
        if path == "export/forever/manifest.json":
            value.pop("commit", None)
    if path.endswith("/conflicts.json") and isinstance(value, list):
        for conflict in value:
            if not isinstance(conflict, dict):
                continue
            for candidate in conflict.get("candidates", []):
                if isinstance(candidate, dict):
                    candidate.pop("revision", None)
    return value


def tracked_product_files(base: str) -> set[str]:
    paths = subprocess.run(
        ["git", "ls-tree", "-r", "--name-only", base, "--", "data", "export/forever"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.splitlines()
    return {
        path for path in paths
        if (path.endswith("/records.jsonl") and is_canonical_path(path))
        or (path.endswith(".json") and (is_canonical_path(path) or path.startswith("export/forever/")))
    }


def changed_products(base: str) -> list[str]:
    current = {
        path.as_posix() for path in Path("data").rglob("*")
        if path.is_file() and (
            (path.name == "records.jsonl" and is_canonical_path(path.as_posix()))
            or (path.suffix == ".json" and is_canonical_path(path.as_posix()))
        )
    }
    current.update(path.as_posix() for path in Path("export/forever").rglob("*.json") if path.is_file())
    previous = tracked_product_files(base)
    changed = []
    for path_text in sorted(current | previous):
        path = Path(path_text)
        exists_now = path.is_file()
        exists_before = path_text in previous
        if not exists_now or not exists_before:
            changed.append(path_text)
            continue
        old = subprocess.run(["git", "show", f"{base}:{path_text}"], check=True, capture_output=True).stdout
        if normalized_product(path_text, old) != normalized_product(path_text, path.read_bytes()):
            changed.append(path_text)
    return changed


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="origin/main", help="Git ref containing the comparison baseline")
    args = parser.parse_args()
    try:
        changed = changed_products(args.base)
    except (OSError, ValueError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        print(f"semantic-changes: {exc}", file=sys.stderr)
        return 1
    if changed:
        print("changed=true")
        print(f"changed generated products: {', '.join(changed)}", file=sys.stderr)
    else:
        print("changed=false")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
