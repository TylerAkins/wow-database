"""Parse Source Forever quest detail pages."""

from __future__ import annotations

import json
import re
from typing import Any

_INFobox_RE = re.compile(
    r'WH\.markup\.printHtml\("(\[.*?\])",\s*"infobox-contents-0"',
    re.DOTALL,
)
_mapper_re = re.compile(r"var myMapper = new Mapper\((\{.*?\})\);", re.DOTALL)
_gatherer_quest_re = re.compile(
    r'WH\.Gatherer\.addData\(5,\s*16,\s*(\{.*?\})\);',
    re.DOTALL,
)
_PAGE_INFO_RE = re.compile(r"var g_pageInfo = (\{.*?\});")
_RACE_RE = re.compile(r"\[race=(\d+)\]")
_CLASS_RE = re.compile(r"\[class=(\d+)\]")
_REQ_LEVEL_RE = re.compile(r"Requires level (\d+)", re.IGNORECASE)

# Source list `reqclass` is a bitmask. Values are Blizzard class IDs.
_CLASS_BIT_TO_ID = {
    1: 1,
    2: 2,
    4: 3,
    8: 4,
    16: 5,
    64: 7,
    128: 8,
    256: 9,
    1024: 11,
}


def parse_quest_detail(html: str, quest_id: int) -> dict[str, Any]:
    result: dict[str, Any] = {
        "questId": quest_id,
        "infoboxMarkup": None,
        "infoboxFlags": {},
        "mapper": None,
        "prerequisiteQuestIds": [],
        "pageInfo": None,
    }

    page_match = _PAGE_INFO_RE.search(html)
    if page_match:
        try:
            result["pageInfo"] = json.loads(page_match.group(1))
        except json.JSONDecodeError:
            pass

    infobox_match = _INFobox_RE.search(html)
    if infobox_match:
        raw = infobox_match.group(1)
        result["infoboxMarkup"] = _unescape_wh_markup(raw)
        result["infoboxFlags"] = _flags_from_infobox(result["infoboxMarkup"])

    mapper_match = _mapper_re.search(html)
    if mapper_match:
        try:
            result["mapper"] = json.loads(mapper_match.group(1))
        except json.JSONDecodeError:
            result["mapper"] = None

    prereq_ids: set[int] = set()
    for block in _gatherer_quest_re.findall(html):
        try:
            table = json.loads(block)
        except json.JSONDecodeError:
            continue
        for key, value in table.items():
            try:
                qid = int(key)
            except (TypeError, ValueError):
                continue
            if qid != quest_id:
                prereq_ids.add(qid)
    result["prerequisiteQuestIds"] = sorted(prereq_ids)

    return result


_QUEST_LINK_RE = re.compile(r"quest=(\d+)")


def extract_spawn_pins(mapper: dict[str, Any] | None) -> list[dict[str, Any]]:
    """Every map coordinate on a page, including multi-spawn objects."""
    if not mapper:
        return []
    objectives = mapper.get("objectives") or {}
    pins: list[dict[str, Any]] = []
    for zone_id, zone_block in objectives.items():
        if not isinstance(zone_block, dict):
            continue
        levels = zone_block.get("levels") or []
        for level in levels:
            if not isinstance(level, list):
                continue
            for entry in level:
                if not isinstance(entry, dict):
                    continue
                for x_coord, y_coord in _entry_coordinates(entry):
                    pins.append(
                        {
                            "zoneId": int(zone_id) if str(zone_id).isdigit() else zone_id,
                            "zoneName": zone_block.get("zone"),
                            "x": x_coord,
                            "y": y_coord,
                            "npcId": entry.get("id"),
                            "npcName": entry.get("name"),
                            "point": entry.get("point"),
                        }
                    )
    return pins


def quest_ids_from_markup(markup: str | None) -> list[int]:
    if not markup:
        return []
    return sorted({int(match) for match in _QUEST_LINK_RE.findall(markup)})


