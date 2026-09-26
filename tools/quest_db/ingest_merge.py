"""Merge parsed list rows into committed JSON indexes."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .sources import SourcePage
from .store import write_json


def merge_quest_list_rows(data_root: Path, rows: list[dict[str, Any]], page: SourcePage) -> int:
    index_path = data_root / "quest_index.json"
    quest_index: dict[str, Any] = {}
    if index_path.is_file():
        quest_index = json.loads(index_path.read_text(encoding="utf-8"))

    for row in rows:
        qid = str(row["id"])
        existing = quest_index.get(qid)
        if existing is None:
            quest_index[qid] = _new_index_entry(row, page)
        else:
            _merge_index_entry(existing, row, page)

    write_json(index_path, quest_index)
    return len(quest_index)


def merge_object_rows(data_root: Path, rows: list[dict[str, Any]], page: SourcePage) -> int:
    index_path = data_root / "object_index.json"
    object_index: dict[str, Any] = {}
    if index_path.is_file():
        object_index = json.loads(index_path.read_text(encoding="utf-8"))

    for row in rows:
        oid = str(row["id"])
        existing = object_index.get(oid)
        if existing is None:
            object_index[oid] = {
                "id": row["id"],
                "name": row.get("name") or row.get("displayName"),
                "displayName": row.get("displayName"),
                "locations": row.get("location") or [],
                "type": row.get("type"),
                "list": row,
                "sourceSlugs": [page.slug],
                "sourceUrls": [page.url],
            }
        else:
            slugs = set(existing.get("sourceSlugs") or [])
            urls = set(existing.get("sourceUrls") or [])
            slugs.add(page.slug)
            urls.add(page.url)
            existing["sourceSlugs"] = sorted(slugs)
            existing["sourceUrls"] = sorted(urls)

    write_json(index_path, object_index)
    return len(object_index)


def _new_index_entry(row: dict[str, Any], page: SourcePage) -> dict[str, Any]:
    return {
        "id": row["id"],
        "name": row.get("name"),
        "zoneId": row.get("category"),
        "zoneId2": row.get("category2"),
        "list": row,
        "sourceSlugs": [page.slug],
        "sourceKinds": [page.kind],
        "sourceUrls": [page.url],
        "hasDetail": False,
    }


def _merge_index_entry(existing: dict[str, Any], row: dict[str, Any], page: SourcePage) -> None:
    slugs = set(existing.get("sourceSlugs") or [])
    kinds = set(existing.get("sourceKinds") or [])
    urls = set(existing.get("sourceUrls") or [])
    slugs.add(page.slug)
    kinds.add(page.kind)
    urls.add(page.url)
    existing["sourceSlugs"] = sorted(slugs)
    existing["sourceKinds"] = sorted(kinds)
    existing["sourceUrls"] = sorted(urls)
    if not existing.get("name"):
        existing["name"] = row.get("name")
    if existing.get("zoneId") in (None, 0):
        existing["zoneId"] = row.get("category")
