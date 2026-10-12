#!/usr/bin/env python3
"""Normalize an existing local Wowhead Forever snapshot without network access."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from canonical import atomic_write_dataset  # noqa: E402

MAX_JSON_BYTES = 64 * 1024 * 1024


class SnapshotError(ValueError):
    pass


def read_json(path: Path) -> Any:
    if path.stat().st_size > MAX_JSON_BYTES:
        raise SnapshotError(f"{path}: exceeds {MAX_JSON_BYTES}-byte limit")
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SnapshotError(f"{path}: invalid UTF-8 JSON: {exc}") from exc


def without_fetch_time(value: dict[str, Any]) -> dict[str, Any]:
    return {key: child for key, child in value.items() if key != "fetchedAt"}


def make_record(entity_type: str, entity_id: str, attrs: dict[str, Any], states: dict[str, str], paths: dict[str, str]) -> dict[str, Any]:
    row = {
        "schemaVersion": 1, "flavor": "forever", "entityType": entity_type, "entityId": entity_id,
        "source": "wowhead", "sourceRevision": "pending", "attributes": attrs,
        "provenance": {field: {"source": "wowhead", "revision": "pending", "path": path} for field, path in sorted(paths.items())},
    }
    if states:
        row["fieldStates"] = states
    return row


def set_value(attrs: dict[str, Any], states: dict[str, str], paths: dict[str, str], field: str, value: Any, path: str) -> None:
    if value is None:
        states[field] = "unavailable"
    else:
        attrs[field] = value
        if value == [] or value == {} or value == "":
            states[field] = "empty"
    paths[field] = path


def detail_entities(root: Path, entity_type: str, expected: int) -> dict[str, tuple[dict[str, Any], str]]:
    folder = root / entity_type
    paths = sorted(folder.glob("*.json")) if folder.is_dir() else []
    if len(paths) != expected:
        raise SnapshotError(f"{entity_type} detail count differs from manifest")
    records = {}
    for path in paths:
        if not path.stem.isdecimal():
            raise SnapshotError(f"invalid {entity_type} detail filename {path.name}")
        body = read_json(path)
        if not isinstance(body, dict) or body.get("id") != int(path.stem) or body.get("kind") != entity_type:
            raise SnapshotError(f"invalid {entity_type} detail record {path.relative_to(root)}")
        records[path.stem] = (body, path.relative_to(root).as_posix())
    return records


def collect_snapshot(root: Path) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    root = root.resolve(strict=True)
    manifest = read_json(root / "manifest.json")
    if not isinstance(manifest, dict) or manifest.get("schemaVersion") != 1 or manifest.get("product") != "forever-quests":
        raise SnapshotError("expected a Wowhead Forever raw manifest with schemaVersion 1")
    stats = manifest.get("stats")
    if not isinstance(stats, dict):
        raise SnapshotError("Wowhead manifest has no stats object")

    index = read_json(root / "quest_index.json")
    if not isinstance(index, dict) or len(index) != stats.get("questIndexCount"):
        raise SnapshotError("quest index count differs from manifest")
    details_dir = root / "details"
    detail_paths = sorted(details_dir.glob("*.json")) if details_dir.is_dir() else []
    if len(detail_paths) != stats.get("questDetailCount"):
        raise SnapshotError("quest detail count differs from manifest")
    details = {}
    for path in detail_paths:
        quest_id = path.stem
        body = read_json(path)
        if not quest_id.isdecimal() or quest_id not in index or not isinstance(body, dict) or body.get("questId") != int(quest_id):
            raise SnapshotError(f"invalid quest detail record {path.relative_to(root)}")
        details[quest_id] = (body, path.relative_to(root).as_posix())

    rows = []
    for key, catalog in index.items():
        if not key.isdecimal() or not isinstance(catalog, dict) or catalog.get("id") != int(key):
            raise SnapshotError(f"invalid quest index entry {key!r}")
        attrs = {"wowheadCatalog": without_fetch_time(catalog)}
        states: dict[str, str] = {}
        paths = {"wowheadCatalog": "quest_index.json"}
        name = catalog.get("name")
        detail = details.get(key)
        name_path = "quest_index.json"
        if name is None and detail:
            page = detail[0].get("pageInfo")
            name = page.get("name") if isinstance(page, dict) else None
            if name is not None:
                name_path = detail[1]
        set_value(attrs, states, paths, "name", name, name_path)
        set_value(attrs, states, paths, "requiredLevel", catalog.get("minLevel"), "quest_index.json")
        listing = catalog.get("list")
        if isinstance(listing, dict):
            set_value(attrs, states, paths, "questLevel", listing.get("level"), "quest_index.json")
        for field in ("classes", "races", "faction", "sourceKinds", "sourceSlugs", "sourceUrls", "zoneId", "zoneId2"):
            if field in catalog:
                set_value(attrs, states, paths, field, catalog[field], "quest_index.json")
        if detail:
            detail_body, detail_path = detail
            attrs["wowheadDetail"] = without_fetch_time(detail_body)
            paths["wowheadDetail"] = detail_path
            for source_field, canonical_field in (("prerequisiteQuestIds", "prerequisiteReferences"), ("startPins", "startPins"), ("mapper", "mapper"), ("infoboxFlags", "flags")):
                if source_field in detail_body:
                    set_value(attrs, states, paths, canonical_field, detail_body[source_field], detail_path)
        else:
            states["detail"] = "unavailable"
            paths["detail"] = "manifest.json"
        rows.append(make_record("quest", key, attrs, states, paths))

    object_index = read_json(root / "object_index.json")
    if not isinstance(object_index, dict) or len(object_index) != stats.get("objectIndexCount"):
        raise SnapshotError("object index count differs from manifest")
    object_details = detail_entities(root, "object", stats.get("objectDetailCount", -1))
    item_details = detail_entities(root, "item", stats.get("itemDetailCount", -1))
    for entity_type, catalog_index, details_by_id in (("object", object_index, object_details), ("item", {}, item_details)):
        for entity_id in sorted(set(catalog_index) | set(details_by_id), key=int):
            catalog = catalog_index.get(entity_id)
            if catalog is not None and (not isinstance(catalog, dict) or catalog.get("id") != int(entity_id)):
                raise SnapshotError(f"invalid {entity_type} index entry {entity_id}")
            attrs: dict[str, Any] = {}
            states: dict[str, str] = {}
            paths: dict[str, str] = {}
            if catalog is not None:
                attrs["wowheadCatalog"] = without_fetch_time(catalog)
                paths["wowheadCatalog"] = "object_index.json"
                for field in ("name", "displayName", "locations", "sourceSlugs", "sourceUrls"):
                    if field in catalog:
                        set_value(attrs, states, paths, field, catalog[field], "object_index.json")
            detail = details_by_id.get(entity_id)
            if detail:
                body, detail_path = detail
                attrs["wowheadDetail"] = without_fetch_time(body)
                paths["wowheadDetail"] = detail_path
                for field in ("name", "spawns", "startsQuestIds", "containedItemIds", "infoboxMarkup"):
                    if field in body:
                        set_value(attrs, states, paths, field, body[field], detail_path)
            else:
                states["detail"] = "unavailable"
                paths["detail"] = "manifest.json"
            rows.append(make_record(entity_type, entity_id, attrs, states, paths))

    zone_path = root / "zone_ui_map_ids.json"
    if zone_path.exists():
        mapping = read_json(zone_path)
        if not isinstance(mapping, dict):
            raise SnapshotError("zone_ui_map_ids.json must be an object")
        reverse: dict[str, list[str]] = {}
        for area_id, data in mapping.items():
            if not area_id.isdecimal() or not isinstance(data, dict) or not isinstance(data.get("uiMapID"), int):
                raise SnapshotError(f"invalid area-to-UiMap mapping {area_id!r}")
            ui_id = str(data["uiMapID"])
            reverse.setdefault(ui_id, []).append(area_id)
            attrs = {"uiMapId": data["uiMapID"], "mappingConfidence": data.get("confidence"), "mappingSamples": data.get("samples")}
            rows.append(make_record("zone", area_id, attrs, {}, {name: "zone_ui_map_ids.json" for name in attrs}))
        for ui_id, areas in reverse.items():
            rows.append(make_record("map", ui_id, {"areaIds": sorted(areas)}, {"geometry": "unavailable", "name": "unavailable"}, {"areaIds": "zone_ui_map_ids.json", "geometry": "zone_ui_map_ids.json", "name": "zone_ui_map_ids.json"}))

    rows.sort(key=lambda row: (row["entityType"], row["entityId"]))
    semantic = [{key: value for key, value in row.items() if key not in {"sourceRevision", "provenance"}} for row in rows]
    revision = "sha256:" + hashlib.sha256(json.dumps(semantic, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    for row in rows:
        row["sourceRevision"] = revision
        for value in row["provenance"].values():
            value["revision"] = revision

    limitations = ["This is the existing local Wowhead snapshot. The daily workflow does not fetch or refresh it."]
    if len(details) != len(index):
        limitations.append(f"{len(index) - len(details)} indexed quest(s) have no validated detail page; those quests remain with detail=unavailable.")
    if stats.get("questDetailFetchErrors"):
        limitations.append(f"The raw manifest reports {stats['questDetailFetchErrors']} quest detail fetch error(s).")
    result = {"source": "wowhead", "flavor": "forever", "sourceRevision": revision,
        "generatedAt": manifest.get("lastFullSyncCompleted") or manifest.get("lastFullSyncStarted"),
        "status": "partial" if len(limitations) > 1 else "complete", "limitations": limitations,
        "upstreamDataEnv": manifest.get("dataEnv"), "upstreamProduct": manifest.get("product")}
    return rows, result


def import_snapshot(root: Path, output: Path) -> None:
    rows, manifest = collect_snapshot(root)
    atomic_write_dataset(output, rows, manifest)
    print(f"Wowhead Forever: wrote {len(rows)} records from {manifest['sourceRevision']}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-root", type=Path, default=Path("data/forever/raw"))
    parser.add_argument("--out", type=Path, default=Path("data/forever/sources/wowhead"))
    args = parser.parse_args(argv)
    try:
        import_snapshot(args.data_root, args.out)
    except (OSError, SnapshotError, ValueError) as exc:
        print(f"wowhead-import: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
