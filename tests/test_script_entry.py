import importlib.util
import sys
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "mysql_schema_compare.py"
SPEC = importlib.util.spec_from_file_location("mysql_schema_compare_script", SCRIPT_PATH)
SCRIPT = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = SCRIPT
SPEC.loader.exec_module(SCRIPT)


class ScriptEntryTest(unittest.TestCase):
    def test_render_schema_diff_limits_table_details(self) -> None:
        schema_diff = SCRIPT.SchemaDiff(
            source_schema="source_db",
            target_schema="target_db",
            source_only_tables=[f"table_{index}" for index in range(101)],
            target_only_tables=[],
            changed_tables=[],
        )

        lines = SCRIPT.render_schema_diff(schema_diff)

        self.assertIn("      table differences: total=101, showing_first=100", lines)
        self.assertIn("      omitted table detail count: 1", lines)
        rendered_table_lines = [line for line in lines if line.startswith("      source only table:")]
        self.assertEqual(100, len(rendered_table_lines))

    def test_render_text_summary_contains_shell_friendly_counts(self) -> None:
        summary = SCRIPT.ComparisonSummary(
            total_targets=3,
            successful_targets=2,
            failed_targets=1,
            consistent_targets=1,
            inconsistent_targets=1,
        )

        lines = SCRIPT.render_text_summary(summary, exit_code=2)

        self.assertIn("MYSQL_SCHEMA_COMPARE_FAILED_TARGETS=1", lines)
        self.assertIn("MYSQL_SCHEMA_COMPARE_INCONSISTENT_TARGETS=1", lines)
        self.assertIn("MYSQL_SCHEMA_COMPARE_EXIT_CODE=2", lines)


if __name__ == "__main__":
    unittest.main()
