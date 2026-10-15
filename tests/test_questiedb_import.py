import gzip
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools" / "questiedb"))

from import_source import ImportErrorMessage, import_source, parse_entity_file, structured_json_value  # noqa: E402
from lua_literal import LuaLiteralError, parse  # noqa: E402


class LuaLiteralTests(unittest.TestCase):
    def test_parses_nested_lua_literals_and_nil_positions(self):
        self.assertEqual(
            parse('return {[42]={"Example",nil,{{1,2}}}, [43]={false,0}}'),
            {42: ["Example", None, [[1, 2]]], 43: [False, 0]},
        )

    def test_expression_is_recorded_without_execution(self):
        marker = Path(tempfile.gettempdir()) / "questiedb-lua-must-not-execute"
        marker.unlink(missing_ok=True)
        parsed = parse(f'return os.execute("touch {marker}")')
        self.assertEqual(parsed["$luaExpression"], "os.execute")
        self.assertFalse(marker.exists())

    def test_preserves_symbolic_negation_without_evaluation(self):
        self.assertEqual(
            parse("return {-rankKeys.ARTISAN}", preserve_expressions=True),
            [{"$luaExpression": "-", "arguments": [{"$luaExpression": "rankKeys.ARTISAN"}]}],
        )

    def test_rejects_arithmetic_instead_of_evaluating_it(self):
        with self.assertRaises(LuaLiteralError):
            parse("return { [1] = 1 + 2 }")


class StructuredLuaTableTests(unittest.TestCase):
    def test_schema_preserves_sparse_positional_tables_and_zone_maps(self):
        self.assertEqual(structured_json_value({"3": [[750]]}, "objectives"), [None, None, [[750]]])
        self.assertEqual(structured_json_value({"2": [123]}, "questgivers"), [None, [123]])
        self.assertEqual(structured_json_value([[[10, 20]]], "spawnlist"), {"1": [[10, 20]]})
        self.assertEqual(structured_json_value({"7": [[[1, 2], {"1": 3, "3": 4}]]}, "waypointlist"), {"7": [[[1, 2], [3, None, 4]]]})
        self.assertEqual(structured_json_value({"2": [[[10, 20]]]}, "trigger"), [None, {"1": [[10, 20]]}])
        self.assertEqual(structured_json_value({"2": 99}, "pair"), [None, 99])

    def test_zone_maps_omit_serialized_nil_holes(self):
        expected = {"3": [[10, 20, 7]]}
        self.assertEqual(structured_json_value({"1": None, "2": None, "3": [[10, 20, 7]]}, "spawnlist"), expected)
        self.assertEqual(structured_json_value([None, None, [[10, 20, 7]]], "spawnlist"), expected)
        self.assertEqual(structured_json_value({"1": None, "3": [[[10, 20, 7]]]}, "waypointlist"), {"3": [[[10, 20, 7]]]})

    def test_new_upstream_structures_are_not_silently_guessed(self):
        with self.assertRaisesRegex(ImportErrorMessage, "unsupported QuestieDB structure"):
            structured_json_value([], "futureStructure")
        with self.assertRaisesRegex(ImportErrorMessage, "non-positional key"):
            structured_json_value({"named": 1}, "pair")


class QuestieImporterTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.repo = Path(self.temp.name) / "QuestieDB"
        (self.repo / "data" / "Classic").mkdir(parents=True)
        (self.repo / "data" / "Forever").mkdir(parents=True)
        for flavor, folder in (("era", "Classic"), ("forever", "Forever")):
            prefix = folder.lower()
            for kind, key_name in (("Quest", "quest"), ("Npc", "npc"), ("Item", "item"), ("Object", "object")):
                body = (
                    f"QuestieDB.{key_name}Keys = {{ ['name'] = 1, ['level'] = 2, ['emptyList'] = 3 }}\n"
                    f"QuestieDB.{key_name}Data = [[return {{ [42] = {{\"{folder} {kind}\", 0, {{}}}}, [43] = {{nil, nil, nil}} }}]]\n"
                )
                (self.repo / "data" / folder / f"{prefix}{kind}DB.lua").write_text(body, encoding="utf-8")
        corrections = self.repo / "src" / "corrections" / "Forever"
        corrections.mkdir(parents=True)
        (corrections / "foreverQuestFixes.lua").write_text(
            "function ForeverQuestFixes:Load()\n"
            "  return { [42] = { [questKeys.level] = 10, [questKeys.extra] = {[1] = \"one\", [zoneIDs.STORMWIND] = \"city\"}, [questKeys.requiredClasses] = classIDs.WARRIOR + classIDs.PALADIN, [questKeys.name] = l10n('corrected') } }\n"
            "end\n"
            "function ForeverQuestFixes:LoadDynamic() return {} end\n",
            encoding="utf-8",
        )
        era_corrections = self.repo / "src" / "corrections" / "Era"
        era_corrections.mkdir(parents=True)
        (era_corrections / "classicQuestFixes.lua").write_text(
            "function ClassicQuestFixes:Load() return { [42] = { [questKeys.level] = 9 } } end\n"
            "function ClassicQuestFixes:LoadAutomatics() return { [42] = { [questKeys.level] = 8 } } end\n",
            encoding="utf-8",
        )
        (era_corrections / "automaticQuestFixes.lua").write_text(
            "function AutomaticQuestFixes:Load() return { [42] = { [questKeys.level] = 7 } } end\n",
            encoding="utf-8",
        )
        shared_corrections = self.repo / "src" / "corrections" / "Shared"
        shared_corrections.mkdir(parents=True)
        (shared_corrections / "itemStartFixes.lua").write_text(
            "function ItemStartFixes:LoadAutomaticQuestStarts() return { [42] = { [itemKeys.startQuest] = 12 } } end\n",
            encoding="utf-8",
        )
        legacy = corrections / "legacy"
        legacy.mkdir()
        (legacy / "classicQuestFixes.lua").write_text(
            "function LegacyQuestFixes:Load() return { [42] = { [questKeys.level] = 6 } } end\n",
            encoding="utf-8",
        )
        other_expansion = self.repo / "src" / "corrections" / "Cata"
        other_expansion.mkdir(parents=True)
        (other_expansion / "cataQuestFixes.lua").write_text(
            "function CataQuestFixes:Load() return { [42] = { [questKeys.level] = 99 } } end\n",
            encoding="utf-8",
        )
        (self.repo / "generator").mkdir()
        (self.repo / "generator/runtime.lua").write_text("fixture")
        def fixture_static(repo, revision, lua):
            rows = []
            for kind, prefix in (("quest", "Quest"), ("npc", "Npc"), ("item", "Item"), ("object", "Object")):
                rows.extend(parse_entity_file(repo / "data/Forever" / f"forever{prefix}DB.lua", kind, repo))
            return rows, {"staticProviders": []}
        self.static_patch = patch("import_source.effective_static_rows", side_effect=fixture_static)
        self.static_patch.start()
        self.addCleanup(self.static_patch.stop)
        subprocess.run(["git", "init", "-q", str(self.repo)], check=True)
        subprocess.run(["git", "-C", str(self.repo), "add", "."], check=True)
        subprocess.run(
            ["git", "-C", str(self.repo), "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"],
            check=True,
        )

    def tearDown(self):
        self.temp.cleanup()

    def test_entity_import_preserves_zero_empty_and_absent_values(self):
        source = self.repo / "data" / "Forever" / "foreverQuestDB.lua"
        rows = parse_entity_file(source, "quest", self.repo)
        first = next(row for row in rows if row["entityId"] == "42")
        self.assertEqual(first["attributes"]["level"], 0)
        self.assertEqual(first["attributes"]["emptyList"], [])
        self.assertEqual(first["fieldStates"]["emptyList"], "empty")
        self.assertEqual(first["fieldStates"]["effectiveValues"], "unavailable")
        self.assertEqual(first["provenance"]["name"]["path"], "data/Forever/foreverQuestDB.lua")
        second = next(row for row in rows if row["entityId"] == "43")
        self.assertEqual(second["fieldStates"]["name"], "missing")

    @unittest.skipUnless((ROOT / ".cache/QuestieDB/generator/runtime.lua").exists(), "cached upstream runtime unavailable")
    def test_registered_static_runtime_exports_effective_values(self):
        self.static_patch.stop()
        upstream = ROOT / ".cache/QuestieDB"
        repo = Path(self.temp.name) / "runtime"
        repo.mkdir()
        for path in upstream.iterdir():
            if path.name != "src":
                (repo / path.name).symlink_to(path, target_is_directory=path.is_dir())
        (repo / "src/corrections").mkdir(parents=True)
        for path in (upstream / "src").iterdir():
            if path.name not in ("config.lua", "corrections"):
                (repo / "src" / path.name).symlink_to(path, target_is_directory=path.is_dir())
        for path in (upstream / "src/corrections").iterdir():
            (repo / "src/corrections" / path.name).symlink_to(path, target_is_directory=path.is_dir())
        config = (upstream / "src/config.lua").read_text()
        specs = """
config.ownedCorrections[#config.ownedCorrections + 1] = {owned='Forever',file='probe.lua',module='ImportProbe',datatype='Quest',static={'Later','First'},dynamic={'Dynamic'},window='Forever'}
config.ownedCorrections[#config.ownedCorrections + 1] = {owned='Forever',file='probe.lua',module='ImportProbe',datatype='Quest',static={'Restricted'},window='Forever',sourceExpansionOrder=0,options={noOverwrites=true}}
"""
        (repo / "src/config.lua").write_text(config.replace("return config", specs + "return config"))
        (repo / "src/corrections/probe.lua").write_text("""
local module = QuestieLoader:CreateModule('ImportProbe')
local keys = QuestieLoader:ImportModule('QuestieDB').questKeys
function module:Later()
 return {[33]={[keys.requiredLevel]={},[keys.preQuestSingle_add]={777},[keys.preQuestSingle_remove]={888}},[999999]={[keys.name]='new quest',[keys.exclusiveTo]={888,555}}}
end
function module:Restricted() return {[33]={[keys.requiredLevel]=99},[999998]={[keys.requiredLevel]=1},[999997]={[keys.name]='named inherited'}} end
function module:Dynamic() return {[999999]={[keys.name]='dynamic name'}} end
function module:First()
 return {[33]={[keys.preQuestSingle]={777,999},[keys.startedBy]={[2]={123}},[keys.objectives]={[3]={{750,nil,7}}},[keys.triggerEnd]={[2]={[1]={{10,20}}}},[keys.extraObjectives]={{[1]={[1]={{3,4}}},[5]={{"item",750}}}},[keys.requiredSkill]={[2]=99}},[999999]={[keys.exclusiveTo_add]={777},[keys.exclusiveTo_remove]={888}}}
end
""")
        out = Path(self.temp.name) / "effective"
        import_source(repo, "forever", out, None)
        path = out / "records.jsonl"
        lines = path.read_text().splitlines() if path.exists() else gzip.decompress((out / "records.jsonl.gz").read_bytes()).decode().splitlines()
        rows = [json.loads(line) for line in lines]
        quest = next(row for row in rows if row["entityType"] == "quest" and row["entityId"] == "33")
        self.assertNotEqual(quest["fieldStates"].get("effectiveValues"), "unavailable")
        manifest = json.loads((out / "manifest.json").read_text())
        self.assertIsNone(quest["attributes"]["requiredLevel"])
        self.assertEqual(quest["fieldStates"]["requiredLevel"], "empty")
        self.assertEqual(quest["attributes"]["objectives"], [None, None, [[750, None, 7]]])
        self.assertEqual(quest["attributes"]["startedBy"], [None, [123]])
        self.assertEqual(quest["attributes"]["triggerEnd"], [None, {"1": [[10, 20]]}])
        self.assertEqual(quest["attributes"]["extraObjectives"], [[{"1": [[3, 4]]}, None, None, None, [["item", 750]]]])
        self.assertEqual(quest["attributes"]["requiredSkill"], [None, 99])
        self.assertEqual(quest["attributes"]["preQuestSingle"], [777, 999])
        self.assertEqual(quest["provenance"]["preQuestSingle"]["provider"], "probe.lua:First")
        self.assertEqual(quest["provenance"]["requiredLevel"]["provider"], "probe.lua:Later")
        created = next(row for row in rows if row["entityType"] == "quest" and row["entityId"] == "999999")
        self.assertEqual(created["attributes"]["exclusiveTo"], [555, 777])
        self.assertEqual(created["attributes"]["name"], "new quest")
        self.assertFalse(any(row["entityId"] == "999998" for row in rows))
        self.assertTrue(any(row["entityId"] == "999997" for row in rows))
        self.assertEqual(created["provenance"]["exclusiveTo"]["provider"], "probe.lua:First")
        self.assertIn("staticProviders", manifest)
        self.assertTrue(any("generated/foreverBaseQuest" in p["name"] for p in manifest["staticProviders"]))
        self.assertTrue(any("traces/foreverQuestTraces" in p["name"] for p in manifest["staticProviders"]))

    def test_rejects_separate_era_flavor(self):
        with self.assertRaisesRegex(ImportErrorMessage, "unsupported flavor"):
            import_source(self.repo, "era", Path(self.temp.name) / "era", None)

    def test_compressed_snapshot_preserves_checksum_and_repeated_bytes(self):
        import hashlib
        out = Path(self.temp.name) / "compressed"
        with patch("canonical.COMPRESSION_THRESHOLD", 1):
            import_source(self.repo, "forever", out, None)
            first = (out / "records.jsonl.gz").read_bytes()
            manifest = json.loads((out / "manifest.json").read_text())
            self.assertEqual(hashlib.sha256(gzip.decompress(first)).hexdigest(), manifest["recordsSha256"])
            self.assertFalse((out / "records.jsonl").exists())
            import_source(self.repo, "forever", out, None)
            self.assertEqual(first, (out / "records.jsonl.gz").read_bytes())
        import_source(self.repo, "forever", out, None)
        self.assertTrue((out / "records.jsonl").exists())
        self.assertFalse((out / "records.jsonl.gz").exists())

    def test_missing_classic_quest_fails_without_replacing_snapshot(self):
        out = Path(self.temp.name) / "snapshot"
        import_source(self.repo, "forever", out, None)
        before = (out / "records.jsonl").read_bytes()
        classic = self.repo / "data" / "Classic" / "classicQuestDB.lua"
        classic.write_text(classic.read_text().replace("[43] = {nil, nil, nil}", '[43] = {nil, nil, nil}, [44] = {"Classic only"}'))
        with self.assertRaisesRegex(ImportErrorMessage, "omit 1 Classic quest ID"):
            import_source(self.repo, "forever", out, None)
        self.assertEqual(before, (out / "records.jsonl").read_bytes())

    def test_forever_import_is_reproducible(self):
        forever_out = Path(self.temp.name) / "data" / "forever" / "sources" / "questiedb"
        import_source(self.repo, "forever", forever_out, None)
        manifest = json.loads((forever_out / "manifest.json").read_text())
        self.assertEqual(manifest["flavor"], "forever")
        first = (forever_out / "records.jsonl").read_bytes()
        import_source(self.repo, "forever", forever_out, None)
        self.assertEqual(first, (forever_out / "records.jsonl").read_bytes())


if __name__ == "__main__":
    unittest.main()
