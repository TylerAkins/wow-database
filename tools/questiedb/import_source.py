#!/usr/bin/env python3
"""Safely import QuestieDB's generated entity literals and static corrections."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from canonical import atomic_write_dataset  # noqa: E402
from lua_literal import LuaLiteralError, parse

ENTITY_TYPES = {
    "quest": ("Quest", "quest"),
    "npc": ("Npc", "npc"),
    "item": ("Item", "item"),
    "object": ("Object", "object"),
}
FLAVORS = {"era": "Classic", "forever": "Forever"}
MAX_SOURCE_BYTES = 64 * 1024 * 1024
SCHEMA_VERSION = 1


class ImportErrorMessage(ValueError):
    pass


def read_source(path: Path) -> str:
    size = path.stat().st_size
    if size > MAX_SOURCE_BYTES:
        raise ImportErrorMessage(f"{path}: source exceeds {MAX_SOURCE_BYTES}-byte limit")
    return path.read_text(encoding="utf-8")


def matching_brace(text: str, open_at: int) -> int:
    depth = 0
    i = open_at
    while i < len(text):
        if text.startswith("--[[", i):
            end = text.find("]]", i + 4)
            if end < 0:
                raise ImportErrorMessage(f"unterminated Lua block comment at byte {i}")
            i = end + 2
        elif text.startswith("--", i):
            end = text.find("\n", i + 2)
            i = len(text) if end < 0 else end + 1
        elif text.startswith("[[", i):
            end = text.find("]]", i + 2)
            if end < 0:
                raise ImportErrorMessage(f"unterminated Lua long string at byte {i}")
            i = end + 2
        elif text[i] in "\"'":
            quote = text[i]
            i += 1
            while i < len(text):
                if text[i] == "\\":
                    i += 2
                elif text[i] == quote:
                    i += 1
                    break
                else:
                    i += 1
            else:
                raise ImportErrorMessage("unterminated Lua string")
        else:
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
                if depth == 0:
                    return i + 1
            i += 1
    raise ImportErrorMessage("unterminated Lua table")


def extract_table(text: str, start: int) -> Any:
    open_at = text.find("{", start)
    if open_at < 0:
        raise ImportErrorMessage("expected a returned Lua table")
    end = matching_brace(text, open_at)
    value = parse(text[open_at:end])
    if not isinstance(value, dict):
        raise ImportErrorMessage("expected keyed Lua table")
    return value


def parse_entity_file(path: Path, entity_type: str, repo: Path) -> list[dict[str, Any]]:
    text = read_source(path)
    key_pattern = re.compile(rf"QuestieDB\.{entity_type}Keys\s*=\s*\{{")
    key_match = key_pattern.search(text)
    if not key_match:
        raise ImportErrorMessage(f"{path}: {entity_type}Keys declaration missing")
    keys_table = extract_table(text, key_match.start())
    keys: dict[str, int] = {}
    for name, index in keys_table.items():
        if not isinstance(name, str) or not isinstance(index, int) or index < 1:
            raise ImportErrorMessage(f"{path}: unsupported {entity_type}Keys entry {name!r}: {index!r}")
        if index in keys.values():
            raise ImportErrorMessage(f"{path}: duplicate key index {index} in {entity_type}Keys")
        keys[name] = index
    if not keys:
        raise ImportErrorMessage(f"{path}: empty {entity_type}Keys declaration")

    data_pattern = re.compile(rf"QuestieDB\.{entity_type}Data\s*=\s*\[\[")
    data_match = data_pattern.search(text)
    if not data_match:
        raise ImportErrorMessage(f"{path}: {entity_type}Data payload missing")
    close_at = text.rfind("]]" )
    if close_at < data_match.end():
        raise ImportErrorMessage(f"{path}: unterminated {entity_type}Data payload")
    payload = parse(text[data_match.end() : close_at])
    if not isinstance(payload, dict) or not payload:
        raise ImportErrorMessage(f"{path}: {entity_type}Data is not a nonempty keyed table")

    rows: list[dict[str, Any]] = []
    for raw_id, values in payload.items():
        if not isinstance(raw_id, int) or raw_id <= 0 or not isinstance(values, list):
            raise ImportErrorMessage(f"{path}: invalid {entity_type} row {raw_id!r}")
        attributes: dict[str, Any] = {}
        states: dict[str, str] = {"effectiveValues": "unavailable"}
        for name, index in keys.items():
            if index > len(values) or values[index - 1] is None:
                states[name] = "missing"
                continue
            value = values[index - 1]
            attributes[name] = value
            if _is_empty(value):
                states[name] = "empty"
        row = {
            "schemaVersion": SCHEMA_VERSION,
            "flavor": "forever" if path.parts[-2] == "Forever" else "era",
            "entityType": ENTITY_TYPES[entity_type][1],
            "entityId": str(raw_id),
            "source": "questiedb",
            "sourceRevision": "pending",
            "attributes": attributes,
            "fieldStates": states,
            "provenance": {
                field: {"source": "questiedb", "revision": "pending", "path": path.relative_to(repo).as_posix()}
                for field in sorted(set(attributes) | set(states))
            },
        }
        rows.append(row)
    rows.sort(key=lambda row: (row["entityType"], row["entityId"]))
    return rows


def _is_empty(value: Any) -> bool:
    return value is None or value == "" or value == [] or value == {}


def parse_static_corrections(repo: Path, flavor: str, revision: str) -> tuple[list[dict[str, Any]], list[str]]:
    # These are the static layers used by QuestieDB's Classic and Forever
    # flavors. Do not infer applicability from every directory in the checkout:
    # the repository also contains correction data for other game expansions.
    folders = ["Era", "Shared"] if flavor == "era" else ["Era", "Forever", "Shared"]
    rows: list[dict[str, Any]] = []
    found_layers: set[str] = set()
    static_methods = re.compile(r"function\s+[A-Za-z_][A-Za-z0-9_]*:(Load|LoadAutomatics|LoadAutomaticQuestStarts)\s*\([^)]*\)")
    for folder in folders:
        base = repo / "src" / "corrections" / folder
        if not base.exists():
            continue
        for path in sorted(base.rglob("*.lua")):
            kind_match = re.search(r"(Quest|NPC|Item|Object).*Fixes\.lua$", path.name, re.IGNORECASE)
            if not kind_match:
                continue
            body = read_source(path)
            entity_type = kind_match.group(1).lower()
            for method_match in static_methods.finditer(body):
                method_end = method_match.end()
                next_method = re.search(r"\bfunction\s+[A-Za-z_][A-Za-z0-9_]*:", body[method_end:])
                method_limit = method_end + next_method.start() if next_method else len(body)
                method_body = body[method_end:method_limit]
                return_match = re.search(r"\breturn\s*\{", method_body)
                if not return_match:
                    continue
                table = extract_table(method_body, return_match.start())
                found_layers.add(folder)
                for target_id, fields in table.items():
                    if not isinstance(target_id, int) or not isinstance(fields, dict):
                        raise ImportErrorMessage(f"{path}: invalid static correction for {target_id!r}")
                    for raw_field, value in fields.items():
                        if not isinstance(raw_field, str) or not raw_field.startswith("$ref:"):
                            raise ImportErrorMessage(f"{path}: correction field key is not a QuestieDB literal reference")
                        field_name = raw_field.removeprefix("$ref:questKeys.").removeprefix("$ref:npcKeys.").removeprefix("$ref:itemKeys.").removeprefix("$ref:objectKeys.")
                        correction_id = f"correction:{folder}:{entity_type}:{target_id}:{field_name}"
                        row = {
                            "schemaVersion": SCHEMA_VERSION,
                            "flavor": "forever" if flavor == "forever" else "era",
                            "entityType": "relationship",
                            "entityId": correction_id,
                            "source": "questiedb",
                            "sourceRevision": revision,
                            "attributes": {
                                "correction": {
                                    "layer": folder,
                                    "targetType": entity_type,
                                    "targetId": str(target_id),
                                    "field": field_name,
                                    "value": value,
                                    "application": "not-evaluated",
                                }
                            },
                            "fieldStates": {"effectiveValue": "unavailable"},
                            "provenance": {
                                "correction": {"source": "questiedb", "revision": revision, "path": path.relative_to(repo).as_posix()},
                                "effectiveValue": {"source": "questiedb", "revision": revision, "path": path.relative_to(repo).as_posix()},
                            },
                        }
                        rows.append(row)
    rows.sort(key=lambda row: (row["entityType"], row["entityId"]))
    limitations = []
    if found_layers:
        limitations.append("Static QuestieDB corrections are preserved as unevaluated literal/expression records; source entity attributes remain the raw generated data values.")
    limitations.append("Dynamic correction functions, derived passes, generated deltas, and support bundles are not evaluated or imported.")
    return rows, limitations


def resolve_revision(repo: Path, supplied: str | None) -> str:
    if supplied:
        revision = supplied
    else:
        result = subprocess.run(["git", "-C", str(repo), "rev-parse", "HEAD"], check=True, capture_output=True, text=True)
        revision = result.stdout.strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ImportErrorMessage("source revision must be a full lowercase Git commit SHA")
    return revision


def import_source(repo: Path, flavor: str, out: Path, revision: str | None) -> None:
    if flavor not in FLAVORS:
        raise ImportErrorMessage(f"unsupported flavor {flavor!r}")
    revision = resolve_revision(repo, revision)
    data_dir = repo / "data" / FLAVORS[flavor]
    rows: list[dict[str, Any]] = []
    for entity_type, (prefix, _) in ENTITY_TYPES.items():
        filename = f"{FLAVORS[flavor].lower()}{prefix}DB.lua"
        path = data_dir / filename
        if not path.is_file():
            raise ImportErrorMessage(f"required entity source is missing: {path}")
        rows.extend(parse_entity_file(path, entity_type, repo))
    corrections, limitations = parse_static_corrections(repo, flavor, revision)
    rows.extend(corrections)
    for row in rows:
        row["sourceRevision"] = revision
        for field, provenance in row["provenance"].items():
            provenance["revision"] = revision
    commit_time = subprocess.run(["git", "-C", str(repo), "show", "-s", "--format=%cI", revision], check=True, capture_output=True, text=True).stdout.strip()
    manifest = {
        "flavor": flavor,
        "source": "questiedb",
        "sourceRevision": revision,
        "generatedAt": commit_time or datetime.now(timezone.utc).isoformat(),
        "status": "partial" if limitations else "complete",
        "limitations": limitations,
    }
    atomic_write_dataset(out, rows, manifest)
    print(f"QuestieDB {flavor}: wrote {len(rows)} records from {revision}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, required=True, help="local QuestieDB checkout")
    parser.add_argument("--flavor", choices=sorted(FLAVORS), required=True)
    parser.add_argument("--revision", help="full source SHA (defaults to local checkout HEAD)")
    parser.add_argument("--out", type=Path, required=True, help="canonical source dataset output directory")
    args = parser.parse_args(argv)
    try:
        import_source(args.repo, args.flavor, args.out, args.revision)
    except (OSError, subprocess.CalledProcessError, LuaLiteralError, ImportErrorMessage) as exc:
        print(f"questiedb-import: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
