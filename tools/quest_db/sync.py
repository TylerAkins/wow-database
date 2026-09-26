"""Orchestrate Source list + quest detail synchronization."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Protocol

from .classify import classify_pin_category
from .http import item_detail_url, object_detail_url, quest_detail_url


class _HtmlClient(Protocol):
    def get_html(self, url: str, *, force: bool = False) -> str: ...
from .ingest import ingest_html
from .parse_page import extract_g_mapper_spawns, extract_inline_listviews, extract_map_quest_givers, listview_ids
from .parse_quest import (
    eligibility_restrictions,
    extract_spawn_pins,
    extract_start_pins,
    parse_quest_detail,
    quest_ids_from_markup,
)
from .sources import SOURCE_PAGES
from .store import load_manifest, save_manifest, utc_now_iso, write_json
from .zone_resolver import bootstrap_zone_ui_map_ids


def sync_sources(
    data_root: Path,
    client: _HtmlClient,
    *,
    force: bool = False,
) -> dict[str, Any]:
    manifest = load_manifest(data_root)
    manifest["lastFullSyncStarted"] = utc_now_iso()
    save_manifest(data_root, manifest)

    for page in SOURCE_PAGES:
        html = client.get_html(page.url, force=force)
        ingest_html(data_root, page.url, html)

    manifest = load_manifest(data_root)
    manifest["stats"]["sourcePageCount"] = len(SOURCE_PAGES)
    manifest["lastFullSyncCompleted"] = utc_now_iso()
    save_manifest(data_root, manifest)
    return manifest


def sync_quest_details(
    data_root: Path,
    client: _HtmlClient,
    *,
    limit: int | None = None,
    quest_ids: list[int] | None = None,
    force: bool = False,
    attunement_seed_path: Path | None = None,
) -> dict[str, Any]:
    manifest = load_manifest(data_root)
    index_path = data_root / "quest_index.json"
    if not index_path.is_file():
        raise SystemExit("quest_index.json missing; run sync-sources or ingest list URLs first")

    quest_index: dict[str, Any] = json.loads(index_path.read_text(encoding="utf-8"))
    details_dir = data_root / "details"
    details_dir.mkdir(parents=True, exist_ok=True)
    errors_path = data_root / "fetch_errors.jsonl"

    attunement_ids = _load_attunement_ids(data_root, attunement_seed_path)

    processed = 0
    skipped_errors = 0
    wanted = {str(qid) for qid in quest_ids} if quest_ids else None
    pending = [
        qid
        for qid in sorted(quest_index.keys(), key=int)
        if (wanted is None or qid in wanted)
        and (force or not (details_dir / f"{qid}.json").is_file())
    ]
    if limit is not None:
        pending = pending[:limit]
    print(
        f"quest details: {len(quest_index) - len(pending)} already saved, {len(pending)} to fetch",
        flush=True,
    )

    for index, qid in enumerate(pending, start=1):
        detail_path = details_dir / f"{qid}.json"
        entry = quest_index[qid]
        url = quest_detail_url(int(qid), entry.get("name"))
        print(f"[{index}/{len(pending)}] {url}", flush=True)
        try:
            html = client.get_html(url, force=force)
        except Exception as exc:  # noqa: BLE001 — log and continue batch
            skipped_errors += 1
            print(f"  failed: {exc}", flush=True)
            with errors_path.open("a", encoding="utf-8") as handle:
                handle.write(json.dumps({"questId": int(qid), "url": url, "error": str(exc)}) + "\n")
            continue

        detail = parse_quest_detail(html, int(qid))
        detail["fetchedAt"] = utc_now_iso()
        detail["startPins"] = extract_start_pins(detail.get("mapper"))

        pin_category = classify_pin_category(
            source_kinds=set(entry.get("sourceKinds") or []),
            list_row=entry.get("list"),
            detail_flags=detail.get("infoboxFlags"),
            attunement_ids=attunement_ids,
            quest_id=int(qid),
        )
        detail["pinCategory"] = pin_category
        restrictions = eligibility_restrictions(detail.get("infoboxMarkup"), entry.get("list"))
        detail.update(restrictions)
        write_json(detail_path, detail)

        entry["pinCategory"] = pin_category
        entry.update(restrictions)
        entry["hasDetail"] = True
        entry["startPinCount"] = len(detail["startPins"])
        if detail["startPins"]:
            entry["primaryStart"] = detail["startPins"][0]
        processed += 1
        print(
            f"  saved quest {qid} faction={restrictions.get('faction')} races={restrictions.get('races')} starts={len(detail['startPins'])}",
            flush=True,
        )

        if processed % 25 == 0:
            write_json(index_path, quest_index)
            manifest["stats"]["questDetailCount"] = len(list(details_dir.glob("*.json")))
            save_manifest(data_root, manifest)

    manifest["stats"]["questDetailCount"] = len(list(details_dir.glob("*.json")))
    manifest["stats"]["questIndexCount"] = len(quest_index)
    manifest["stats"]["questDetailFetchErrors"] = skipped_errors
    manifest["lastFullSyncCompleted"] = utc_now_iso()
    write_json(index_path, quest_index)
    save_manifest(data_root, manifest)
    return manifest


def zone_page_url(zone_id: int) -> str:
    return f"https://www.wowhead.com/forever/zone={zone_id}"


def sync_zone_starters(
    data_root: Path,
    client: _HtmlClient,
    *,
    limit: int | None = None,
    zone_ids: list[int] | None = None,
    force: bool = False,
) -> dict[str, Any]:
    """Read each zone's starts-quest tab and record item and object starters."""
    manifest = load_manifest(data_root)
    errors_path = data_root / "fetch_errors.jsonl"
    index_path = data_root / "quest_index.json"
    if not index_path.is_file():
        raise SystemExit("quest_index.json missing; ingest list URLs first")
    quest_index = json.loads(index_path.read_text(encoding="utf-8"))
    if zone_ids is None:
        zone_ids = sorted(
            {
                int(entry["zoneId"])
                for entry in quest_index.values()
                if isinstance(entry.get("zoneId"), int) and entry["zoneId"] > 0
            }
        )
    else:
        zone_ids = sorted(set(zone_ids))
    if limit is not None:
        zone_ids = zone_ids[:limit]

    starters_path = data_root / "zone_starters.json"
    starters: dict[str, Any] = {}
    if starters_path.is_file() and not force:
        starters = json.loads(starters_path.read_text(encoding="utf-8"))

    print(f"zone pages: {len(zone_ids)} to fetch", flush=True)
    for index, zone_id in enumerate(zone_ids, start=1):
        url = zone_page_url(zone_id)
        print(f"[{index}/{len(zone_ids)}] {url}", flush=True)
        try:
            html = client.get_html(url, force=force)
        except Exception as exc:  # noqa: BLE001 — log and continue batch
            print(f"  failed: {exc}", flush=True)
            with errors_path.open("a", encoding="utf-8") as handle:
                handle.write(json.dumps({"kind": "zone", "id": zone_id, "url": url, "error": str(exc)}) + "\n")
            continue
        added_objects = _merge_zone_starters(data_root, starters, zone_id, extract_inline_listviews(html))
        givers = extract_map_quest_givers(html)
        zone_givers = starters.setdefault("givers", {})
        zone_givers[str(zone_id)] = givers
        print(
            f"  zone {zone_id} item starters={_item_count(starters, zone_id)} "
            f"map givers={len(givers)} new objects={added_objects}",
            flush=True,
        )
        if index % 10 == 0:
            write_json(starters_path, starters)

    write_json(starters_path, starters)
    manifest["stats"]["zoneStarterItems"] = len(starters.get("items") or {})
    manifest["stats"]["zoneStarterObjects"] = len(starters.get("objects") or {})
    save_manifest(data_root, manifest)
    print(
        f"starters: {manifest['stats']['zoneStarterItems']} items, "
        f"{manifest['stats']['zoneStarterObjects']} objects",
        flush=True,
    )
    return manifest


