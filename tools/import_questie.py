#!/usr/bin/env python3
"""Export and apply a pinned QuestieDB Forever snapshot."""

from __future__ import annotations

import argparse
import csv
import json
import re
import subprocess
import sys
import tempfile
from collections import Counter
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))

from quest_db.questie_import import apply_questie_record, build_collections, json_cell
from quest_db.store import write_json

COMMIT = "9d39232dab48e35811a7cc02473c2f4e42b62ab6"
RAW = ROOT / "data" / "forever" / "raw"
SNAPSHOT = ROOT / "data" / "forever" / "questie" / "quests.json"
COLLECTIONS = ROOT / "data" / "forever" / "compiled" / "collections"
REPORTS = ROOT / "data" / "forever" / "reports"


def target_ids(index: dict[str, Any]) -> list[int]:
    return sorted(int(key) for key, row in index.items()
                  if row.get("list", {}).get("firstseenpatch") == 16001)


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def read_git_json(ref: str, path: str) -> Any:
    result = subprocess.run(["git", "show", f"{ref}:{path}"], cwd=ROOT,
                            capture_output=True, text=True, check=True)
    return json.loads(result.stdout)


def export_snapshot(questiedb: Path, ids: list[int], lua: str) -> dict[str, Any]:
    head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=questiedb,
                          capture_output=True, text=True, check=True).stdout.strip()
    if head != COMMIT:
        raise ValueError(f"QuestieDB checkout is {head}; required {COMMIT}")
    with tempfile.TemporaryDirectory(prefix="questie-ids-") as temp:
        ids_path = Path(temp) / "ids.txt"
        ids_path.write_text("".join(f"{qid}\n" for qid in ids), encoding="ascii")
        result = subprocess.run(
            [lua, str(ROOT / "tools" / "quest_db" / "export_questie.lua"), str(ids_path)],
            cwd=questiedb, capture_output=True, text=True, check=True,
        )
    rows = [json.loads(line) for line in result.stdout.splitlines()]
    if [row["id"] for row in rows] != ids:
        raise ValueError("QuestieDB exporter did not return every requested quest ID in order")
    fixes = questiedb / "src" / "corrections" / "Forever" / "foreverQuestFixes.lua"
    notes: list[dict[str, Any]] = []
    current_id = None
    for line in fixes.read_text(encoding="utf-8").splitlines():
        match = re.match(r"\s*\[(\d+)\]\s*=\s*\{", line)
        if match:
            current_id = int(match.group(1))
        if current_id in ids and "--" in line:
            comment = line.split("--", 1)[1].strip()
            if re.search(r"\b(todo|check|verify|not sure|needs?)\b", comment, re.I):
                notes.append({"questId": current_id, "note": comment})
    return {
        "schemaVersion": 1, "source": "Questie/QuestieDB", "commit": COMMIT,
        "flavor": "Forever", "locale": "enUS", "targetCount": len(ids),
        "numericZeroIsDefault": True,
        "quests": {str(row["id"]): row["row"] for row in rows if "row" in row},
        "missingQuestIds": [row["id"] for row in rows if row.get("missing")],
        "sourceNotes": notes,
    }


