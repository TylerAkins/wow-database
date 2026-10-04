#!/usr/bin/env python3
"""Tests for deterministic per-zone compilation."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TOOLS = ROOT / "tools"
sys.path.insert(0, str(TOOLS))

from quest_db.compile_zones import (
    ZoneCompileError,
    build_zone_bundles,
    check_zone_bundles,
    write_zone_bundles,
)
from quest_db.sources import SOURCE_PAGES, SourcePage


class CompileZoneTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.raw_root = Path(self.temp_dir.name) / "raw"
        self.pages = (
            SourcePage("https://example.test/durotar", "zone", "kd/durotar"),
            SourcePage("https://example.test/elwynn", "zone", "ek/elwynn-forest"),
        )
        self._write_fixture(self.pages[0], [(10, 14), (2, 14)])
        self._write_fixture(self.pages[1], [(3, 12)])

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def test_compiles_keyed_lossless_records_in_numeric_order(self) -> None:
        rendered = build_zone_bundles(
            self.raw_root,
            "forever",
            ["durotar"],
            source_pages=self.pages,
        )

        self.assertEqual(["durotar.json"], list(rendered))
        bundle = json.loads(rendered["durotar.json"])
        self.assertEqual(1, bundle["schemaVersion"])
        self.assertEqual("forever", bundle["gameVersion"])
        self.assertEqual("kd/durotar", bundle["zone"]["sourceSlug"])
        self.assertEqual(14, bundle["zone"]["zoneId"])
        self.assertEqual(["2", "10"], list(bundle["quests"]))
        self.assertEqual("index-2", bundle["quests"]["2"]["index"]["marker"])
        self.assertEqual("detail-2", bundle["quests"]["2"]["detail"]["marker"])
        self.assertLess(rendered["durotar.json"].index('"2":'), rendered["durotar.json"].index('"10":'))

    def test_zone_selection_rejects_unknown_slug(self) -> None:
        with self.assertRaisesRegex(ZoneCompileError, "Unknown zone slug"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                ["missing"],
                source_pages=self.pages,
            )

    def test_rejects_missing_detail(self) -> None:
        (self.raw_root / "details" / "2.json").unlink()

        with self.assertRaisesRegex(ZoneCompileError, "Missing quest detail 2"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                ["durotar"],
                source_pages=self.pages,
            )

    def test_rejects_malformed_quest_id(self) -> None:
        path = self.raw_root / "sources" / "kd_durotar.json"
        source = json.loads(path.read_text(encoding="utf-8"))
        source["quests"][0]["id"] = "10"
        self._write(path, source)

        with self.assertRaisesRegex(ZoneCompileError, "malformed quest id"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                ["durotar"],
                source_pages=self.pages,
            )

    def test_rejects_conflicting_zone_ids_and_count_mismatches(self) -> None:
        path = self.raw_root / "sources" / "kd_durotar.json"
        source = json.loads(path.read_text(encoding="utf-8"))
        source["quests"][1]["category"] = 12
        self._write(path, source)
        with self.assertRaisesRegex(ZoneCompileError, "expected one zone id"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                ["durotar"],
                source_pages=self.pages,
            )

        source["questCount"] = 99
        self._write(path, source)
        with self.assertRaisesRegex(ZoneCompileError, "questCount"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                ["durotar"],
                source_pages=self.pages,
            )

    def test_rejects_duplicate_output_names(self) -> None:
        duplicate_pages = self.pages + (
            SourcePage("https://example.test/other", "zone", "other/durotar"),
        )

        with self.assertRaisesRegex(ZoneCompileError, "Duplicate zone output name"):
            build_zone_bundles(
                self.raw_root,
                "forever",
                source_pages=duplicate_pages,
            )

    def test_write_and_check_support_full_and_targeted_runs(self) -> None:
        output = Path(self.temp_dir.name) / "compiled"
        all_rendered = build_zone_bundles(
            self.raw_root,
            "forever",
            source_pages=self.pages,
        )
        write_zone_bundles(output, all_rendered, replace_all=True)
        self.assertEqual([], check_zone_bundles(output, all_rendered, check_all=True))

        (output / "durotar.json").write_text("stale\n", encoding="utf-8")
        selected = build_zone_bundles(
            self.raw_root,
            "forever",
            ["durotar"],
            source_pages=self.pages,
        )
        self.assertEqual(
            ["compiled file differs: durotar.json"],
            check_zone_bundles(output, selected, check_all=False),
        )
        write_zone_bundles(output, selected, replace_all=False)
        self.assertTrue((output / "elwynn-forest.json").is_file())
        self.assertEqual([], check_zone_bundles(output, all_rendered, check_all=True))

    def _write_fixture(self, page: SourcePage, rows: list[tuple[int, int]]) -> None:
        source_rows = [{"id": quest_id, "category": zone_id} for quest_id, zone_id in rows]
        self._write(
            self.raw_root / "sources" / f"{page.slug.replace('/', '_')}.json",
            {
                "fetchedAt": "2026-09-24T00:00:00+00:00",
                "kind": "zone",
                "questCount": len(source_rows),
                "quests": source_rows,
                "slug": page.slug,
                "url": page.url,
            },
        )
        index_path = self.raw_root / "quest_index.json"
        index = json.loads(index_path.read_text(encoding="utf-8")) if index_path.is_file() else {}
        for quest_id, _ in rows:
            index[str(quest_id)] = {"id": quest_id, "marker": f"index-{quest_id}"}
            self._write(
                self.raw_root / "details" / f"{quest_id}.json",
                {"questId": quest_id, "marker": f"detail-{quest_id}"},
            )
        self._write(index_path, index)

    @staticmethod
    def _write(path: Path, value: object) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


class ForeverSnapshotTests(unittest.TestCase):
    def test_snapshot_counts_and_all_zone_details(self) -> None:
        raw_root = ROOT / "data" / "forever" / "raw"
        quest_index = json.loads((raw_root / "quest_index.json").read_text(encoding="utf-8"))

        self.assertEqual(5058, len(quest_index))
        self.assertEqual(5058, len(list((raw_root / "details").glob("*.json"))))
        self.assertEqual(330, len(list((raw_root / "object").glob("*.json"))))
        self.assertEqual(1, len(list((raw_root / "item").glob("*.json"))))
        self.assertTrue((raw_root / "details" / "7507.json").is_file())

        rendered = build_zone_bundles(raw_root, "forever")
        self.assertEqual(59, len(rendered))
        durotar = json.loads(rendered["durotar.json"])
        self.assertEqual(65, durotar["questCount"])
        self.assertIn("96875", durotar["quests"])
        self.assertEqual("Orgrimmar", durotar["quests"]["96875"]["detail"]["startPins"][0]["zoneName"])

        expected_zone_names = {
            page.slug.rsplit("/", 1)[-1]
            for page in SOURCE_PAGES
            if page.kind == "zone"
        }
        self.assertEqual(expected_zone_names, {Path(name).stem for name in rendered})


if __name__ == "__main__":
    unittest.main()
