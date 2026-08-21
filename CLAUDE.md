# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**gdbtools** — a mixed Go/Python toolset for managing and auditing GoldenDB (a MySQL-distributed database). Go provides standalone CLI executables; Python provides schema/privilege comparison tooling.

## Build & Test Commands

```bash
# Go: build a single CLI
go build ./cmd/mysqlcompare          # produces ./mysqlcompare binary

# Go: run directly (no build)
go run ./cmd/mysqlcompare --help

# Go: run all tests
go test ./...

# Go: run tests for one package
go test ./internal/mysqlcompare/

# Go: run a single test
go test ./internal/mysqlcompare/ -run TestSpecificName

# Go: format code
gofmt -w .

# Python: install & test
pip install -e .
python -m unittest discover -s tests/

# Python: run a single test
python -m unittest tests.test_config
```

## Go Module

- Module: `github.com/ywhywl/gdbtools`
- Go version: 1.23.1
- Key dependencies: `go-sql-driver/mysql`, `xuri/excelize/v2`, `gopkg.in/yaml.v3`
- CLI parsing: standard library `flag` (no Cobra/urfave-cli)

## Architecture

### Directory Layout

```
cmd/                   Go CLI entrypoints (one dir per command, only main.go)
internal/              Go application logic per feature
pkg/goldendb/          Reusable Go client library (GoldenDB REST API)
src/gdbtools/          Python package implementation
tests/                 Python unit tests
docs/                  User-facing design & usage docs
scripts/               Standalone helper scripts
```

### Go CLI Structure

Every Go command follows the same pattern:
- `cmd/<tool>/main.go` — tiny: calls `<tool>.Run(os.Args[1:])`, prints errors, exits with returned code
- `internal/<tool>/` — feature logic split by responsibility:
  - `run.go` — flag parsing & orchestration
  - `types.go` — data structures
  - `client.go` — external client wrappers (MySQL, HTTP)
  - `collector.go` — data collection / query logic
  - `diff.go` / `rules.go` — comparison or rule evaluation
  - `report.go` — text & JSON rendering
  - `config.go` — config file parsing
  - `utils.go` — small helpers
  - `errors.go` — typed errors

### Available CLIs

| Command | Purpose |
|---------|---------|
| `mysqlcompare` | Compare MySQL source/target schema & privileges |
| `mysqlpricheck` | Audit MySQL 5.7 privileges |
| `db-auth-lookup` | Query DB auth from Excel/CSV mapping files |
| `insight-batch-onboard-hosts` | Batch onboard hosts into GoldenDB Insight |
| `insight-batch-create` | Batch create GoldenDB clusters |
| `insight-batch-add-cn` | Batch add CN nodes |
| `insight-batch-add-dn` | Batch add DN nodes |
| `insight-create-dbmgr` | Create & grant dbmgr admin user |

### Internal Packages

Most `internal/<tool>/` packages map one-to-one with a CLI command. Three are **shared** and reused across multiple Insight CLIs:

| Package | Purpose |
|---------|---------|
| `insightinput` | Tabular input parsing (CSV/Excel) shared by Insight batch tools |
| `insightonboard` | Host onboarding logic reused by batch-onboard |
| `insightopen` | GoldenDB Insight REST API client (auth, cluster, task, DB group ops) |

### Code Placement Decision Rules

| If it is... | Place it in... |
|-------------|----------------|
| Reusable client/library | `pkg/` |
| Internal logic for one Go command | `internal/<tool>/` |
| New Go executable | `cmd/<tool>/main.go` |
| Extension to Python compare tooling | `src/gdbtools/` |
| One-off operational helper | `scripts/` |
| User-facing docs | `docs/` |

## Python Structure

Python code lives under `src/gdbtools/` with modules split by responsibility:

| Module | Purpose |
|--------|---------|
| `cli.py` | CLI argument parsing & entry |
| `config.py` | Configuration file parsing |
| `collector.py` | Data collection logic |
| `diffing.py` | Schema/privilege comparison logic |
| `reporting.py` | Text & JSON rendering |
| `models.py` | Dataclasses & data models |
| `mysql_client.py` | MySQL connection helpers |
| `utils.py` | Small reusable helpers |

Tests use standard library `unittest` under `tests/`. Use `dataclasses` for configuration-style models.

## Conventions

- **CLI flags**: kebab-case (`--source-dsn`, `--exclude-users`)
- **JSON/config keys**: snake_case
- **Run signature**: `func Run(argv []string) (int, error)`
- **Exit codes**: 0 = success; non-zero = error
- **Output**: support both human-readable text and machine-parseable JSON
- **Error messages**: actionable, mention which flag/input is invalid
- **Go formatting**: `gofmt -w .`
- **Tests**: `*_test.go` next to Go packages; `unittest` for Python under `tests/`
- **No framework dependencies**: standard library first for both Go and Python

## Adding a New Go CLI

1. Create `cmd/<tool>/main.go` (minimal — just calls `<tool>.Run()`)
2. Create `internal/<tool>/` with `run.go`, `types.go`, and other files as needed
3. Add tests in `internal/<tool>/` as `<tool>_test.go`
4. Add docs under `docs/<tool>_usage.md`
5. Update README.md tool index table
6. Format with `gofmt`, run tests

See README.md for full code templates for new Go CLIs and Python features.
