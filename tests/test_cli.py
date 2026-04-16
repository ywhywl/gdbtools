import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from gdbtools.cli import build_summary, determine_exit_code
from gdbtools.models import PrivilegeDiff, TargetComparison


class CliSummaryTest(unittest.TestCase):
    def test_summary_counts_success_failure_and_inconsistency(self) -> None:
        comparisons = [
            TargetComparison(
                target="target_a",
                schema_pairs=[],
                schema_diffs=[],
                privilege_diff=PrivilegeDiff(),
            ),
            TargetComparison(
                target="target_b",
                schema_pairs=[],
                schema_diffs=[],
                privilege_diff=PrivilegeDiff(changed_identities=[{"identity": "app@%"}]),
            ),
            TargetComparison(
                target="target_c",
                schema_pairs=[],
                schema_diffs=[],
                privilege_diff=PrivilegeDiff(),
                error="connection failed",
            ),
        ]

        summary = build_summary(comparisons)
        self.assertEqual(3, summary.total_targets)
        self.assertEqual(2, summary.successful_targets)
        self.assertEqual(1, summary.failed_targets)
        self.assertEqual(1, summary.consistent_targets)
        self.assertEqual(1, summary.inconsistent_targets)

    def test_exit_code_prefers_failure(self) -> None:
        summary = build_summary(
            [
                TargetComparison(
                    target="target_a",
                    schema_pairs=[],
                    schema_diffs=[],
                    privilege_diff=PrivilegeDiff(),
                    error="query timeout",
                ),
                TargetComparison(
                    target="target_b",
                    schema_pairs=[],
                    schema_diffs=[],
                    privilege_diff=PrivilegeDiff(changed_identities=[{"identity": "app"}]),
                ),
            ]
        )
        self.assertEqual(2, determine_exit_code(summary))

    def test_exit_code_returns_nonzero_on_difference(self) -> None:
        summary = build_summary(
            [
                TargetComparison(
                    target="target_a",
                    schema_pairs=[],
                    schema_diffs=[],
                    privilege_diff=PrivilegeDiff(target_only_identities=[{"identity": "app"}]),
                )
            ]
        )
        self.assertEqual(1, determine_exit_code(summary))

    def test_exit_code_returns_zero_when_all_consistent(self) -> None:
        summary = build_summary(
            [
                TargetComparison(
                    target="target_a",
                    schema_pairs=[],
                    schema_diffs=[],
                    privilege_diff=PrivilegeDiff(),
                )
            ]
        )
        self.assertEqual(0, determine_exit_code(summary))


if __name__ == "__main__":
    unittest.main()
