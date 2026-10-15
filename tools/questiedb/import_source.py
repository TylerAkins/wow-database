#!/usr/bin/env python3
"""Import QuestieDB Forever data after its registered Static Corrections."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import tempfile
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
FLAVORS = {"forever": "Forever"}
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


def extract_table(text: str, start: int, *, preserve_expressions: bool = False) -> Any:
    open_at = text.find("{", start)
    if open_at < 0:
        raise ImportErrorMessage("expected a returned Lua table")
    end = matching_brace(text, open_at)
    value = parse(text[open_at:end], preserve_expressions=preserve_expressions)
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
            attributes[name] = json_value(value)
            if _is_empty(value):
                states[name] = "empty"
        row = {
            "schemaVersion": SCHEMA_VERSION,
            "flavor": "forever",
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


def json_value(value: Any) -> Any:
    if isinstance(value, list):
        return [json_value(item) for item in value]
    if isinstance(value, dict):
        result = {}
        for key, item in value.items():
            name = str(key)
            if name in result:
                raise ImportErrorMessage(f"Lua table keys collide in JSON: {name!r}")
            result[name] = json_value(item)
        return result
    return value


def _is_empty(value: Any) -> bool:
    return value is None or value == "" or value == [] or value == {}


# These are the nested structures declared by QuestieDB's table operation schema.
TUPLE_SHAPES = {
    "pair": ("integer", "integer"),
    "coordinate": ("number", "number", "integer"),
    "objective": ("integer", "string", "integer"),
    "kill": ("idarray", "integer", "string", "integer"),
    "reference": ("referenceKind", "integer"),
    "extra": ("spawnlist", "integer", "string", "integer", "references"),
    "trigger": ("string", "spawnlist"),
    "questgivers": ("idarray", "idarray", "idarray"),
    "objectives": ("objectiveList", "objectiveList", "objectiveList", "pair", "killList", "objectiveList"),
}
LIST_SHAPES = {
    "idarray": "integer", "stringarray": "string", "pairs": "pair", "extraobjectives": "extra",
    "objectiveList": "objective", "killList": "kill", "references": "reference", "path": "coordinate",
}
SCALAR_SHAPES = {"integer", "number", "string", "referenceKind"}


def structured_json_value(value: Any, shape: str) -> Any:
    """Keep schema maps keyed and retain nil holes in positional Lua tables."""
    if value is None or shape in SCALAR_SHAPES:
        return value
    if shape in ("spawnlist", "waypointlist"):
        if isinstance(value, list):
            entries = ((str(index), child) for index, child in enumerate(value, 1) if child is not None)
        elif isinstance(value, dict):
            entries = value.items()
        else:
            raise ImportErrorMessage(f"expected {shape} map, got {value!r}")
        child_shape = "coordinate" if shape == "spawnlist" else "path"
        return {str(zone): structured_json_value(rows, "list:" + child_shape) for zone, rows in entries if rows is not None}
    slots = TUPLE_SHAPES.get(shape)
    element = LIST_SHAPES.get(shape)
    if shape.startswith("list:"):
        element = shape.removeprefix("list:")
    if slots is None and element is None:
        raise ImportErrorMessage(f"unsupported QuestieDB structure {shape!r}")
    if isinstance(value, dict):
        indices = {}
        for key, child in value.items():
            if not str(key).isdigit() or int(key) < 1:
                raise ImportErrorMessage(f"non-positional key {key!r} in QuestieDB {shape}")
            indices[int(key)] = child
        value = [indices.get(index) for index in range(1, max(indices, default=0) + 1)]
    if not isinstance(value, list):
        raise ImportErrorMessage(f"expected positional QuestieDB {shape}, got {value!r}")
    if slots is not None and len(value) > len(slots):
        raise ImportErrorMessage(f"unknown tuple slot in QuestieDB {shape}")
    return [structured_json_value(child, slots[index] if slots else element) for index, child in enumerate(value)]


def effective_static_rows(repo: Path, revision: str, lua: str | None = None) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    interpreter = lua or os.environ.get("LUA51") or shutil.which("lua5.1") or shutil.which("lua-5.1")
    if not interpreter:
        raise ImportErrorMessage("Lua 5.1 is required for effective static data; set LUA51 or pass --lua")
    version = subprocess.run([interpreter, "-e", "io.write(_VERSION)"], check=True, capture_output=True, text=True).stdout
    if version != "Lua 5.1":
        raise ImportErrorMessage(f"expected Lua 5.1, found {version!r}")
    bridge = Path(__file__).with_name("effective_static.lua").resolve()
    rows = []
    with tempfile.TemporaryDirectory(prefix="questiedb-static-") as temporary:
        result_path = Path(temporary) / "effective.lua"
        result = subprocess.run([interpreter, str(bridge), str(result_path)], cwd=repo, capture_output=True, text=True)
        if result.returncode:
            raise ImportErrorMessage(f"registered static provider evaluation failed: {result.stderr.strip()}")
        with result_path.open(encoding="utf-8") as stream:
            metadata = json_value(parse(stream.readline()))
            structures = metadata.pop("fieldStructures")
            for line in stream:
                row = json_value(parse(line))
                for key in ("attributes", "fieldStates", "provenance"):
                    if row[key] == []:
                        row[key] = {}
                for field, shape in structures[row["entityType"]].items():
                    if field in row["attributes"]:
                        row["attributes"][field] = structured_json_value(row["attributes"][field], shape)
                for field in row.pop("scalarClears"):
                    row["attributes"][field] = None
                row.update(schemaVersion=SCHEMA_VERSION, flavor="forever", source="questiedb", sourceRevision=revision)
                for provenance in row["provenance"].values():
                    provenance.update(source="questiedb", revision=revision)
                rows.append(row)
    return rows, metadata


def resolve_revision(repo: Path, supplied: str | None) -> str:
    if supplied:
        revision = supplied
    else:
        result = subprocess.run(["git", "-C", str(repo), "rev-parse", "HEAD"], check=True, capture_output=True, text=True)
        revision = result.stdout.strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ImportErrorMessage("source revision must be a full lowercase Git commit SHA")
    return revision


def import_source(repo: Path, flavor: str, out: Path, revision: str | None, lua: str | None = None) -> None:
    if flavor not in FLAVORS:
        raise ImportErrorMessage(f"unsupported flavor {flavor!r}")
    revision = resolve_revision(repo, revision)
    if not (repo / "generator" / "runtime.lua").is_file():
        raise ImportErrorMessage("QuestieDB generator/runtime.lua is required to evaluate registered Static Corrections")
    rows, metadata = effective_static_rows(repo, revision, lua)
    classic_path = repo / "data" / "Classic" / "classicQuestDB.lua"
    classic_ids = {row["entityId"] for row in parse_entity_file(classic_path, "quest", repo)}
    forever_ids = {row["entityId"] for row in rows if row["entityType"] == "quest"}
    missing = sorted(classic_ids - forever_ids, key=int)
    if missing:
        raise ImportErrorMessage(f"Forever generated quests omit {len(missing)} Classic quest ID(s): {', '.join(missing[:20])}")
    limitations = ["Effective static data includes registered Static Corrections only. Dynamic Corrections, Derived Passes, localization, getter defaults and getter normalization are not evaluated."]
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
        **metadata,
    }
    atomic_write_dataset(out, rows, manifest)
    print(f"QuestieDB {flavor}: wrote {len(rows)} records from {revision}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, required=True, help="local QuestieDB checkout")
    parser.add_argument("--flavor", choices=sorted(FLAVORS), required=True)
    parser.add_argument("--lua", help="Lua 5.1 executable (or set LUA51)")
    parser.add_argument("--revision", help="full source SHA (defaults to local checkout HEAD)")
    parser.add_argument("--out", type=Path, required=True, help="canonical source dataset output directory")
    args = parser.parse_args(argv)
    try:
        import_source(args.repo, args.flavor, args.out, args.revision, args.lua)
    except (OSError, subprocess.CalledProcessError, LuaLiteralError, ImportErrorMessage) as exc:
        print(f"questiedb-import: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
