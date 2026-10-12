import unittest

from tools.coverage_guard import check_retention


class CoverageGuardTests(unittest.TestCase):
    def test_retains_threshold_and_allows_growth(self):
        self.assertTrue(check_retention(100, 50, 0.5))
        self.assertTrue(check_retention(100, 120, 0.5))

    def test_flags_large_record_loss(self):
        self.assertFalse(check_retention(100, 49, 0.5))

    def test_new_dataset_has_no_previous_coverage_to_compare(self):
        self.assertTrue(check_retention(0, 1, 1.0))


if __name__ == "__main__":
    unittest.main()
