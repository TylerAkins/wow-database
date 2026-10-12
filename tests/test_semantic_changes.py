import gzip
import os
import subprocess
import tempfile
from pathlib import Path
import unittest

from tools.semantic_changes import changed_products, normalized_product, semantic_records_equal


class SemanticChangesTests(unittest.TestCase):
    def test_revision_only_changes_are_ignored(self):
        before = b'{"entityType":"quest","entityId":"1","sourceRevision":"old","attributes":{"name":"A"},"provenance":{"name":{"source":"wowhead","revision":"old"}}}\n'
        after = b'{"entityType":"quest","entityId":"1","sourceRevision":"new","attributes":{"name":"A"},"provenance":{"name":{"source":"wowhead","revision":"new"}}}\n'
        self.assertTrue(semantic_records_equal(before, after))

    def test_value_changes_are_reported(self):
        before = b'{"entityType":"quest","entityId":"1","sourceRevision":"old","attributes":{"name":"A"}}\n'
        after = b'{"entityType":"quest","entityId":"1","sourceRevision":"new","attributes":{"name":"B"}}\n'
        self.assertFalse(semantic_records_equal(before, after))

    def test_compilation_products_ignore_revision_only_provenance(self):
        before = b'{"sourceRevision":"old","attributes":{"name":"A"},"provenance":{"name":{"sources":[{"source":"wowhead","revision":"old"}]}}}\n'
        after = b'{"sourceRevision":"new","attributes":{"name":"A"},"provenance":{"name":{"sources":[{"source":"wowhead","revision":"new"}]}}}\n'
        self.assertEqual(normalized_product("data/forever/compilation/records.jsonl", before), normalized_product("data/forever/compilation/records.jsonl", after))

    def test_compressed_products_compare_decoded_values(self):
        data = b'{"entityType":"quest","entityId":"1","attributes":{"name":"A"}}\n'
        compressed = gzip.compress(data, mtime=0)
        self.assertEqual(
            normalized_product("data/era/sources/questiedb/records.jsonl.gz", compressed),
            normalized_product("data/era/sources/questiedb/records.jsonl", data),
        )
        with self.assertRaises((EOFError, OSError)):
            normalized_product("data/era/sources/questiedb/records.jsonl.gz", compressed[:-8])

    def test_storage_transition_does_not_create_semantic_change(self):
        original_cwd = Path.cwd()
        with tempfile.TemporaryDirectory() as temp:
            try:
                os.chdir(temp)
                subprocess.run(["git", "init", "-q"], check=True)
                path = Path("data/era/sources/questiedb/records.jsonl")
                path.parent.mkdir(parents=True)
                before = b'{"entityType":"quest","entityId":"1","sourceRevision":"old","attributes":{"name":"A"}}\n'
                path.write_bytes(before)
                subprocess.run(["git", "add", "data"], check=True)
                subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"], check=True)
                path.unlink()
                compressed = path.with_suffix(".jsonl.gz")
                compressed.write_bytes(gzip.compress(before.replace(b'old', b'new'), mtime=0))
                self.assertEqual(changed_products("HEAD"), [])
                compressed.write_bytes(gzip.compress(before.replace(b'"A"', b'"B"'), mtime=0))
                self.assertEqual(changed_products("HEAD"), [compressed.as_posix()])
            finally:
                os.chdir(original_cwd)

    def test_legacy_manifest_ignores_upstream_commit_only(self):
        before = b'{"commit":"old","generatedAt":"old-time","questCount":12}'
        after = b'{"commit":"new","generatedAt":"new-time","questCount":12}'
        self.assertEqual(normalized_product("export/forever/manifest.json", before), normalized_product("export/forever/manifest.json", after))


if __name__ == "__main__":
    unittest.main()
