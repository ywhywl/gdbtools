import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from gdbtools.cli import parse_args
from gdbtools.config import parse_connection_dsn, parse_target_dsns


class ConfigParseTest(unittest.TestCase):
    def test_connection_uses_default_credentials(self) -> None:
        config = parse_connection_dsn(
            "mysql://127.0.0.1:3307/dbname",
            default_user="app",
            default_password="secret",
        )
        self.assertEqual("app", config.user)
        self.assertEqual("secret", config.password)
        self.assertEqual("127.0.0.1", config.host)
        self.assertEqual(3307, config.port)
        self.assertEqual("dbname", config.database)

    def test_explicit_user_keeps_default_password(self) -> None:
        config = parse_connection_dsn(
            "mysql://reader@127.0.0.1:3306/",
            default_user="app",
            default_password="secret",
        )
        self.assertEqual("reader", config.user)
        self.assertEqual("secret", config.password)

    def test_target_dsn_supports_multiple_separators_in_one_argument(self) -> None:
        configs = parse_target_dsns(
            ["mysql://10.0.0.1:3306/|mysql://10.0.0.2:3306/\nmysql://10.0.0.3:3306/"],
            default_user="app",
            default_password="secret",
        )
        self.assertEqual(3, len(configs))
        self.assertEqual(
            ["10.0.0.1", "10.0.0.2", "10.0.0.3"],
            [config.host for config in configs],
        )

    def test_cli_parse_args_accepts_default_credentials(self) -> None:
        options = parse_args(
            [
                "--source-dsn",
                "mysql://127.0.0.1:3306/",
                "--target-dsn",
                "mysql://10.0.0.1:3306/,mysql://10.0.0.2:3306/",
                "--default-user",
                "app",
                "--default-password",
                "secret",
            ]
        )
        self.assertEqual("app", options.source.user)
        self.assertEqual("secret", options.source.password)
        self.assertEqual(2, len(options.targets))
        self.assertEqual(["10.0.0.1", "10.0.0.2"], [item.host for item in options.targets])


if __name__ == "__main__":
    unittest.main()
