"""Infer Source zone id → UiMapID using the current ATT pin database."""

from __future__ import annotations

import re
from collections import Counter
from pathlib import Path
from typing import Any

_QUEST_LINE_RE = re.compile(
    r"\[(\d+)\]\s*=\s*\{[^}]*mapID=(\d+)",
)


def bootstrap_zone_ui_map_ids(
    quest_index: dict[str, Any],
    att_quests_lua: Path,
) -> dict[str, dict[str, Any]]:
    """Return zoneId -> {uiMapID, confidence, samples}."""
    att_by_quest: dict[int, int] = {}
    if att_quests_lua.is_file():
        for quest_id, map_id in _QUEST_LINE_RE.findall(att_quests_lua.read_text(encoding="utf-8")):
            att_by_quest[int(quest_id)] = int(map_id)

    votes: dict[int, Counter[int]] = {}
    for entry in quest_index.values():
        if not isinstance(entry, dict):
            continue
        try:
            quest_id = int(entry["id"])
            zone_id = int(entry.get("zoneId") or entry.get("category") or 0)
        except (KeyError, TypeError, ValueError):
            continue
        if zone_id <= 0:
            continue
        att_map = att_by_quest.get(quest_id)
        if att_map is None:
            continue
        votes.setdefault(zone_id, Counter())[att_map] += 1

    result: dict[str, dict[str, Any]] = {}
    for zone_id, counter in sorted(votes.items()):
        if not counter:
            continue
        ui_map_id, count = counter.most_common(1)[0]
        total = sum(counter.values())
        result[str(zone_id)] = {
            "uiMapID": ui_map_id,
            "confidence": round(count / total, 3),
            "samples": total,
        }
    return result
