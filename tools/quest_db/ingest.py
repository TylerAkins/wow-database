"""Ingest Source pages from fetched HTML (paste-URL workflow)."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .classify import classify_pin_category
from .http import quest_detail_url
from .ingest_merge import merge_object_rows, merge_quest_list_rows
from .parse_page import extract_page_listviews, quest_id_from_url
from .parse_quest import eligibility_restrictions, extract_start_pins, parse_quest_detail
from .sources import SOURCE_PAGES, SourcePage
from .store import load_manifest, save_manifest, utc_now_iso, write_json

_FOREVER_PREFIX = "https://www.wowhead.com/forever/"


def resolve_source_page(url: str) -> SourcePage | None:
    normalized = url.split("?", 1)[0].rstrip("/")
    for page in SOURCE_PAGES:
        if page.url.rstrip("/") == normalized:
            return page
    return None


def infer_source_page(url: str) -> SourcePage:
    known = resolve_source_page(url)
    if known is not None:
        return known

    normalized = url.split("?", 1)[0]
    if not normalized.startswith(_FOREVER_PREFIX):
        raise ValueError(f"Not a Source Forever URL: {url}")

    rel = normalized[len(_FOREVER_PREFIX) :]
    kind = "uncategorized"
    if rel.startswith("quests/battlegrounds/"):
        kind = "battleground"
    elif rel.startswith("quests/dungeons/"):
        kind = "dungeon"
    elif rel.startswith("quests/raids/"):
        kind = "raid"
    elif rel.startswith("quests/world-events/"):
        kind = "world_event"
    elif rel.startswith("quests/professions/"):
        kind = "profession"
    elif rel.startswith("quests/classes/"):
        kind = "class"
    elif rel.startswith("quests/"):
        kind = "zone"
    elif rel.startswith("objects/"):
        kind = "objects"

    slug = rel.replace("/", "_").replace("-", "-")
    return SourcePage(url=normalized, kind=kind, slug=slug)


def ingest_html(data_root: Path, url: str, html: str) -> dict[str, Any]:
    data_root.mkdir(parents=True, exist_ok=True)
    manifest = load_manifest(data_root)
    manifest["lastCheckedForChanges"] = utc_now_iso()

    quest_id = quest_id_from_url(url)
    if quest_id is not None:
        return _ingest_quest_detail(data_root, manifest, quest_id, html)

    page = infer_source_page(url)
    listviews = extract_page_listviews(html)
    if not listviews:
        raise ValueError(f"No list data found in HTML for {url}")

    quest_rows: list[dict[str, Any]] = []
    object_rows: list[dict[str, Any]] = []
    for view in listviews:
        template = view.get("template")
        data = view.get("data") or []
        if template == "quest":
            quest_rows.extend(data)
        elif template == "object":
            object_rows.extend(data)

    report: dict[str, Any] = {
        "url": url,
        "slug": page.slug,
        "kind": page.kind,
        "fetchedAt": utc_now_iso(),
        "questRows": len(quest_rows),
        "objectRows": len(object_rows),
    }

    sources_dir = data_root / "sources"
    sources_dir.mkdir(parents=True, exist_ok=True)

    if quest_rows:
        snapshot = {
            "url": page.url,
            "kind": page.kind,
            "slug": page.slug,
            "fetchedAt": report["fetchedAt"],
            "questCount": len(quest_rows),
            "quests": quest_rows,
        }
        write_json(sources_dir / f"{page.slug.replace('/', '_')}.json", snapshot)
        index_count = merge_quest_list_rows(data_root, quest_rows, page)
        report["questIndexSize"] = index_count
        manifest["sources"][page.slug] = {
            "url": page.url,
            "kind": page.kind,
            "lastFetched": report["fetchedAt"],
            "questCount": len(quest_rows),
        }

    if object_rows:
        obj_snapshot = {
            "url": page.url,
            "kind": page.kind,
            "slug": page.slug,
            "fetchedAt": report["fetchedAt"],
            "objectCount": len(object_rows),
            "objects": object_rows,
        }
        write_json(sources_dir / f"{page.slug.replace('/', '_')}.json", obj_snapshot)
        object_count = merge_object_rows(data_root, object_rows, page)
        report["objectIndexSize"] = object_count
        manifest["sources"][page.slug] = {
            "url": page.url,
            "kind": page.kind,
            "lastFetched": report["fetchedAt"],
            "objectCount": len(object_rows),
        }

    manifest["stats"]["sourcePageCount"] = len(manifest.get("sources", {}))
    if (data_root / "quest_index.json").is_file():
        manifest["stats"]["questIndexCount"] = len(
            json.loads((data_root / "quest_index.json").read_text(encoding="utf-8"))
        )
    if (data_root / "object_index.json").is_file():
        manifest["stats"]["objectIndexCount"] = len(
            json.loads((data_root / "object_index.json").read_text(encoding="utf-8"))
        )
    save_manifest(data_root, manifest)
    return report


def _ingest_quest_detail(
    data_root: Path,
    manifest: dict[str, Any],
    quest_id: int,
    html: str,
) -> dict[str, Any]:
    detail = parse_quest_detail(html, quest_id)
    detail["fetchedAt"] = utc_now_iso()
    detail["startPins"] = extract_start_pins(detail.get("mapper"))

    index_path = data_root / "quest_index.json"
    quest_index: dict[str, Any] = {}
    if index_path.is_file():
        quest_index = json.loads(index_path.read_text(encoding="utf-8"))

    entry = quest_index.get(str(quest_id), {"id": quest_id, "sourceKinds": [], "sourceSlugs": []})
    attunement_path = data_root / "attunement_quest_ids.json"
    attunement_ids: set[int] = set()
    if attunement_path.is_file():
        raw = json.loads(attunement_path.read_text(encoding="utf-8"))
        if isinstance(raw, list):
            attunement_ids = {int(x) for x in raw}

    pin_category = classify_pin_category(
        source_kinds=set(entry.get("sourceKinds") or []),
        list_row=entry.get("list"),
        detail_flags=detail.get("infoboxFlags"),
        attunement_ids=attunement_ids,
        quest_id=quest_id,
    )
    detail["pinCategory"] = pin_category
    restrictions = eligibility_restrictions(detail.get("infoboxMarkup"), entry.get("list"))
    detail.update(restrictions)

    details_dir = data_root / "details"
    details_dir.mkdir(parents=True, exist_ok=True)
    write_json(details_dir / f"{quest_id}.json", detail)

    entry["pinCategory"] = pin_category
    entry.update(restrictions)
    entry["hasDetail"] = True
    entry["startPinCount"] = len(detail["startPins"])
    if detail["startPins"]:
        entry["primaryStart"] = detail["startPins"][0]
    quest_index[str(quest_id)] = entry
    write_json(index_path, quest_index)

    manifest["stats"]["questDetailCount"] = len(list(details_dir.glob("*.json")))
    manifest["stats"]["questIndexCount"] = len(quest_index)
    save_manifest(data_root, manifest)

    return {
        "url": quest_detail_url(quest_id, entry.get("name")),
        "questId": quest_id,
        "pinCategory": pin_category,
        "startPinCount": len(detail["startPins"]),
        "fetchedAt": detail["fetchedAt"],
    }
