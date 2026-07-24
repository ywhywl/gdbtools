# gdbtools

## Repository Guide

This repository is a mixed Go/Python toolset. New features should follow the existing layout and conventions below so that CLI behavior, package boundaries, tests, and docs stay consistent.

### Directory structure

- `cmd/`
  - Go CLI entrypoints only
  - one command per directory, usually only `main.go`
  - examples: `cmd/mysqlcompare`, `cmd/mysqlpricheck`, `cmd/insight-create-dbmgr`
- `internal/`
  - Go application logic for commands
  - package names generally match the command or feature name
  - keep flag parsing in `run.go`, shared data structures in `types.go`, and domain logic split by responsibility
  - common file split patterns already used here:
    - `run.go`: CLI parsing and orchestration
    - `client.go`: external client wrappers such as MySQL or HTTP
    - `collector.go`: data collection / query logic
    - `diff.go` or `rules.go`: comparison or rule evaluation
    - `report.go`: text and JSON rendering
    - `config.go`: file/config parsing
    - `utils.go`: small reusable helpers
    - `errors.go`: typed usage or validation errors
- `pkg/`
  - reusable Go libraries that are not tied to a single command
  - current example: `pkg/goldendb`
- `src/gdbtools/`
  - Python package implementation
  - use for Python CLI logic and reusable Python modules
- `scripts/`
  - standalone helper scripts and shell tools
  - keep script-specific docs next to the script when useful
- `tests/`
  - Python unit tests for `src/gdbtools`
- `docs/`
  - user-facing design docs, usage manuals, and interface docs

### Go code style

- Prefer small feature packages under `internal/<feature>/` instead of adding unrelated code to existing packages.
- Keep `cmd/<tool>/main.go` minimal:
  - call `<feature>.Run(os.Args[1:])`
  - print errors to `stderr`
  - exit with the returned code
- Use the standard `flag` package for CLI parsing.
- Return `(int, error)` from top-level `Run` functions.
- Use explicit validation with clear messages such as `--api is required`.
- Favor narrow files with clear responsibilities instead of one large package file.
- Keep structs and helper names straightforward:
  - `Options`
  - `FileConfig`
  - `ConnectionConfig`
  - `TargetComparison`
  - `AuditSummary`
- Prefer plain data models plus pure helper functions over deep object hierarchies.
- Use standard library first; avoid new dependencies unless they clearly reduce maintenance cost.
- Format all Go code with `gofmt`.

### Python code style

- Keep Python implementation under `src/gdbtools/` and expose CLI behavior through `src/gdbtools/cli.py` or `__main__.py`.
- Use dataclasses for configuration-style models where that is already the pattern.
- Keep modules focused by responsibility:
  - `config.py`
  - `collector.py`
  - `diffing.py`
  - `reporting.py`
  - `mysql_client.py`
- Prefer simple functions and explicit transformations over framework-heavy patterns.
- Tests use the standard library `unittest`.

### Naming conventions

- Command names use lowercase, no spaces, usually one feature per command.
- Go internal package names should match the command or domain name:
  - `internal/mysqlcompare`
  - `internal/mysqlpricheck`
  - `internal/insightdbmgr`
- File names should reflect responsibility rather than implementation detail.
- CLI flags use kebab-case:
  - `--source-dsn`
  - `--exclude-users`
  - `--output-format`
- JSON keys and config keys use snake_case where that is already established:
  - `default_user`
  - `default_password`

### CLI behavior conventions

- Support both text output and JSON output when the tool is intended for both humans and automation.
- Text output should be readable in terminals and shell pipelines.
- JSON output should be stable and structured enough for downstream parsing.
- Keep credential priority explicit in docs and code.
- Reuse existing multi-value input behavior when possible:
  - repeated flags
  - separators such as `,`, `|`, or newlines
- For command exit codes:
  - reserve `0` for success / no blocking issue
  - use non-zero codes for inconsistent state, API failure, validation failure, or runtime failure
  - document the exit code contract in the command usage doc

### Error handling conventions

- Validate required flags early in `Run`.
- Prefer actionable error messages that say which flag or input is invalid.
- Keep usage-style validation separate from operational failures when practical.
- When iterating over multiple targets, collect per-target failures instead of aborting the whole run immediately if partial results are still useful.

### Testing conventions