def render_report(changes: list[dict[str, Any]], snapshot: dict[str, Any],
                  index: dict[str, Any]) -> tuple[str, str]:
    from io import StringIO

    output = StringIO()
    writer = csv.writer(output, lineterminator="\n")
    writer.writerow(("questId", "name", "zoneId", "field", "before", "after",
                     "questieField", "questieProvenance", "questieCommit", "attField", "reviewNote"))
    note_ids = {note["questId"] for note in snapshot.get("sourceNotes", [])}
    for change in sorted(changes, key=lambda row: (row["questId"], row["path"])):
        qid = change["questId"]
        field = change["sourceField"]
        review = []
        if field == "preQuestSingle" and isinstance(change["after"], list) and len(change["after"]) > 1:
            review.append("alternative prerequisites (OR)")
        if field == "preQuestGroup" and isinstance(change["after"], list) and len(change["after"]) > 1:
            review.append("grouped prerequisites (AND)")
        if qid in note_ids:
            review.append("QuestieDB source has a verification comment")
        if change["before"] is not None and not change["path"].startswith("detail.questie."):
            review.append("conflicts with existing value")
        if not change["attField"]:
            review.append("no direct ATT field mapped")
        writer.writerow((qid, index[str(qid)].get("name", ""), index[str(qid)].get("zoneId", ""),
                         change["path"], json_cell(change["before"]), json_cell(change["after"]),
                         field, change["provenance"], change["questieCommit"], change["attField"],
                         "; ".join(review)))
    field_counts = Counter(change["sourceField"] for change in changes)
    conflicts = sum(1 for change in changes if change["before"] is not None
                    and not change["path"].startswith("detail.questie."))
    missing_zephras = [qid for qid in snapshot["missingQuestIds"]
                       if index[str(qid)].get("zoneId") == 16593]
    lines = [
        "# QuestieDB Forever import", "",
        f"Source: https://github.com/Questie/QuestieDB/tree/{snapshot['commit']}",
        f"Target quests: {snapshot['targetCount']}",
        f"QuestieDB records: {len(snapshot['quests'])}",
        f"Missing QuestieDB records: {len(snapshot['missingQuestIds'])}",
        f"Missing Zephras Isle QuestieDB records: {len(missing_zephras)}",
        f"Changed fields: {len(changes)}", f"Existing value conflicts: {conflicts}", "",
        "## Changes by QuestieDB field", "",
    ]
    lines.extend(f"- `{field}`: {count}" for field, count in sorted(field_counts.items()))
    lines.extend(("", "## Missing QuestieDB records", "",
                  ", ".join(map(str, snapshot["missingQuestIds"])) or "None", "",
                  "## QuestieDB verification comments", ""))
    lines.extend(f"- {note['questId']}: {note['note']}" for note in snapshot.get("sourceNotes", []))
    if not snapshot.get("sourceNotes"):
        lines.append("None")
    tomb_weed = snapshot["quests"].get("99142", {}).get("fields", {})
    if tomb_weed and not (tomb_weed.get("preQuestSingle") or tomb_weed.get("preQuestGroup")):
        lines.extend(("", "## Tomb Weed relationship", "",
                      "QuestieDB has no prerequisite field for 99142 Tomb Weed at this commit. "
                      "The proposed link to 5482 Doom Weed remains unverified.", ""))
    return output.getvalue(), "\n".join(lines).rstrip("\n") + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--questiedb", type=Path, help="Pinned QuestieDB checkout for fresh export")
    parser.add_argument("--lua", default="luajit", help="Lua 5.1 compatible executable")
    parser.add_argument("--dry-run", action="store_true", help="Report changes without writing")
    parser.add_argument("--report-from-ref", metavar="REF",
                        help="Rebuild the full change log against a pre-import Git revision")
    args = parser.parse_args(argv)

    index = read_json(RAW / "quest_index.json")
    current_ids = target_ids(index)
    snapshot = export_snapshot(args.questiedb, current_ids, args.lua) if args.questiedb else read_json(SNAPSHOT)
    snapshot_ids = set(map(int, snapshot["quests"])) | set(snapshot["missingQuestIds"])
    if snapshot.get("commit") != COMMIT or snapshot.get("targetCount") != len(snapshot_ids):
        raise ValueError("QuestieDB snapshot revision or internal target count is invalid")
    if args.questiedb and snapshot_ids != set(current_ids):
        raise ValueError("QuestieDB export does not cover the complete current patch set")
    absent_ids = snapshot_ids - {int(key) for key in index}
    if absent_ids:
        raise ValueError(f"QuestieDB snapshot IDs absent from quest index: {sorted(absent_ids)}")
    ids = sorted(snapshot_ids | set(current_ids) |
                 {int(key) for key, row in index.items() if row.get("zoneId") == 16593})

    details: dict[str, Any] = {}
    changes: list[dict[str, Any]] = []
    for qid in ids:
        key = str(qid)
        path = RAW / "details" / f"{qid}.json"
        if not path.is_file():
            raise ValueError(f"missing Forever detail record: {qid}")
        detail = read_json(path)
        row = snapshot["quests"].get(key)
        if row:
            index[key], detail, row_changes = apply_questie_record(index[key], detail, row, COMMIT)
            for change in row_changes:
                change["questId"] = qid
            changes.extend(row_changes)
        details[key] = detail
    if args.report_from_ref:
        baseline = read_git_json(args.report_from_ref, "data/forever/raw/quest_index.json")
        changes = []
        for qid in sorted(snapshot_ids):
            key = str(qid)
            row = snapshot["quests"].get(key)
            if not row:
                continue
            old_detail = read_git_json(args.report_from_ref, f"data/forever/raw/details/{qid}.json")
            expected_index, expected_detail, row_changes = apply_questie_record(
                baseline[key], old_detail, row, COMMIT,
            )
            if expected_index != index[key] or expected_detail != details[key]:
                raise ValueError(f"quest {qid}: current data differs from import of {args.report_from_ref}")
            for change in row_changes:
                change["questId"] = qid
            changes.extend(row_changes)
    # Collections need every Zephras entry, which is inside the selected patch set.
    collections = build_collections(index, details)
    report_csv, report_md = render_report(changes, snapshot, index)
    counts = {slug: collection["questCount"] for slug, collection in collections.items()}
    print(f"QuestieDB {COMMIT}: {len(snapshot_ids)} targets, {len(snapshot['missingQuestIds'])} missing, "
          f"{len(changes)} field changes, collections {counts}")
    if args.dry_run:
        return 0
    if args.questiedb:
        write_json(SNAPSHOT, snapshot)
    write_json(RAW / "quest_index.json", index)
    for key, detail in details.items():
        path = RAW / "details" / f"{key}.json"
        if read_json(path) != detail:
            write_json(path, detail)
    for slug, collection in collections.items():
        write_json(COLLECTIONS / f"{slug}.json", collection)
    if changes:
        REPORTS.mkdir(parents=True, exist_ok=True)
        (REPORTS / "questie-forever-changes.csv").write_text(report_csv, encoding="utf-8")
        (REPORTS / "questie-forever-summary.md").write_text(report_md, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
