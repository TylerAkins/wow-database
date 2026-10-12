"""Read/write Source database JSON artifacts."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def default_manifest() -> dict[str, Any]:
    return {
        "schemaVersion": SCHEMA_VERSION,
        "dataEnv": 16,
        "product": "forever-quests",
        "lastCheckedForChanges": None,
        "lastFullSyncStarted": None,
        "lastFullSyncCompleted": None,
        "sources": {},
        "stats": {
            "questIndexCount": 0,
            "questDetailCount": 0,
            "objectIndexCount": 0,
            "sourcePageCount": 0,
        },
    }


def load_manifest(root: Path) -> dict[str, Any]:
    path = root / "manifest.json"
    if not path.is_file():
        return default_manifest()
    data = read_json(path)
    if not isinstance(data, dict):
        return default_manifest()
    return data


def save_manifest(root: Path, manifest: dict[str, Any]) -> None:
    write_json(root / "manifest.json", manifest)
