import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from gdbtools.collector import ensure_bundle, remap_privilege_bundles
from gdbtools.diffing import diff_privileges


class PrivilegeMergeTest(unittest.TestCase):
    def test_user_mode_merges_hosts(self) -> None:
        bundles = {}
        first = ensure_bundle(bundles, "app", "%", "user")
        first.global_privileges.add("SELECT")
        first.hosts.add("%")

        second = ensure_bundle(bundles, "app", "10.%", "user")
        second.global_privileges.add("UPDATE")
        second.hosts.add("10.%")

        self.assertEqual(1, len(bundles))
        payload = bundles["app"].to_dict()
        self.assertEqual(["%", "10.%"], payload["hosts"])
        self.assertEqual(["SELECT", "UPDATE"], payload["global_privileges"])

    def test_diff_privileges_finds_missing_identity(self) -> None:
        source = {}
        target = {}
        ensure_bundle(source, "app", "%", "user_host").global_privileges.add("SELECT")
        diff = diff_privileges(source, target)
        self.assertEqual(1, len(diff.source_only_identities))
        self.assertFalse(diff.target_only_identities)
        self.assertFalse(diff.changed_identities)

    def test_remap_privileges_normalizes_schema_name(self) -> None:
        bundles = {}
        bundle = ensure_bundle(bundles, "app", "%", "user_host")
        bundle.db_privileges.setdefault("dbname_1", set()).add("SELECT")
        bundle.table_privileges.setdefault(("dbname_1", "orders"), set()).add("SELECT")

        remapped = remap_privilege_bundles(bundles, {"dbname_1": "dbname_0"})
        payload = remapped["app@%"].to_dict()
        self.assertIn("dbname_0", payload["db_privileges"])
        self.assertIn("dbname_0.orders", payload["table_privileges"])


if __name__ == "__main__":
    unittest.main()