def _item_count(starters: dict[str, Any], zone_id: int) -> int:
    return sum(1 for item in (starters.get("items") or {}).values() if item.get("zoneId") == zone_id)


def _merge_zone_starters(
    data_root: Path,
    starters: dict[str, Any],
    zone_id: int,
    views: list[dict[str, Any]],
) -> int:
    items = starters.setdefault("items", {})
    objects = starters.setdefault("objects", {})
    added = 0
    for view in views:
        if view.get("id") != "starts-quest":
            continue
        for row in view.get("data") or []:
            item_id = row.get("id")
            if not isinstance(item_id, int):
                continue
            sources = []
            for source in row.get("sourcemore") or []:
                if not isinstance(source, dict):
                    continue
                sources.append(
                    {
                        "type": source.get("t"),
                        "id": source.get("ti"),
                        "name": source.get("n"),
                        "zoneId": source.get("z", zone_id),
                    }
                )
                if source.get("t") == 2 and isinstance(source.get("ti"), int):
                    key = str(source["ti"])
                    if key not in objects:
                        added += 1
                    objects[key] = {
                        "id": source["ti"],
                        "name": source.get("n") or row.get("name"),
                        "zoneId": source.get("z", zone_id),
                        "itemId": item_id,
                    }
            items[str(item_id)] = {
                "id": item_id,
                "name": row.get("name"),
                "zoneId": zone_id,
                "sources": sources,
            }
    _add_starter_objects_to_index(data_root, objects)
    return added


