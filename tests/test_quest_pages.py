#!/usr/bin/env python3
"""Tests for Source database parsers (offline fixtures)."""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TOOLS = ROOT / "tools"
sys.path.insert(0, str(TOOLS))

from quest_db.classify import classify_pin_category
from quest_db.ingest import ingest_html
from quest_db.parse_list import parse_quest_list
from quest_db.parse_page import (
    extract_g_mapper_spawns,
    extract_inline_listviews,
    extract_map_quest_givers,
    extract_page_listviews,
    listview_ids,
)
from quest_db.parse_quest import (
    eligibility_restrictions,
    extract_spawn_pins,
    extract_start_pins,
    parse_quest_detail,
    quest_ids_from_markup,
)

FIXTURES = ROOT / "tests" / "fixtures" / "quest-pages"


class ParseListTests(unittest.TestCase):
    def test_elwynn_listview_extracts_quest_rows(self) -> None:
        html = (FIXTURES / "elwynn-forest.html").read_text(encoding="utf-8")
        rows = parse_quest_list(html)
        self.assertGreater(len(rows), 10)
        ids = {row["id"] for row in rows}
        self.assertIn(6, ids)
        self.assertIn(16, ids)


class ParseQuestTests(unittest.TestCase):
    def test_quest_mapper_start_pin(self) -> None:
        html = (FIXTURES / "quest-6.html").read_text(encoding="utf-8")
        detail = parse_quest_detail(html, 6)
        pins = extract_start_pins(detail["mapper"])
        self.assertEqual(1, len(pins))
        self.assertEqual(823, pins[0]["npcId"])
        self.assertAlmostEqual(48.2, pins[0]["x"])
        self.assertAlmostEqual(42.8, pins[0]["y"])

    def test_spawn_pins_keep_every_coordinate(self) -> None:
        mapper = {
            "objectives": {
                "17": {
                    "zone": "The Barrens",
                    "levels": [
                        [
                            {
                                "id": 3238,
                                "name": "Chen's Empty Keg",
                                "coords": [[55.8, 20.0], [43.0, 32.5], [57.1, 27.4]],
                                "point": "start",
                            }
                        ]
                    ],
                }
            }
        }
        pins = extract_spawn_pins(mapper)
        self.assertEqual([(55.8, 20.0), (43.0, 32.5), (57.1, 27.4)], [(pin["x"], pin["y"]) for pin in pins])
        self.assertEqual([819], quest_ids_from_markup("[url=/forever/quest=819/chens-empty-keg]"))

    def test_repeatable_flag_from_infobox(self) -> None:
        html = (FIXTURES / "quest-16.html").read_text(encoding="utf-8")
        detail = parse_quest_detail(html, 16)
        self.assertTrue(detail["infoboxFlags"].get("repeatable"))


class ParsePageTests(unittest.TestCase):
    def test_zone_starts_quest_listview(self) -> None:
        html = """
        new Listview({template: 'item', id: 'starts-quest', data: [{"id":4926,"name":"Chen's Empty Keg","sourcemore":[{"n":"Chen's Empty Keg","t":2,"ti":3238,"z":17}]}]});
        """
        views = extract_inline_listviews(html)
        self.assertEqual("starts-quest", views[0]["id"])
        self.assertEqual(3238, views[0]["data"][0]["sourcemore"][0]["ti"])

    def test_zone_map_lists_quest_givers_by_name(self) -> None:
        html = """
        var mapShowObject = new ShowOnMap({"hordequests":[{"coords":[[62.2,38.4]],"name":"Brewmaster Drohn","type":1,"id":3292,"quests":[{"name":"Chen's Empty Keg"}]}],"alliancequests":[]});
        """
        givers = extract_map_quest_givers(html)
        self.assertEqual(3292, givers[0]["id"])
        self.assertEqual(["Chen's Empty Keg"], givers[0]["questNames"])
        self.assertEqual([[62.2, 38.4]], givers[0]["coords"])

    def test_object_page_spawns_and_item_quest_link(self) -> None:
        html = """
        <script>var g_mapperData = {"17":[{"count":2,"coords":[[55.8,20],[57.1,9]],"uiMapId":1413,"uiMapName":"The Barrens"}]};</script>
        new Listview({template: 'item', id: 'contains', data: [{"id":4926,"name":"Chen's Empty Keg"}]});
        new Listview({template: 'quest', id: 'starts', data: [{"id":819,"name":"Chen's Empty Keg"}]});
        """
        pins = extract_g_mapper_spawns(html)
        self.assertEqual(1413, pins[0]["uiMapId"])
        self.assertEqual([(55.8, 20.0), (57.1, 9.0)], [(pin["x"], pin["y"]) for pin in pins])
        self.assertEqual([4926], listview_ids(html, "contains"))
        self.assertEqual([819], listview_ids(html, "starts"))

    def test_objects_page_json_listview(self) -> None:
        html = (FIXTURES / "objects-quests.html").read_text(encoding="utf-8")
        views = extract_page_listviews(html)
        templates = {view.get("template") for view in views}
        self.assertIn("object", templates)
        object_rows = next(v for v in views if v.get("template") == "object")["data"]
        self.assertGreater(len(object_rows), 100)


class IngestTests(unittest.TestCase):
    def test_ingest_objects_page_writes_object_index(self) -> None:
        import shutil
        import tempfile

        html = (FIXTURES / "objects-quests.html").read_text(encoding="utf-8")
        tmp = Path(tempfile.mkdtemp())
        try:
            report = ingest_html(
                tmp,
                "https://www.wowhead.com/forever/objects/quests",
                html,
            )
            self.assertGreater(report["objectRows"], 100)
            index = json.loads((tmp / "object_index.json").read_text(encoding="utf-8"))
            self.assertGreater(len(index), 100)
        finally:
            shutil.rmtree(tmp)


class EligibilityTests(unittest.TestCase):
    def test_tauren_and_shaman_from_infobox(self) -> None:
        markup = (
            "[ul][li]Side: [span class=icon-horde]Horde[/span][/li]"
            "[li]Races: [race=2], [race=6], [br][race=8][/li]"
            "[li]Class: [class=7][/li][li]Requires level 20[/li][/ul]"
        )
        result = eligibility_restrictions(markup, {"reqclass": 0, "side": 2, "reqlevel": 20})
        self.assertEqual("Horde", result["faction"])
        self.assertEqual([2, 6, 8], result["races"])
        self.assertEqual([7], result["classes"])
        self.assertEqual(20, result["minLevel"])

    def test_class_bitmask_when_page_has_no_class_tag(self) -> None:
        result = eligibility_restrictions(
            "[ul][li]Side: [span class=icon-alliance]Alliance[/span][/li][/ul]",
            {"reqclass": 1024, "side": 1, "reqlevel": 16},
        )
        self.assertEqual([11], result["classes"])
        self.assertEqual([], result["races"])


class ClassifyTests(unittest.TestCase):
    def test_battleground_source_is_pvp(self) -> None:
        cat = classify_pin_category(
            source_kinds={"battleground"},
            list_row={"type": 41},
            detail_flags={},
        )
        self.assertEqual("pvp", cat)

    def test_dungeon_source_is_instance(self) -> None:
        cat = classify_pin_category(
            source_kinds={"dungeon"},
            list_row={},
            detail_flags={},
        )
        self.assertEqual("instance", cat)


if __name__ == "__main__":
    unittest.main()
