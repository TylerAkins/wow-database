import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools" / "questiedb"))

from import_source import import_source, parse_entity_file, parse_static_corrections  # noqa: E402
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

    def test_rejects_arithmetic_instead_of_evaluating_it(self):
        with self.assertRaises(LuaLiteralError):
            parse("return { [1] = 1 + 2 }")


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
            "  return { [42] = { [questKeys.level] = 10, [questKeys.name] = l10n('corrected') } }\n"
            "end\n"
            "function ForeverQuestFixes:LoadDynamic() return {} end\n",
            encoding="utf-8",
        )
        era_corrections = self.repo / "src" / "corrections" / "Era"
        era_corrections.mkdir(parents=True)
        (era_corrections / "classicQuestFixes.lua").write_text(
            "function ClassicQuestFixes:Load() return { [42] = { [questKeys.level] = 9 } } end\n",
            encoding="utf-8",
        )
        shared_corrections = self.repo / "src" / "corrections" / "Shared"
        shared_corrections.mkdir(parents=True)
        (shared_corrections / "itemStartFixes.lua").write_text(
            "function ItemStartFixes:LoadAutomaticQuestStarts() return { [42] = { [itemKeys.startQuest] = 12 } } end\n",
            encoding="utf-8",
        )
        other_expansion = self.repo / "src" / "corrections" / "Cata"
        other_expansion.mkdir(parents=True)
        (other_expansion / "cataQuestFixes.lua").write_text(
            "function CataQuestFixes:Load() return { [42] = { [questKeys.level] = 99 } } end\n",
            encoding="utf-8",
        )
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

    def test_static_corrections_are_preserved_as_unevaluated_records(self):
        revision = subprocess.run(["git", "-C", str(self.repo), "rev-parse", "HEAD"], check=True, capture_output=True, text=True).stdout.strip()
        rows, limitations = parse_static_corrections(self.repo, "forever", revision)
        correction = next(row for row in rows if row["entityId"].startswith("correction:Forever:") and row["entityId"].endswith(":42:level"))
        self.assertEqual(correction["attributes"]["correction"]["value"], 10)
        self.assertEqual(correction["attributes"]["correction"]["application"], "not-evaluated")
        dynamic = next(row for row in rows if row["entityId"].startswith("correction:Forever:") and row["entityId"].endswith(":42:name"))
        self.assertEqual(dynamic["attributes"]["correction"]["value"]["$luaExpression"], "l10n")
        self.assertTrue(any(row["entityId"].startswith("correction:Era:") and row["attributes"]["correction"]["value"] == 9 for row in rows))
        self.assertTrue(any(row["entityId"].startswith("correction:Shared:item:") and row["attributes"]["correction"]["value"] == 12 for row in rows))
        self.assertFalse(any(row["attributes"]["correction"]["value"] == 99 for row in rows))
        self.assertTrue(any("Dynamic correction" in limitation for limitation in limitations))

    def test_era_import_keeps_only_classic_and_shared_corrections(self):
        revision = subprocess.run(["git", "-C", str(self.repo), "rev-parse", "HEAD"], check=True, capture_output=True, text=True).stdout.strip()
        rows, _ = parse_static_corrections(self.repo, "era", revision)
        self.assertTrue(any(row["entityId"].startswith("correction:Era:") for row in rows))
        self.assertTrue(any(row["entityId"].startswith("correction:Shared:") for row in rows))
        self.assertFalse(any(row["entityId"].startswith("correction:Forever:") for row in rows))
        self.assertFalse(any(row["attributes"]["correction"]["value"] == 99 for row in rows))

    def test_import_writes_both_flavors_separately_and_is_reproducible(self):
        revision = subprocess.run(["git", "-C", str(self.repo), "rev-parse", "HEAD"], check=True, capture_output=True, text=True).stdout.strip()
        era_out = Path(self.temp.name) / "data" / "era" / "sources" / "questiedb"
        forever_out = Path(self.temp.name) / "data" / "forever" / "sources" / "questiedb"
        import_source(self.repo, "era", era_out, revision)
        import_source(self.repo, "forever", forever_out, revision)
        era_manifest = json.loads((era_out / "manifest.json").read_text())
        forever_manifest = json.loads((forever_out / "manifest.json").read_text())
        self.assertEqual(era_manifest["flavor"], "era")
        self.assertEqual(forever_manifest["flavor"], "forever")
        first = (forever_out / "records.jsonl").read_bytes()
        import_source(self.repo, "forever", forever_out, revision)
        self.assertEqual(first, (forever_out / "records.jsonl").read_bytes())


if __name__ == "__main__":
    unittest.main()