- Go tests live next to the package as `*_test.go`.
- Python tests live under `tests/`.
- Prefer focused unit tests around parsing, rule logic, diff behavior, and rendering.
- Follow the current pattern of lightweight fake clients for collector/diff tests instead of requiring live services.
- For Go commands, verify at least:
  - flag parsing
  - config parsing
  - core rule/diff behavior
  - output rendering
- For Python commands, verify:
  - config parsing
  - selector behavior
  - diff behavior
  - script entry behavior

### Documentation conventions

- Add user-facing docs under `docs/` for every new standalone CLI.
- Keep the root `README.md` updated with:
  - new command entrypoints
  - high-level purpose
  - links to detailed docs
- If a script is standalone and likely to be run directly, add usage notes either in `README.md` or a sibling `.md` file.
- Design docs should explain scope, data sources, rules, and exit codes.
- Usage docs should include build/run examples and parameter descriptions.

### Recommended workflow for new features

1. Decide whether the feature is:
   - a new Go CLI under `cmd/` + `internal/`
   - an addition to an existing Go package
   - a Python module/script under `src/gdbtools/` or `scripts/`
2. Reuse existing patterns from the closest feature package instead of inventing a new layout.
3. Add or update tests in the same turn as the code.
4. Add or update docs in the same turn as the code.
5. Run formatting and the smallest relevant test/build command before finishing.

### Current repository pattern summary

- Go is used for newer CLI tools and reusable clients.
- Python remains in use for existing MySQL compare tooling and helper scripts.
- The repo prefers explicit, standard-library-based CLI code over framework-heavy abstractions.
- Logic is typically organized as:
  - parse input
  - collect data
  - transform or compare
  - render text/JSON
  - return deterministic exit codes

## Development Templates

Use the following templates as the default baseline for new features. The purpose is to make new tools look like they belong in this repository on the first pass.

### Template: new Go CLI

Recommended layout:

```text
cmd/<tool>/
  main.go

internal/<tool>/
  run.go
  types.go
  client.go
  collector.go
  report.go
  utils.go
  errors.go
  <tool>_test.go

docs/
  <tool>_design.md
  <tool>_usage.md
```

Minimal `cmd/<tool>/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/<tool>"
)

func main() {
	exitCode, err := <tool>.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(exitCode)
}
```

Minimal `internal/<tool>/run.go`:

```go
package <tool>

import (
	"flag"
	"os"
)

func Run(argv []string) (int, error) {
	options, err := parseArgs(argv)
	if err != nil {
		return 0, err
	}

	report, err := execute(options)
	if err != nil {
		return 0, err
	}

	_, _ = os.Stdout.WriteString(renderReport(report, options.OutputFormat) + "\n")
	return determineExitCode(report), nil
}

func parseArgs(argv []string) (Options, error) {
	fs := flag.NewFlagSet("<tool>", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	// define flags here
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	return Options{}, nil
}
```

Suggested `types.go` baseline:

```go
package <tool>

type Options struct {
	OutputFormat string
}

type Report struct {
	Status string `json:"status"`
}
```

Go CLI checklist:

- `cmd/<tool>/main.go` must stay tiny
- top-level orchestration goes in `run.go`
- parsing and normalization helpers go in `utils.go`
- external systems go behind `client.go`
- collection/query logic goes in `collector.go`
- rendering goes in `report.go`
- shared models go in `types.go`
- user-facing docs go in `docs/`
- tests stay in the same package as `*_test.go`
- update `README.md` when the command is user-facing

### Template: new Python feature

Recommended layout when extending `src/gdbtools/`:

```text
src/gdbtools/
  cli.py
  config.py
  collector.py
  diffing.py
  reporting.py
  models.py
  utils.py

tests/
  test_<feature>.py
```

Minimal module baseline:

```python
from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class FeatureOptions:
    output_format: str = "text"


def run_feature(options: FeatureOptions) -> str:
    return options.output_format
```

Python checklist:

- public CLI parsing stays in `cli.py`
- config parsing stays in `config.py`
- collection logic stays in `collector.py`
- compare/rule logic stays in `diffing.py`
- rendering stays in `reporting.py`
- shared models stay in `models.py`
- add `tests/test_<feature>.py`
- avoid introducing a new layout unless the feature is truly standalone

### Template: new standalone script

Use `scripts/` only for intentionally standalone helpers.

Recommended layout:

```text
scripts/
  <tool>.py or <tool>.sh
  <tool>.md
```

Script checklist:

- keep dependencies minimal
- document invocation and required environment
- if logic grows beyond a small helper, migrate it into `src/gdbtools/` or `internal/<tool>/`

### Template: tests

For a new Go feature, start with tests in this shape:

```go
func TestParseArgs(t *testing.T) {}

func TestCollectorBehavior(t *testing.T) {}

func TestRuleOrDiffBehavior(t *testing.T) {}

func TestRenderOutput(t *testing.T) {}
```

For a new Python feature, start with tests in this shape:

```python
class FeatureTest(unittest.TestCase):
    def test_parse_config(self) -> None:
        pass

    def test_core_logic(self) -> None:
        pass

    def test_render_output(self) -> None:
        pass
```

### Template: docs

Each new standalone CLI should usually add:

- `docs/<tool>_design.md`
- `docs/<tool>_usage.md`

Design doc should include:

- problem statement
- scope and non-goals
- directory and module layout
- data sources / external dependencies
- workflow or rule definition
- output contract
- exit codes

Usage doc should include:

- build command
- run examples
- parameter descriptions
- config file examples
- output examples
- permission or environment requirements

### Pre-merge checklist

Before considering a feature complete, verify all of the following:

- code is placed in the correct top-level directory
- naming matches an existing nearby feature
- README is updated if the feature is user-facing
- docs are added or updated
- Go code is formatted with `gofmt`
- relevant Go tests pass
- relevant Python tests pass
- command help text is coherent
- exit codes are documented
- text output and JSON output are both reviewed if both are supported

### Decision rules

Use these rules to avoid layout drift:

- If it is a reusable client or library, prefer `pkg/`
- If it is internal logic for one Go command, prefer `internal/<tool>/`
- If it is a new Go executable, add `cmd/<tool>/main.go`
- If it extends the Python compare tooling, prefer `src/gdbtools/`
- If it is only a one-off operational helper, prefer `scripts/`
- If it needs user-facing behavior, add docs under `docs/`

## Go CLI

### Tool Index

| Tool | Type | Purpose | Docs |
| --- | --- | --- | --- |
| `insight-batch-onboard-hosts` | Go CLI | Batch onboard hosts into GoldenDB Insight | [Usage](docs/insight_batch_onboard_hosts.md), [Overview](docs/insight_tools_overview.md) |
| `insight-batch-create` | Go CLI | Batch create GoldenDB clusters through Insight | [Usage](docs/insight_batch_create.md), [Overview](docs/insight_tools_overview.md) |
| `insight-batch-add-cn` | Go CLI | Batch add CN nodes to existing GoldenDB clusters | [Usage](docs/insight_batch_add_cn.md), [Overview](docs/insight_tools_overview.md) |
| `insight-batch-add-dn` | Go CLI | Batch add DN nodes to existing GoldenDB clusters | [Usage](docs/insight_batch_add_dn.md), [Overview](docs/insight_tools_overview.md) |
| `insight-create-dbmgr` | Go CLI | Create and grant the dbmgr admin user through Insight | [Usage](docs/insight_create_dbmgr.md), [Overview](docs/insight_tools_overview.md) |
| `db-auth-lookup` | Go CLI | Query database authorization details for a business from four Excel/CSV mapping files | [Design](docs/db_auth_lookup_design.md), [Usage](docs/db_auth_lookup_usage.md) |
| `mysqlcompare` | Go CLI | Compare MySQL source/target schema structure and privileges | [Usage](docs/mysqlcompare_usage.md) |
| `mysqlpricheck` | Go CLI | Audit MySQL 5.7 privileges inside a single instance | [Design](docs/mysqlpricheck_design.md), [Usage](docs/mysqlpricheck_usage.md) |
| `mysql-rename-db` | Go CLI | Rename MySQL 5.7 database using RENAME TABLE with pre-flight checks | [Usage](docs/mysql_rename_db_usage.md) |

Build or run:

```bash
go build ./cmd/mysqlcompare
go run ./cmd/mysqlcompare --help

go build ./cmd/mysqlpricheck
go run ./cmd/mysqlpricheck --help

go build ./cmd/db-auth-lookup
go run ./cmd/db-auth-lookup --help
```

Install remotely:

```bash
go install github.com/ywhywl/gdbtools/cmd/mysqlcompare@latest
```

Download this module into the local Go module cache:

```bash
go get github.com/ywhywl/gdbtools@latest
```

Detailed tool documentation is linked in the table above.
