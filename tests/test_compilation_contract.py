import copy
import json
import unittest
from pathlib import Path

from tools.canonical import validate_rows


class CompilationContractTests(unittest.TestCase):
    def test_published_schema_and_field_priorities(self):
        root = Path(__file__).resolve().parents[1]
        manifest_schema = json.loads((root / "schemas/compilation-manifest.schema.json").read_text())
        self.assertIn("compilationFormatVersion", manifest_schema["required"])
        self.assertEqual(manifest_schema["properties"]["compilationFormatVersion"]["const"], 2)

        attributes = json.loads((root / "schemas/compilation-attributes.schema.json").read_text())
        self.assertEqual(attributes["properties"]["questLevel"]["type"], ["integer", "null"])
        self.assertEqual(attributes["properties"]["zoneOrSort"]["type"], ["integer", "null"])
        self.assertIn("required", attributes["$defs"]["basicObjectives"]["items"])
        self.assertEqual(attributes["$defs"]["basicObjectives"]["items"]["required"], ["id"])

        policy = json.loads((root / "schemas/field-resolution.json").read_text())["fields"]
        for field in ("startedBy", "finishedBy", "objectives", "requiredRaces", "preQuestGroup"):
            self.assertEqual(policy[f"quest.{field}"], ["questiedb", "wowhead", "allthethings"])
        self.assertNotIn("quest.unrecognizedField", policy)

    def fixture(self):
        row = {"schemaVersion": 1, "flavor": "forever", "entityType": "quest", "entityId": "1",
               "source": "compilation", "sourceRevision": "compiled",
               "attributes": {"name": "Quest", "startedBy": [{"type": "npc", "id": 9}],
                              "sourceData": {"questiedb": {"revision": "q", "attributes": {"name": "Quest"}}}},
               "provenance": {"name": {"sources": [{"source": "questiedb", "revision": "q"}]}}}
        manifest = {"source": "compilation", "flavor": "forever", "sourceRevision": "compiled", "compilationFormatVersion": 2}
        return row, manifest

    def test_format_two_and_legacy_marker(self):
        row, manifest = self.fixture()
        validate_rows([row], manifest)
        legacy = dict(manifest)
        legacy.pop("compilationFormatVersion")
        validate_rows([row], legacy)

    def test_future_version_and_source_marker_fail(self):
        row, manifest = self.fixture()
        for version in (1, 3, "2", True):
            with self.subTest(version=version), self.assertRaises(ValueError):
                validate_rows([row], dict(manifest, compilationFormatVersion=version))
        with self.assertRaises(ValueError):
            validate_rows([], dict(manifest, source="questiedb"))

    def test_legacy_shapes_and_corrupt_originals_fail(self):
        row, manifest = self.fixture()
        for mutation in ("provider", "audit", "original"):
            bad = copy.deepcopy(row)
            if mutation == "provider":
                bad["attributes"]["startedBy"] = [[9]]
            elif mutation == "audit":
                bad["attributes"].pop("sourceData")
            else:
                bad["attributes"]["sourceData"]["questiedb"]["revision"] = ""
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                validate_rows([bad], manifest)

    def test_python_reader_preserves_signed_prerequisite_groups(self):
        row, manifest = self.fixture()
        row["attributes"]["preQuestGroup"] = [4976, -4962]
        validate_rows([row], manifest)
        row["attributes"]["preQuestGroup"] = [0]
        with self.assertRaises(ValueError):
            validate_rows([row], manifest)

    def test_python_reader_preserves_signed_spell_lock(self):
        row, manifest = self.fixture()
        row["attributes"]["requiredSpell"] = -1229432
        validate_rows([row], manifest)

    def test_python_reader_preserves_explicit_scalar_clear(self):
        row, manifest = self.fixture()
        row["attributes"]["requiredSpell"] = None
        row["fieldStates"] = {"requiredSpell": "empty"}
        validate_rows([row], manifest)
        row["fieldStates"] = {}
        with self.assertRaises(ValueError):
            validate_rows([row], manifest)

    def test_python_reader_rejects_malformed_format_two_shared_fields(self):
        row, manifest = self.fixture()
        for field, value in (("questLevel", 1.5), ("preQuestSingle", 7),
                             ("objectives", {"creature": [{"text": "Missing ID"}]})):
            bad = copy.deepcopy(row)
            bad["attributes"][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                validate_rows([bad], manifest)
