"""Deterministic writer for canonical source datasets."""

from __future__ import annotations

import gzip
import hashlib
import io
import json
import os
import shutil
import tempfile
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1
COMPRESSION_THRESHOLD = 50 * 1024 * 1024
SOURCES = {"wowhead", "allthethings", "questiedb", "foreverclient", "compilation"}
UPSTREAM_SOURCES = SOURCES - {"compilation"}
FLAVORS = {"forever"}
ENTITY_TYPES = {"quest", "npc", "item", "object", "zone", "map", "spell", "faction", "achievement", "profession", "relationship", "questmetadata"}
FIELD_STATES = {"missing", "unsupported", "unavailable", "invalid", "empty", "conflicting"}


def validate_rows(rows: list[dict[str, Any]], manifest: dict[str, Any]) -> None:
    source = manifest.get("source")
    flavor = manifest.get("flavor")
    revision = manifest.get("sourceRevision")
    if source not in SOURCES or flavor not in FLAVORS or not isinstance(revision, str) or not revision:
        raise ValueError("manifest requires a supported source, flavor, and sourceRevision")
    seen: set[tuple[str, str, str]] = set()
    for number, row in enumerate(rows, 1):
        if row.get("schemaVersion") != SCHEMA_VERSION:
            raise ValueError(f"record {number}: schemaVersion must be {SCHEMA_VERSION}")
        if row.get("source") != source or row.get("flavor") != flavor or row.get("sourceRevision") != revision:
            raise ValueError(f"record {number}: source, flavor, or revision differs from manifest")
        entity_type = row.get("entityType")
        entity_id = row.get("entityId")
        if entity_type not in ENTITY_TYPES or not isinstance(entity_id, str) or not entity_id.strip() or any(c in entity_id for c in "\r\n\t"):
            raise ValueError(f"record {number}: invalid entity type or ID")
        key = (flavor, entity_type, entity_id)
        if key in seen:
            raise ValueError(f"record {number}: duplicate entity {key}")
        seen.add(key)
        attributes = row.get("attributes")
        states = row.get("fieldStates", {})
        provenance = row.get("provenance", {})
        if not isinstance(attributes, dict) or not isinstance(states, dict) or not isinstance(provenance, dict):
            raise ValueError(f"record {number}: attributes, fieldStates, and provenance must be objects")
        for field, state in states.items():
            if state not in FIELD_STATES:
                raise ValueError(f"record {number}: unsupported state {state!r} for {field}")
            if state == "empty" and field not in attributes:
                raise ValueError(f"record {number}: empty field {field} has no explicit value")
            if field in attributes and state not in {"empty", "invalid"}:
                raise ValueError(f"record {number}: field {field} has both a value and state {state}")
        for field, value in provenance.items():
            if field not in attributes and field not in states:
                raise ValueError(f"record {number}: provenance for {field} has no field value or state")
            if not isinstance(value, dict):
                raise ValueError(f"record {number}: provenance for {field} must be an object")
            if value.get("source") != source or value.get("revision") != revision:
                raise ValueError(f"record {number}: provenance for {field} does not match manifest")


def dataset_content(rows: list[dict[str, Any]], manifest: dict[str, Any]) -> bytes:
    ordered = sorted(rows, key=lambda row: (row["entityType"], row["entityId"]))
    validate_rows(ordered, manifest)
    return b"".join(
        (json.dumps(row, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
        for row in ordered
    )


def atomic_write_dataset(output: Path, rows: list[dict[str, Any]], manifest: dict[str, Any]) -> None:
    content = dataset_content(rows, manifest)
    result = dict(manifest)
    result["schemaVersion"] = SCHEMA_VERSION
    result["recordCount"] = len(rows)
    result["recordsSha256"] = hashlib.sha256(content).hexdigest()
    parent = output.parent
    parent.mkdir(parents=True, exist_ok=True)
    temp = Path(tempfile.mkdtemp(prefix=".dataset-", dir=parent))
    old = output.with_name(output.name + ".old")
    try:
        if len(content) >= COMPRESSION_THRESHOLD:
            buffer = io.BytesIO()
            with gzip.GzipFile(filename="", mode="wb", fileobj=buffer, mtime=0) as compressed:
                compressed.write(content)
            (temp / "records.jsonl.gz").write_bytes(buffer.getvalue())
        else:
            (temp / "records.jsonl").write_bytes(content)
        (temp / "manifest.json").write_text(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        if old.exists():
            shutil.rmtree(old)
        if output.exists():
            os.replace(output, old)
        try:
            os.replace(temp, output)
        except OSError:
            if old.exists() and not output.exists():
                os.replace(old, output)
            raise
        if old.exists():
            shutil.rmtree(old)
    finally:
        if temp.exists():
            shutil.rmtree(temp)
