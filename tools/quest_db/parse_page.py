"""Extract Source list data from saved HTML (inline Listview + JSON listviews)."""

from __future__ import annotations

import json
import re
from typing import Any

from .parse_list import parse_quest_list

_LISTVIEWS_JSON_RE = re.compile(
    r'<script type="application/json" id="data\.page\.listPage\.listviews">(\[.*?\])</script>',
    re.DOTALL,
)
_QUEST_ID_RE = re.compile(r"quest=(\d+)")


def extract_inline_listviews(html: str) -> list[dict[str, Any]]:
    """Parse `new Listview({... data: [...]})` blocks, including zone tabs."""
    views: list[dict[str, Any]] = []
    decoder = json.JSONDecoder()
    for match in re.finditer(r"new Listview\(\{", html):
        window = html[match.start() : match.start() + 700]
        template_match = re.search(r"template:\s*'([^']+)'", window)
        id_match = re.search(r"id:\s*'([^']+)'", window)
        data_at = window.find("data:")
        if template_match is None or data_at < 0:
            continue
        bracket = html.find("[", match.start() + data_at)
        if bracket < 0:
            continue
        try:
            data, _end = decoder.raw_decode(html[bracket:])
        except json.JSONDecodeError:
            continue
        if not isinstance(data, list):
            continue
        views.append(
            {
                "template": template_match.group(1),
                "id": id_match.group(1) if id_match else None,
                "data": [row for row in data if isinstance(row, dict)],
            }
        )
    return views


def extract_g_mapper_spawns(html: str) -> list[dict[str, Any]]:
    """Coordinates from an object page's `g_mapperData` block."""
    marker = "var g_mapperData = "
    start = html.find(marker)
    if start < 0:
        return []
    try:
        payload, _end = json.JSONDecoder().raw_decode(html[start + len(marker) :])
    except json.JSONDecodeError:
        return []
    if not isinstance(payload, dict):
        return []
    pins: list[dict[str, Any]] = []
    for zone_id, rows in payload.items():
        if not isinstance(rows, list):
            continue
        for row in rows:
            if not isinstance(row, dict):
                continue
            for pair in row.get("coords") or []:
                if not (isinstance(pair, list) and len(pair) == 2):
                    continue
                pins.append(
                    {
                        "zoneId": int(zone_id) if str(zone_id).isdigit() else zone_id,
                        "uiMapId": row.get("uiMapId"),
                        "uiMapName": row.get("uiMapName"),
                        "x": float(pair[0]),
                        "y": float(pair[1]),
                    }
                )
    return pins


def listview_ids(html: str, view_id: str) -> list[int]:
    ids: list[int] = []
    for view in extract_inline_listviews(html):
        if view.get("id") != view_id:
            continue
        for row in view.get("data") or []:
            row_id = row.get("id")
            if isinstance(row_id, int):
                ids.append(row_id)
    return ids


def extract_map_quest_givers(html: str) -> list[dict[str, Any]]:
    """NPCs and objects plotted as quest givers on a zone map.

    Quest entries on this map have a name but not a quest id.
    """
    marker = "var mapShowObject = new ShowOnMap("
    start = html.find(marker)
    if start < 0:
        return []
    try:
        payload, _end = json.JSONDecoder().raw_decode(html[start + len(marker) :])
    except json.JSONDecodeError:
        return []
    if not isinstance(payload, dict):
        return []
    givers: list[dict[str, Any]] = []
    for side in ("alliancequests", "hordequests"):
        for row in payload.get(side) or []:
            if not isinstance(row, dict):
                continue
            coords = []
            for pair in row.get("coords") or []:
                if isinstance(pair, list) and len(pair) == 2:
                    coords.append([float(pair[0]), float(pair[1])])
            givers.append(
                {
                    "id": row.get("id"),
                    "name": row.get("name"),
                    "type": row.get("type"),
                    "side": side,
                    "coords": coords,
                    "questNames": [
                        quest.get("name")
                        for quest in row.get("quests") or []
                        if isinstance(quest, dict) and quest.get("name")
                    ],
                }
            )
    return givers


def extract_page_listviews(html: str) -> list[dict[str, Any]]:
    views: list[dict[str, Any]] = []

    quest_rows = parse_quest_list(html)
    if quest_rows:
        views.append({"template": "quest", "id": "quests", "data": quest_rows})

    json_match = _LISTVIEWS_JSON_RE.search(html)
    if json_match:
        try:
            payload = json.loads(json_match.group(1))
        except json.JSONDecodeError:
            payload = []
        if isinstance(payload, list):
            for entry in payload:
                if isinstance(entry, dict) and isinstance(entry.get("data"), list):
                    views.append(entry)

    return views


def quest_id_from_url(url: str) -> int | None:
    match = _QUEST_ID_RE.search(url)
    if not match:
        return None
    return int(match.group(1))
