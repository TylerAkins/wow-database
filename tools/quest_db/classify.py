"""Quest pin / guide categories and display colors."""

from __future__ import annotations

from typing import Any, Literal

PinCategory = Literal["normal", "repeatable", "instance", "attunement", "pvp", "raid"]

# Atlas tint RGB (0–1). PvP uses Horde-adjacent purple, distinct from repeat blue and attunement orange.
PIN_COLORS: dict[PinCategory, dict[str, Any]] = {
    "normal": {
        "label": "Normal",
        "atlas": "QuestNormal",
        "tint": None,
        "fallbackTga": "QuestAvailable.tga",
    },
    "repeatable": {
        "label": "Repeatable",
        "atlas": "QuestNormal",
        "tint": [0.12, 0.72, 1.00],
        "fallbackTga": "QuestRepeatable.tga",
    },
    "instance": {
        "label": "Dungeon / instance",
        "atlas": "QuestNormal",
        "tint": [1.00, 0.32, 0.08],
        "fallbackTga": "QuestAttunement.tga",
    },
    "attunement": {
        "label": "Attunement",
        "atlas": "QuestNormal",
        "tint": [1.00, 0.32, 0.08],
        "fallbackTga": "QuestAttunement.tga",
    },
    "pvp": {
        "label": "PvP / battleground",
        "atlas": "QuestNormal",
        "tint": [0.78, 0.22, 0.95],
        "fallbackTga": "QuestPvP.tga",
    },
    "raid": {
        "label": "Raid",
        "atlas": "QuestNormal",
        "tint": [1.00, 0.32, 0.08],
        "fallbackTga": "QuestAttunement.tga",
        "note": "Most Forever raids are not in-game yet; category reserved for Onyxia and future raids.",
    },
}


def classify_pin_category(
    *,
    source_kinds: set[str],
    list_row: dict[str, Any] | None,
    detail_flags: dict[str, bool] | None,
    attunement_ids: set[int] | None = None,
    quest_id: int | None = None,
) -> PinCategory:
    flags = detail_flags or {}
    row = list_row or {}

    if "battleground" in source_kinds or flags.get("pvp"):
        return "pvp"
    if quest_id is not None and attunement_ids and quest_id in attunement_ids:
        return "attunement"
    if "raid" in source_kinds or flags.get("raid"):
        return "raid"
    if "dungeon" in source_kinds or flags.get("dungeon"):
        return "instance"

    if flags.get("repeatable") or flags.get("daily") or flags.get("weekly") or flags.get("monthly"):
        return "repeatable"

    # Source list rows use quest type 41 for PvP/battleground quests in some indexes.
    try:
        if int(row.get("type", 0)) == 41:
            return "pvp"
    except (TypeError, ValueError):
        pass

    return "normal"