def _add_starter_objects_to_index(data_root: Path, objects: dict[str, Any]) -> None:
    index_path = data_root / "object_index.json"
    object_index: dict[str, Any] = {}
    if index_path.is_file():
        object_index = json.loads(index_path.read_text(encoding="utf-8"))
    changed = False
    for key, obj in objects.items():
        if key in object_index:
            continue
        object_index[key] = {
            "id": obj["id"],
            "name": obj.get("name"),
            "displayName": obj.get("name"),
            "locations": [obj.get("zoneId")] if obj.get("zoneId") is not None else [],
            "fromZoneStarter": True,
            "itemId": obj.get("itemId"),
        }
        changed = True
    if changed:
        write_json(index_path, object_index)


def sync_object_details(
    data_root: Path,
    client: _HtmlClient,
    *,
    limit: int | None = None,
    object_ids: list[int] | None = None,
    item_ids: list[int] | None = None,
    force: bool = False,
) -> dict[str, Any]:
    """Download object (and optional item) pages and copy every spawn onto the quest."""
    manifest = load_manifest(data_root)
    errors_path = data_root / "fetch_errors.jsonl"
    jobs = _object_jobs(data_root, object_ids=object_ids, item_ids=item_ids, force=force)
    if limit is not None:
        jobs = jobs[:limit]
    print(f"object/item pages: {len(jobs)} to fetch", flush=True)

    processed = 0
    for index, job in enumerate(jobs, start=1):
        print(f"[{index}/{len(jobs)}] {job['url']}", flush=True)
        try:
            html = client.get_html(job["url"], force=force)
        except Exception as exc:  # noqa: BLE001 — log and continue batch
            print(f"  failed: {exc}", flush=True)
            with errors_path.open("a", encoding="utf-8") as handle:
                handle.write(
                    json.dumps({"kind": job["kind"], "id": job["id"], "url": job["url"], "error": str(exc)}) + "\n"
                )
            continue
        detail = parse_quest_detail(html, job["id"])
        page_info = detail.get("pageInfo") if isinstance(detail.get("pageInfo"), dict) else {}
        spawns = extract_g_mapper_spawns(html) or extract_spawn_pins(detail.get("mapper"))
        quest_ids = listview_ids(html, "starts") or quest_ids_from_markup(detail.get("infoboxMarkup"))
        contained_items = listview_ids(html, "contains") if job["kind"] == "object" else []
        for item_id in contained_items:
            quest_ids = _merge_ids(quest_ids, _quest_ids_from_item(client, item_id, force=False))
        record = {
            "id": job["id"],
            "kind": job["kind"],
            "name": job.get("name") or page_info.get("name"),
            "fetchedAt": utc_now_iso(),
            "spawns": spawns,
            "startsQuestIds": quest_ids,
            "containedItemIds": contained_items,
            "infoboxMarkup": detail.get("infoboxMarkup"),
        }
        write_json(data_root / job["kind"] / f"{job['id']}.json", record)
        attached = _attach_spawns_to_quests(data_root, record)
        processed += 1
        print(
            f"  saved {job['kind']} {job['id']} spawns={len(spawns)} quests={quest_ids} attached={attached}",
            flush=True,
        )
        if processed % 25 == 0:
            manifest["stats"]["objectDetailCount"] = _json_count(data_root / "object")
            save_manifest(data_root, manifest)

    manifest["stats"]["objectDetailCount"] = _json_count(data_root / "object")
    manifest["stats"]["itemDetailCount"] = _json_count(data_root / "item")
    save_manifest(data_root, manifest)
    return manifest


def _merge_ids(left: list[int], right: list[int]) -> list[int]:
    return sorted(set(left) | set(right))


def _quest_ids_from_item(client: _HtmlClient, item_id: int, *, force: bool) -> list[int]:
    url = item_detail_url(item_id)
    try:
        html = client.get_html(url, force=force)
    except Exception as exc:  # noqa: BLE001 — object spawns still save if the item page fails
        print(f"  contained item {item_id} failed: {exc}", flush=True)
        return []
    detail = parse_quest_detail(html, item_id)
    return listview_ids(html, "starts") or quest_ids_from_markup(detail.get("infoboxMarkup"))


def _json_count(path: Path) -> int:
    if not path.is_dir():
        return 0
    return len(list(path.glob("*.json")))


