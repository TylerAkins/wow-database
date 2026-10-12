import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools" / "wowhead"))

from import_snapshot import SnapshotError, collect_snapshot, import_snapshot  # noqa: E402


def write_json(path: Path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, sort_keys=True), encoding="utf-8")


class WowheadImportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name) / "raw"
        self.root.mkdir()
        write_json(self.root / "manifest.json", {
            "schemaVersion": 1, "product": "forever-quests", "dataEnv": 16,
            "lastFullSyncCompleted": "2026-09-25T11:33:22+00:00",
            "stats": {"questIndexCount": 2, "questDetailCount": 1, "questDetailFetchErrors": 1, "objectIndexCount": 1, "objectDetailCount": 1, "itemDetailCount": 1},
        })
        write_json(self.root / "quest_index.json", {
            "1": {"id": 1, "name": "Zero and empty", "minLevel": 0, "races": [], "classes": [], "list": {"level": 0, "money": 0}, "sourceSlugs": ["zone/one"]},
            "2": {"id": 2, "name": "No detail", "minLevel": 1, "races": [], "classes": [], "list": {"level": 2}},
        })
        write_json(self.root / "details" / "1.json", {"questId": 1, "fetchedAt": "volatile-a", "pageInfo": {"name": "Zero and empty"}, "prerequisiteQuestIds": [], "startPins": [], "mapper": None})
        write_json(self.root / "object_index.json", {"10": {"id": 10, "name": "Object", "locations": [], "sourceSlugs": ["objects/quests"]}})
        write_json(self.root / "object" / "10.json", {"id": 10, "kind": "object", "name": "Object", "fetchedAt": "volatile-object", "spawns": [], "startsQuestIds": [1], "containedItemIds": []})
        write_json(self.root / "item" / "20.json", {"id": 20, "kind": "item", "name": None, "fetchedAt": "volatile-item", "spawns": [], "startsQuestIds": []})
        write_json(self.root / "zone_ui_map_ids.json", {"100": {"uiMapID": 200, "confidence": 0, "samples": 0}})

    def tearDown(self):
        self.temp.cleanup()

    def test_import_preserves_zero_empty_and_unavailable_states(self):
        rows, manifest = collect_snapshot(self.root)
        quest = next(row for row in rows if row["entityType"] == "quest" and row["entityId"] == "1")
        self.assertEqual(quest["attributes"]["requiredLevel"], 0)
        self.assertEqual(quest["fieldStates"]["races"], "empty")
        self.assertEqual(quest["fieldStates"]["prerequisiteReferences"], "empty")
        missing_detail = next(row for row in rows if row["entityType"] == "quest" and row["entityId"] == "2")
        self.assertEqual(missing_detail["fieldStates"]["detail"], "unavailable")
        item = next(row for row in rows if row["entityType"] == "item")
        self.assertEqual(item["fieldStates"]["name"], "unavailable")
        self.assertIsNone(item["attributes"]["wowheadDetail"]["name"])
        self.assertEqual(manifest["status"], "partial")
        self.assertEqual(len(rows), 6)

    def test_fetch_timestamps_do_not_change_semantic_revision_or_records(self):
        before_rows, before_manifest = collect_snapshot(self.root)
        raw = json.loads((self.root / "details" / "1.json").read_text())
        raw["fetchedAt"] = "different timestamp"
        write_json(self.root / "details" / "1.json", raw)
        after_rows, after_manifest = collect_snapshot(self.root)
        self.assertEqual(before_manifest["sourceRevision"], after_manifest["sourceRevision"])
        self.assertEqual(before_rows, after_rows)

    def test_detail_fallback_name_uses_detail_provenance(self):
        index_path = self.root / "quest_index.json"
        index = json.loads(index_path.read_text())
        index["1"].pop("name")
        write_json(index_path, index)
        rows, _ = collect_snapshot(self.root)
        quest = next(row for row in rows if row["entityType"] == "quest" and row["entityId"] == "1")
        self.assertEqual(quest["attributes"]["name"], "Zero and empty")
        self.assertEqual(quest["provenance"]["name"]["path"], "details/1.json")

    def test_snapshot_count_mismatch_fails_before_output_replacement(self):
        out = Path(self.temp.name) / "canonical"
        import_snapshot(self.root, out)
        before = (out / "records.jsonl").read_bytes()
        path = self.root / "manifest.json"
        manifest = json.loads(path.read_text())
        manifest["stats"]["questIndexCount"] = 1
        write_json(path, manifest)
        with self.assertRaises(SnapshotError):
            import_snapshot(self.root, out)
        self.assertEqual(before, (out / "records.jsonl").read_bytes())


if __name__ == "__main__":
    unittest.main()
