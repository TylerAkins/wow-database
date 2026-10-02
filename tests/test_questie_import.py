"""Behavioral checks for the pinned QuestieDB overlay."""

from __future__ import annotations

import sys
import unittest
import csv
import json
from io import StringIO
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "tools"))

from quest_db.questie_import import apply_questie_record, build_collections
from quest_db.ingest_merge import merge_quest_list_rows
from quest_db.sources import SourcePage
from import_questie import render_report


class QuestieImportTests(unittest.TestCase):
    def setUp(self) -> None:
        self.index = {"id": 92685, "name": "The Hills Have Eyes", "minLevel": 4,
                      "list": {"firstseenpatch": 16001, "level": 5, "reqlevel": 4},
                      "zoneId": 16593, "classes": [], "races": []}
        self.detail = {"questId": 92685, "prerequisiteQuestIds": [],
                       "infoboxMarkup": "original", "startPins": []}

    def test_group_prerequisites_remain_all_required(self) -> None:
        row = {"fields": {"requiredLevel": 5, "preQuestGroup": [92682, 92683, 92684],
                          "nextQuestInChain": 92693}, "provenance": {}}
        index, detail, changes = apply_questie_record(self.index, self.detail, row, "abc")
        self.assertEqual([92682, 92683, 92684], detail["prerequisiteQuestIds"])
        self.assertEqual([92682, 92683, 92684], detail["requirements"]["allOf"])
        self.assertEqual(92693, detail["requirements"]["nextQuestInChain"])
        self.assertEqual(5, index["minLevel"])
        self.assertEqual(4, index["list"]["reqlevel"])
        self.assertEqual("original", detail["infoboxMarkup"])
        self.assertTrue(changes)

    def test_alternative_prerequisites_are_not_flattened(self) -> None:
        row = {"fields": {"preQuestSingle": [92514, 93461]}, "provenance": {}}
        _, detail, _ = apply_questie_record(self.index, self.detail, row, "abc")
        self.assertEqual([92514, 93461], detail["requirements"]["anyOf"])
        self.assertEqual([], detail["prerequisiteQuestIds"])

    def test_zero_defaults_do_not_erase_known_values(self) -> None:
        row = {"fields": {"requiredLevel": 0, "questLevel": 0,
                          "requiredRaces": 0, "requiredClasses": 0}, "provenance": {}}
        index, _, _ = apply_questie_record(self.index, self.detail, row, "abc")
        self.assertEqual(4, index["minLevel"])
        self.assertEqual([], index["classes"])
        self.assertEqual([], index["races"])

    def test_skyborne_and_class_masks_map_to_ids(self) -> None:
        row = {"fields": {"requiredRaces": 4294967296, "requiredClasses": 64},
               "provenance": {}}
        index, _, _ = apply_questie_record(self.index, self.detail, row, "abc")
        self.assertEqual([95], index["races"])
        self.assertEqual([7], index["classes"])
        self.assertEqual("Alliance", index["faction"])

    def test_reapplying_same_record_is_idempotent(self) -> None:
        row = {"fields": {"requiredLevel": 5, "preQuestSingle": [92679]}, "provenance": {}}
        first_index, first_detail, _ = apply_questie_record(self.index, self.detail, row, "abc")
        second_index, second_detail, changes = apply_questie_record(first_index, first_detail, row, "abc")
        self.assertEqual(first_index, second_index)
        self.assertEqual(first_detail, second_detail)
        self.assertEqual([], changes)

    def test_collections_select_zephras_and_patch_without_duplicates(self) -> None:
        index = {"1": self.index, "2": {"id": 2, "zoneId": 12,
                  "list": {"firstseenpatch": 16001}}}
        details = {"1": self.detail, "2": {"questId": 2}}
        collections = build_collections(index, details)
        self.assertEqual(["1"], list(collections["zephras-isle"]["quests"]))
        self.assertEqual(["1", "2"], list(collections["new-in-forever"]["quests"]))

    def test_collection_rejects_missing_detail(self) -> None:
        with self.assertRaisesRegex(ValueError, "missing detail records"):
            build_collections({"92685": self.index}, {})

    def test_report_has_one_row_per_changed_field_with_record_path(self) -> None:
        row = {"fields": {"requiredLevel": 5}, "provenance": {}}
        index, _, changes = apply_questie_record(self.index, self.detail, row, "abc")
        for change in changes:
            change["questId"] = 92685
        csv_text, _ = render_report(changes, {"commit": "abc", "targetCount": 1,
                                            "quests": {"92685": row}, "missingQuestIds": []},
                                    {"92685": index})
        rows = list(csv.DictReader(StringIO(csv_text)))
        self.assertEqual(len(changes), len(rows))
        self.assertEqual({change["path"] for change in changes},
                         {report["field"] for report in rows})

    def test_wowhead_list_refresh_reapplies_questie_fields(self) -> None:
        from tempfile import TemporaryDirectory

        with TemporaryDirectory() as temp:
            raw = Path(temp) / "raw"
            raw.mkdir()
            overlay = raw.parent / "questie" / "quests.json"
            overlay.parent.mkdir()
            overlay.write_text(json.dumps({"commit": "abc", "quests": {
                "92685": {"fields": {"requiredLevel": 5}, "provenance": {}}
            }}), encoding="utf-8")
            page = SourcePage("https://example.test/quests", "zone", "test/zone")
            merge_quest_list_rows(raw, [{"id": 92685, "name": "The Hills Have Eyes",
                                        "category": 16593, "reqlevel": 4}], page)
            entry = json.loads((raw / "quest_index.json").read_text(encoding="utf-8"))["92685"]
            self.assertEqual(5, entry["minLevel"])
            self.assertEqual(4, entry["list"]["reqlevel"])


class QuestieSnapshotTests(unittest.TestCase):
    def test_committed_collections_and_log_match_snapshot(self) -> None:
        root = Path(__file__).resolve().parents[1]
        data = root / "data" / "forever"
        snapshot = json.loads((data / "questie" / "quests.json").read_text(encoding="utf-8"))
        self.assertEqual(820, snapshot["targetCount"])
        self.assertEqual(820, len(snapshot["quests"]) + len(snapshot["missingQuestIds"]))
        collections = data / "compiled" / "collections"
        zephras = json.loads((collections / "zephras-isle.json").read_text(encoding="utf-8"))
        forever = json.loads((collections / "new-in-forever.json").read_text(encoding="utf-8"))
        self.assertEqual(114, zephras["questCount"])
        self.assertEqual(820, forever["questCount"])
        self.assertEqual([92682, 92683, 92684],
                         zephras["quests"]["92685"]["detail"]["requirements"]["allOf"])
        self.assertEqual([92642, 92645],
                         zephras["quests"]["92880"]["detail"]["requirements"]["allOf"])
        with (data / "reports" / "questie-forever-changes.csv").open(
            encoding="utf-8", newline=""
        ) as handle:
            report = list(csv.DictReader(handle))
        summary = (data / "reports" / "questie-forever-summary.md").read_text(encoding="utf-8")
        self.assertIn(f"Changed fields: {len(report)}", summary)
        self.assertEqual(len(report), len({(row["questId"], row["field"]) for row in report}))


if __name__ == "__main__":
    unittest.main()
