"""Apply a pinned, composed QuestieDB snapshot to Forever quest records."""

from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any

RACE_BITS = {1: 1, 2: 2, 4: 3, 8: 4, 16: 5, 32: 6, 64: 7, 128: 8,
             256: 9, 4294967296: 95, 8589934592: 96}
CLASS_BITS = {1: 1, 2: 2, 4: 3, 8: 4, 16: 5, 64: 7, 128: 8,
              256: 9, 1024: 11}
REQUIREMENT_FIELDS = (
    "preQuestSingle", "preQuestGroup", "nextQuestInChain", "childQuests",
    "inGroupWith", "exclusiveTo", "parentQuest", "breadcrumbForQuestId",
    "breadcrumbs", "availableUntilCompleted", "availableStartingWith",
    "disabledByQuest", "requiredSkill", "requiredMinRep", "requiredMaxRep",
    "requiredSpell", "requiredSpecialization", "requiredMaxLevel", "requiredRanks",
)
ATT_FIELDS = {
    "preQuestSingle": "sourceQuest or sourceQuests + sourceQuestNumRequired=1",
    "preQuestGroup": "sourceQuests (AND)",
    "requiredLevel": "lvl",
    "requiredRaces": "races",
    "requiredClasses": "classes",
    "startedBy": "qg/provider; coord",
    "starters": "qg/provider; coord",
    "nextQuestInChain": "sourceQuest on successor",
}


def _ids_from_mask(mask: int, bits: dict[int, int], label: str) -> list[int]:
    unknown = mask & ~sum(bits)
    if unknown:
        raise ValueError(f"unknown {label} mask bits: {unknown}")
    return [value for bit, value in bits.items() if mask & bit]


def apply_questie_record(
    index: dict[str, Any], detail: dict[str, Any], row: dict[str, Any], commit: str,
) -> tuple[dict[str, Any], dict[str, Any], list[dict[str, Any]]]:
    """Return copied, enriched records and every effective field change."""
    index = copy.deepcopy(index)
    detail = copy.deepcopy(detail)
    fields = row.get("fields") or {}
    provenance = row.get("provenance") or {}
    changes: list[dict[str, Any]] = []

    def set_value(record: dict[str, Any], path: str, value: Any, source: str) -> None:
        keys = path.split(".")
        target = record
        for key in keys[:-1]:
            target = target.setdefault(key, {})
        old = target.get(keys[-1])
        if old == value:
            return
        target[keys[-1]] = copy.deepcopy(value)
        record_path = ("index." if record is index else "detail.") + path
        changes.append({"path": record_path, "before": old, "after": value,
                        "sourceField": source, "provenance": provenance.get(source, "QuestieDB"),
                        "attField": ATT_FIELDS.get(source, ""), "questieCommit": commit})

    for field, value in sorted(fields.items()):
        set_value(detail, f"questie.fields.{field}", value, field)
    for field, owner in sorted(provenance.items()):
        set_value(detail, f"questie.provenance.{field}", owner, field)
    for field in ("starters", "finishers"):
        if row.get(field):
            set_value(detail, f"questie.{field}", row[field], field)
    set_value(detail, "questie.commit", commit, "commit")
    set_value(index, "questieCommit", commit, "commit")

    if fields.get("name"):
        set_value(index, "name", fields["name"], "name")
    if isinstance(fields.get("requiredLevel"), int) and fields["requiredLevel"] > 0:
        set_value(index, "minLevel", fields["requiredLevel"], "requiredLevel")
        set_value(detail, "minLevel", fields["requiredLevel"], "requiredLevel")
    if isinstance(fields.get("questLevel"), int) and fields["questLevel"] != 0:
        set_value(index, "questLevel", fields["questLevel"], "questLevel")
        set_value(detail, "questLevel", fields["questLevel"], "questLevel")
    for field, target, bits in (("requiredRaces", "races", RACE_BITS),
                                ("requiredClasses", "classes", CLASS_BITS)):
        mask = fields.get(field)
        if isinstance(mask, int) and mask > 0:
            values = _ids_from_mask(mask, bits, field)
            set_value(index, target, values, field)
            set_value(detail, target, values, field)
            if field == "requiredRaces":
                alliance = {1, 3, 4, 7, 95}
                horde = {2, 5, 6, 8, 9, 96}
                race_set = set(values)
                if race_set <= alliance:
                    set_value(index, "faction", "Alliance", field)
                    set_value(detail, "faction", "Alliance", field)
                elif race_set <= horde:
                    set_value(index, "faction", "Horde", field)
                    set_value(detail, "faction", "Horde", field)

    requirements = {field: fields[field] for field in REQUIREMENT_FIELDS
                    if field in fields and fields[field] not in (None, 0, [], {})}
    if requirements:
        if requirements.get("preQuestGroup"):
            requirements["allOf"] = requirements["preQuestGroup"]
        if requirements.get("preQuestSingle"):
            requirements["anyOf"] = requirements["preQuestSingle"]
        for field, value in sorted(requirements.items()):
            set_value(detail, f"requirements.{field}", value, field)
        single = requirements.get("preQuestSingle") or []
        group = requirements.get("preQuestGroup") or []
        if group and not single:
            set_value(detail, "prerequisiteQuestIds", group, "preQuestGroup")
        elif len(single) == 1 and not group:
            set_value(detail, "prerequisiteQuestIds", single, "preQuestSingle")
    return index, detail, changes


def build_collections(index: dict[str, Any], details: dict[str, Any]) -> dict[str, Any]:
    """Build sorted, complete collections from effective records."""
    result = {}
    for slug, predicate in (
        ("zephras-isle", lambda row: row.get("zoneId") == 16593),
        ("new-in-forever", lambda row: row.get("list", {}).get("firstseenpatch") == 16001),
    ):
        quest_ids = sorted((key for key, row in index.items() if predicate(row)), key=int)
        missing = [key for key in quest_ids if key not in details]
        if missing:
            raise ValueError(f"{slug}: missing detail records: {', '.join(missing)}")
        result[slug] = {"schemaVersion": 1, "gameVersion": "forever",
                        "collection": slug, "questCount": len(quest_ids),
                        "quests": {key: {"index": index[key], "detail": details[key]}
                                   for key in quest_ids}}
    return result


def json_cell(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def load_overlay(raw_root: Path) -> tuple[str, dict[str, Any]]:
    path = raw_root.parent / "questie" / "quests.json"
    if not path.is_file():
        return "", {}
    snapshot = json.loads(path.read_text(encoding="utf-8"))
    return snapshot["commit"], snapshot["quests"]


def render_collections(raw_root: Path) -> dict[str, str]:
    index = json.loads((raw_root / "quest_index.json").read_text(encoding="utf-8"))
    selected = {key: row for key, row in index.items()
                if row.get("zoneId") == 16593
                or row.get("list", {}).get("firstseenpatch") == 16001}
    details = {key: json.loads((raw_root / "details" / f"{key}.json").read_text(encoding="utf-8"))
               for key in selected}
    return {f"{slug}.json": json.dumps(bundle, indent=2, sort_keys=True,
                                      ensure_ascii=False) + "\n"
            for slug, bundle in build_collections(selected, details).items()}
