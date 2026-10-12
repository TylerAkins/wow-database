import unittest

from tools.semantic_changes import normalized_product, semantic_records_equal


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

    def test_legacy_manifest_ignores_upstream_commit_only(self):
        before = b'{"commit":"old","generatedAt":"old-time","questCount":12}'
        after = b'{"commit":"new","generatedAt":"new-time","questCount":12}'
        self.assertEqual(normalized_product("export/forever/manifest.json", before), normalized_product("export/forever/manifest.json", after))


if __name__ == "__main__":
    unittest.main()
