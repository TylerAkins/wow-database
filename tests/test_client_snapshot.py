import gzip
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from tools.client.package_snapshot import package_snapshot


class ClientSnapshotTests(unittest.TestCase):
    def fixture(self, root):
        root.mkdir()
        tables = [{"name": name, "fileDataId": index, "definitionSha256": "definition", "status": "written", "recordCount": 1}
                  for index, name in enumerate(("QuestV2", "TactKey"), 1)]
        manifest = {"schemaVersion": 1, "gameVersion": "forever", "source": "foreverclient", "client": {"buildNumber": 70338}, "tables": tables}
        (root / "manifest.json").write_text(json.dumps(manifest))
        for table in tables:
            header = {"kind": "table", "schemaVersion": 1, "gameVersion": "forever", "source": "foreverclient", "clientBuild": 70338,
                      "table": table["name"], "fileDataId": table["fileDataId"], "definitionSha256": "definition"}
            (root / (table["name"] + ".jsonl")).write_text(json.dumps(header) + "\n" + json.dumps({"kind": "record", "id": 42, "fields": {"ID": 42}}) + "\n")
        return manifest

    def test_packages_identical_bytes_and_excludes_keys(self):
        with tempfile.TemporaryDirectory() as temp:
            source, output = Path(temp) / "input", Path(temp) / "output"
            self.fixture(source)
            package_snapshot(source, output)
            first = (output / "QuestV2.jsonl.gz").read_bytes()
            self.assertEqual(gzip.decompress(first), (source / "QuestV2.jsonl").read_bytes())
            index = json.loads((output / "files.json").read_text())
            self.assertEqual(index["files"]["QuestV2.jsonl.gz"]["recordsSha256"], hashlib.sha256(gzip.decompress(first)).hexdigest())
            self.assertIn("TactKey", index["excludedTables"])
            self.assertFalse((output / "TactKey.jsonl.gz").exists())
            package_snapshot(source, output)
            self.assertEqual(first, (output / "QuestV2.jsonl.gz").read_bytes())

    def test_bad_count_preserves_published_snapshot(self):
        with tempfile.TemporaryDirectory() as temp:
            source, output = Path(temp) / "input", Path(temp) / "output"
            manifest = self.fixture(source)
            package_snapshot(source, output)
            before = (output / "QuestV2.jsonl.gz").read_bytes()
            manifest["tables"][0]["recordCount"] = 2
            (source / "manifest.json").write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, "record count"):
                package_snapshot(source, output)
            self.assertEqual(before, (output / "QuestV2.jsonl.gz").read_bytes())

    def test_rejects_table_path_traversal(self):
        with tempfile.TemporaryDirectory() as temp:
            source, output = Path(temp) / "input", Path(temp) / "output"
            manifest = self.fixture(source)
            manifest["tables"][0]["name"] = "../QuestV2"
            (source / "manifest.json").write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, "invalid or duplicate"):
                package_snapshot(source, output)
            self.assertFalse(output.exists())
