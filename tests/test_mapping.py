import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from gdbtools.diffing import map_schema_pairs


class MapSchemaPairsTest(unittest.TestCase):
    def test_exact_one_to_many(self) -> None:
        pairs = map_schema_pairs(
            source_available=["dbname_0"],
            source_selected=["dbname_0"],
            source_selectors=["dbname_0"],
            target_available=["dbname_1", "dbname_2"],
            target_selected=["dbname_1", "dbname_2"],
            target_selectors=["dbname_1", "dbname_2"],
        )
        self.assertEqual(
            [("dbname_0", "dbname_1"), ("dbname_0", "dbname_2")],
            [(item.source_schema, item.target_schema) for item in pairs],
        )

    def test_exact_pair_by_index(self) -> None:
        pairs = map_schema_pairs(
            source_available=["s1", "s2"],
            source_selected=["s1", "s2"],
            source_selectors=["s1", "s2"],
            target_available=["t1", "t2"],
            target_selected=["t1", "t2"],
            target_selectors=["t1", "t2"],
        )
        self.assertEqual(
            [("s1", "t1"), ("s2", "t2")],
            [(item.source_schema, item.target_schema) for item in pairs],
        )

    def test_wildcard_uses_same_name_intersection(self) -> None:
        pairs = map_schema_pairs(
            source_available=["a_0", "a_1"],
            source_selected=["a_0", "a_1"],
            source_selectors=["a_%"],
            target_available=["a_1", "a_2"],
            target_selected=["a_1", "a_2"],
            target_selectors=["a_%"],
        )
        self.assertEqual([("a_1", "a_1")], [(item.source_schema, item.target_schema) for item in pairs])


if __name__ == "__main__":
    unittest.main()