def _entry_coordinates(entry: dict[str, Any]) -> list[tuple[float, float]]:
    coords = entry.get("coords")
    pairs: list[tuple[float, float]] = []
    if isinstance(coords, list):
        for pair in coords:
            if isinstance(pair, list) and len(pair) == 2:
                pairs.append((float(pair[0]), float(pair[1])))
    if pairs:
        return pairs
    coord = entry.get("coord")
    if isinstance(coord, list) and len(coord) == 2:
        return [(float(coord[0]), float(coord[1]))]
    return []


def extract_start_pins(mapper: dict[str, Any] | None) -> list[dict[str, Any]]:
    if not mapper:
        return []
    objectives = mapper.get("objectives") or {}
    pins: list[dict[str, Any]] = []
    for zone_id, zone_block in objectives.items():
        if not isinstance(zone_block, dict):
            continue
        levels = zone_block.get("levels") or []
        for level in levels:
            if not isinstance(level, list):
                continue
            for entry in level:
                if not isinstance(entry, dict):
                    continue
                if entry.get("point") != "start":
                    continue
                coord = entry.get("coord")
                if not (isinstance(coord, list) and len(coord) == 2):
                    continue
                pins.append(
                    {
                        "zoneId": int(zone_id) if str(zone_id).isdigit() else zone_id,
                        "zoneName": zone_block.get("zone"),
                        "x": float(coord[0]),
                        "y": float(coord[1]),
                        "npcId": entry.get("id"),
                        "npcName": entry.get("name"),
                    }
                )
    return pins


def classes_from_bitmask(mask: int) -> list[int]:
    if mask <= 0:
        return []
    return [class_id for bit, class_id in _CLASS_BIT_TO_ID.items() if mask & bit]


def eligibility_restrictions(markup: str | None, list_row: dict[str, Any] | None = None) -> dict[str, Any]:
    """Fields Eligibility.lua already checks: faction, races, classes, minLevel.

    Empty races or classes means no restriction. faction is omitted when both sides can take it.
    Race ids are Source [race=N] tags, which match Blizzard race ids (6 = Tauren).
    Class ids prefer [class=N] on the quest page, then the list bitmask.
    """
    text = markup or ""
    races = _unique_ints(_RACE_RE.findall(text))
    classes = _unique_ints(_CLASS_RE.findall(text))
    row = list_row or {}
    if not classes:
        try:
            classes = classes_from_bitmask(int(row.get("reqclass") or 0))
        except (TypeError, ValueError):
            classes = []

    faction: str | None = None
    if "icon-alliance" in text:
        faction = "Alliance"
    elif "icon-horde" in text:
        faction = "Horde"
    else:
        side = row.get("side")
        if side == 1:
            faction = "Alliance"
        elif side == 2:
            faction = "Horde"

    min_level: int | None = None
    level_match = _REQ_LEVEL_RE.search(text)
    if level_match:
        min_level = int(level_match.group(1))
    else:
        try:
            raw_level = int(row.get("reqlevel") or 0)
        except (TypeError, ValueError):
            raw_level = 0
        if raw_level > 0:
            min_level = raw_level

    result: dict[str, Any] = {
        "races": races,
        "classes": classes,
    }
    if faction:
        result["faction"] = faction
    if min_level is not None:
        result["minLevel"] = min_level
    return result


def _unique_ints(values: list[str]) -> list[int]:
    seen: set[int] = set()
    ordered: list[int] = []
    for value in values:
        number = int(value)
        if number in seen:
            continue
        seen.add(number)
        ordered.append(number)
    return ordered


def _unescape_wh_markup(raw: str) -> str:
    return (
        raw.replace(r"\/", "/")
        .replace(r"\[", "[")
        .replace(r"\]", "]")
        .replace(r"\"", '"')
    )


def _flags_from_infobox(markup: str | None) -> dict[str, bool]:
    if not markup:
        return {}
    text = markup.lower()
    return {
        "repeatable": "repeatable" in text,
        "daily": "daily" in text,
        "weekly": "weekly" in text,
        "monthly": "monthly" in text,
        "pvp": "pvp" in text or "battleground" in text,
        "dungeon": "dungeon" in text,
        "raid": "raid" in text,
        "sharable": "sharable" in text,
    }