def _object_jobs(
    data_root: Path,
    *,
    object_ids: list[int] | None,
    item_ids: list[int] | None,
    force: bool,
) -> list[dict[str, Any]]:
    jobs: list[dict[str, Any]] = []
    fetch_objects = object_ids is not None or item_ids is None
    if fetch_objects:
        index_path = data_root / "object_index.json"
        index: dict[str, Any] = {}
        if index_path.is_file():
            index = json.loads(index_path.read_text(encoding="utf-8"))
        ids = set(object_ids) if object_ids is not None else {int(key) for key in index}
        if object_ids is None:
            ids.update(int(path.stem) for path in (data_root / "object").glob("*.json") if path.stem.isdigit())
        for oid in sorted(ids):
            path = data_root / "object" / f"{oid}.json"
            if not force and path.is_file():
                continue
            entry = index.get(str(oid), {})
            name = entry.get("name") or entry.get("displayName")
            jobs.append(
                {
                    "kind": "object",
                    "id": oid,
                    "name": name,
                    "url": object_detail_url(oid, name),
                }
            )
    for iid in sorted(set(item_ids or [])):
        path = data_root / "item" / f"{iid}.json"
        if not force and path.is_file():
            continue
        jobs.append({"kind": "item", "id": iid, "name": None, "url": item_detail_url(iid)})
    return jobs


def _attach_spawns_to_quests(data_root: Path, record: dict[str, Any]) -> int:
    if not record["spawns"] or not record["startsQuestIds"]:
        return 0
    details_dir = data_root / "details"
    index_path = data_root / "quest_index.json"
    quest_index: dict[str, Any] = {}
    if index_path.is_file():
        quest_index = json.loads(index_path.read_text(encoding="utf-8"))
    attached = 0
    for qid in record["startsQuestIds"]:
        detail_path = details_dir / f"{qid}.json"
        if not detail_path.is_file():
            continue
        detail = json.loads(detail_path.read_text(encoding="utf-8"))
        pins = _merge_spawn_pins(detail.get("objectSpawns") or [], record)
        detail["objectSpawns"] = pins
        if not detail.get("startPins"):
            detail["startPins"] = pins
        write_json(detail_path, detail)
        entry = quest_index.get(str(qid))
        if entry is not None:
            entry["startPinCount"] = len(detail["startPins"])
            entry["objectSpawnCount"] = len(pins)
            if detail["startPins"]:
                entry["primaryStart"] = detail["startPins"][0]
        attached += 1
    if quest_index:
        write_json(index_path, quest_index)
    return attached


def _merge_spawn_pins(existing: list[dict[str, Any]], record: dict[str, Any]) -> list[dict[str, Any]]:
    merged = [pin for pin in existing if pin.get("entityId") != record["id"] or pin.get("entityKind") != record["kind"]]
    for pin in record["spawns"]:
        copied = dict(pin)
        copied["entityKind"] = record["kind"]
        copied["entityId"] = record["id"]
        copied["entityName"] = record.get("name")
        merged.append(copied)
    return merged


def backfill_eligibility(data_root: Path) -> int:
    """Fill faction/races/classes/minLevel on detail files already downloaded."""
    index_path = data_root / "quest_index.json"
    quest_index: dict[str, Any] = {}
    if index_path.is_file():
        quest_index = json.loads(index_path.read_text(encoding="utf-8"))

    updated = 0
    details_dir = data_root / "details"
    for path in details_dir.glob("*.json"):
        detail = json.loads(path.read_text(encoding="utf-8"))
        qid = str(detail.get("questId") or path.stem)
        entry = quest_index.get(qid, {})
        restrictions = eligibility_restrictions(detail.get("infoboxMarkup"), entry.get("list"))
        detail.update(restrictions)
        write_json(path, detail)
        if entry:
            entry.update(restrictions)
            quest_index[qid] = entry
        updated += 1

    if quest_index:
        write_json(index_path, quest_index)
    return updated


def rebuild_zone_map(data_root: Path, att_quests_lua: Path) -> dict[str, Any]:
    index = json.loads((data_root / "quest_index.json").read_text(encoding="utf-8"))
    zone_map = bootstrap_zone_ui_map_ids(index, att_quests_lua)
    write_json(data_root / "zone_ui_map_ids.json", zone_map)
    return zone_map


def _load_attunement_ids(data_root: Path, seed_path: Path | None) -> set[int]:
    path = seed_path or (data_root / "attunement_quest_ids.json")
    if path.is_file():
        raw = json.loads(path.read_text(encoding="utf-8"))
        if isinstance(raw, list):
            return {int(x) for x in raw}
    return set()
