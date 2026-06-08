#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"

usage() {
  cat <<'EOF'
Usage:
  bash scripts/mysql_migrate.sh \
    --source <host:port> \
    --source-schemas <schema1,schema2,...> \
    --target <host:port:schema[|host:port:schema...]> \
    [options]

Required:
  --source <host:port>
  --source-schemas <schema1,schema2,...>
  --target <targets>

Targets:
  A single --target value may contain multiple targets separated by newline, |, or ,
  and --target may be specified multiple times.

Options:
  --config <file>
  --mysqldump-bin <path>
  --mysql-bin <path>
  --ssh-bin <path>
  --scp-bin <path>
  --source-login-path <name>
  --target-login-path <name>
  --defaults-file <file>
  --source-defaults-file <file>
  --target-defaults-file <file>
  --with-data
  --data-tables <table1,table2,...>
  --dump-data-with-drop
  --structure-with-drop
  --no-create-schema
  --create-schema-if-missing
  --source-relay <host>
  --target-relay <host>
  --source-relay-user <user>
  --target-relay-user <user>
  --relay-tmp-dir <dir>
  --compress-threshold-mb <num>
  --compress-cmd <cmd>
  --decompress-cmd <cmd>
  --dry-run
  --verbose
  --keep-dump-files
  --cleanup
  --help
EOF
}

SOURCE=""
SOURCE_SCHEMAS=""
TARGETS=""
CONFIG_FILE=""

MYSQLDUMP_BIN="mysqldump"
MYSQL_BIN="mysql"
SSH_BIN="ssh"
SCP_BIN="scp"

SOURCE_LOGIN_PATH=""
TARGET_LOGIN_PATH=""
DEFAULTS_FILE=""
SOURCE_DEFAULTS_FILE=""
TARGET_DEFAULTS_FILE=""

WITH_DATA="false"
DATA_TABLES=""
DUMP_DATA_WITH_DROP="false"
STRUCTURE_WITH_DROP="false"
CREATE_SCHEMA_IF_MISSING="true"

SOURCE_RELAY=""
TARGET_RELAY=""
SOURCE_RELAY_USER=""
TARGET_RELAY_USER=""
RELAY_TMP_DIR="/tmp/mysql_migrate"
COMPRESS_THRESHOLD_MB="50"
COMPRESS_CMD="gzip"
DECOMPRESS_CMD="gzip -dc"

DRY_RUN="false"
VERBOSE="false"
KEEP_DUMP_FILES="false"
CLEANUP="true"

LOCAL_TMP_DIR=""
LOCAL_CACHE_DIR=""

SOURCE_HOST=""
SOURCE_PORT=""

declare -a RAW_TARGET_ARGS=()
declare -a SOURCE_SCHEMA_LIST=()
declare -a DATA_TABLE_LIST=()
declare -a TARGET_HOSTS=()
declare -a TARGET_PORTS=()
declare -a TARGET_SCHEMAS=()
declare -a STRUCTURE_REMOTE_PATHS=()
declare -a DATA_REMOTE_PATHS=()
declare -a STRUCTURE_LOCAL_PATHS=()
declare -a DATA_LOCAL_PATHS=()

log() {
  printf '[INFO] %s\n' "$*" >&2
}

debug() {
  if [[ "$VERBOSE" == "true" ]]; then
    printf '[DEBUG] %s\n' "$*" >&2
  fi
}

fail() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

trim() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

bool_is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

sanitize_name() {
  printf '%s' "$1" | tr -c 'A-Za-z0-9._-' '_'
}

shell_quote() {
  printf '%q' "$1"
}

join_quoted() {
  local out=""
  local item
  for item in "$@"; do
    if [[ -n "$out" ]]; then
      out+=" "
    fi
    out+="$(shell_quote "$item")"
  done
  printf '%s' "$out"
}

parse_host_port() {
  local value="$1"
  local host="${value%%:*}"
  local port="${value##*:}"
  [[ -n "$host" && -n "$port" && "$host" != "$port" ]] || return 1
  [[ "$port" =~ ^[0-9]+$ ]] || return 1
  printf '%s\n%s\n' "$host" "$port"
}

split_csv() {
  local value="$1"
  local normalized="${value//$'\n'/,}"
  local __out_var="$2"
  local old_ifs="$IFS"
  local -a __tmp=()
  local idx=0
  IFS=',' read -r -a __tmp <<< "$normalized"
  IFS="$old_ifs"
  for ((idx = 0; idx < ${#__tmp[@]}; idx++)); do
    __tmp[$idx]="$(trim "${__tmp[$idx]}")"
  done
  eval "$__out_var=()"
  for ((idx = 0; idx < ${#__tmp[@]}; idx++)); do
    eval "$__out_var+=(\"\${__tmp[$idx]}\")"
  done
}

split_targets() {
  local raw=""
  local normalized=""
  local -a parts=()
  local part=""
  for raw in "${RAW_TARGET_ARGS[@]}"; do
    normalized="${raw//$'\n'/,}"
    normalized="${normalized//|/,}"
    split_csv "$normalized" parts
    for part in "${parts[@]}"; do
      part="$(trim "$part")"
      [[ -n "$part" ]] || continue
      parse_target_spec "$part"
    done
  done
}

parse_target_spec() {
  local spec="$1"
  local first="${spec%%:*}"
  local remain="${spec#*:}"
  local second="${remain%%:*}"
  local schema="${remain#*:}"
  [[ -n "$first" && -n "$second" && -n "$schema" && "$remain" != "$schema" ]] || fail "invalid target format: $spec"
  [[ "$second" =~ ^[0-9]+$ ]] || fail "invalid target port in: $spec"
  TARGET_HOSTS+=("$first")
  TARGET_PORTS+=("$second")
  TARGET_SCHEMAS+=("$schema")
}

load_config_file() {
  local config="$1"
  [[ -f "$config" ]] || fail "config file not found: $config"
  # shellcheck disable=SC1090
  source "$config"
}

preload_config_from_args() {
  local args=("$@")
  local i=0
  while [[ $i -lt ${#args[@]} ]]; do
    case "${args[$i]}" in
      --config)
        ((i + 1 < ${#args[@]})) || fail "--config requires a value"
        load_config_file "${args[$((i + 1))]}"
        return 0
        ;;
    esac
    ((i += 1))
  done
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        [[ $# -ge 2 ]] || fail "--config requires a value"
        CONFIG_FILE="$2"
        shift 2
        ;;
      --source)
        [[ $# -ge 2 ]] || fail "--source requires a value"
        SOURCE="$2"
        shift 2
        ;;
      --source-schemas)
        [[ $# -ge 2 ]] || fail "--source-schemas requires a value"
        SOURCE_SCHEMAS="$2"
        shift 2
        ;;
      --target)
        [[ $# -ge 2 ]] || fail "--target requires a value"
        RAW_TARGET_ARGS+=("$2")
        shift 2
        ;;
      --mysqldump-bin)
        MYSQLDUMP_BIN="$2"
        shift 2
        ;;
      --mysql-bin)
        MYSQL_BIN="$2"
        shift 2
        ;;
      --ssh-bin)
        SSH_BIN="$2"
        shift 2
        ;;
      --scp-bin)
        SCP_BIN="$2"
        shift 2
        ;;
      --source-login-path)
        SOURCE_LOGIN_PATH="$2"
        shift 2
        ;;
      --target-login-path)
        TARGET_LOGIN_PATH="$2"
        shift 2
        ;;
      --defaults-file)
        DEFAULTS_FILE="$2"
        shift 2
        ;;
      --source-defaults-file)
        SOURCE_DEFAULTS_FILE="$2"
        shift 2
        ;;
      --target-defaults-file)
        TARGET_DEFAULTS_FILE="$2"
        shift 2
        ;;
      --with-data)
        WITH_DATA="true"
        shift
        ;;
      --data-tables)
        [[ $# -ge 2 ]] || fail "--data-tables requires a value"
        DATA_TABLES="$2"
        shift 2
        ;;
      --dump-data-with-drop)
        DUMP_DATA_WITH_DROP="true"
        shift
        ;;
      --structure-with-drop)
        STRUCTURE_WITH_DROP="true"
        shift
        ;;
      --no-create-schema)
        CREATE_SCHEMA_IF_MISSING="false"
        shift
        ;;
      --create-schema-if-missing)
        CREATE_SCHEMA_IF_MISSING="true"
        shift
        ;;
      --source-relay)
        SOURCE_RELAY="$2"
        shift 2
        ;;
      --target-relay)
        TARGET_RELAY="$2"
        shift 2
        ;;
      --source-relay-user)
        SOURCE_RELAY_USER="$2"
        shift 2
        ;;
      --target-relay-user)
        TARGET_RELAY_USER="$2"
        shift 2
        ;;
      --relay-tmp-dir)
        RELAY_TMP_DIR="$2"
        shift 2
        ;;
      --compress-threshold-mb)
        COMPRESS_THRESHOLD_MB="$2"
        shift 2
        ;;
      --compress-cmd)
        COMPRESS_CMD="$2"
        shift 2
        ;;
      --decompress-cmd)
        DECOMPRESS_CMD="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN="true"
        shift
        ;;
      --verbose)
        VERBOSE="true"
        shift
        ;;
      --keep-dump-files)
        KEEP_DUMP_FILES="true"
        shift
        ;;
      --cleanup)
        CLEANUP="true"
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done
}

apply_config_defaults() {
  SOURCE="${SOURCE:-${SOURCE_ADDR:-}}"
  SOURCE_SCHEMAS="${SOURCE_SCHEMAS:-${SOURCE_SCHEMAS_LIST:-}}"

  if [[ ${#RAW_TARGET_ARGS[@]} -eq 0 && -n "${TARGETS:-}" ]]; then
    RAW_TARGET_ARGS+=("$TARGETS")
  fi
}

validate_required_commands() {
  command -v "$MYSQLDUMP_BIN" >/dev/null 2>&1 || fail "mysqldump not found: $MYSQLDUMP_BIN"
  command -v "$MYSQL_BIN" >/dev/null 2>&1 || fail "mysql not found: $MYSQL_BIN"
  command -v "$SSH_BIN" >/dev/null 2>&1 || fail "ssh not found: $SSH_BIN"
  command -v "$SCP_BIN" >/dev/null 2>&1 || fail "scp not found: $SCP_BIN"
}

validate_inputs() {
  local source_parts_text=""
  local src_schema=""

  [[ -n "$SOURCE" ]] || fail "--source is required"
  [[ -n "$SOURCE_SCHEMAS" ]] || fail "--source-schemas is required"
  [[ ${#RAW_TARGET_ARGS[@]} -gt 0 || -n "${TARGETS:-}" ]] || fail "--target is required"
  [[ "$COMPRESS_THRESHOLD_MB" =~ ^[0-9]+$ ]] || fail "--compress-threshold-mb must be an integer"

  source_parts_text="$(parse_host_port "$SOURCE")" || fail "invalid --source format: $SOURCE"
  SOURCE_HOST="${source_parts_text%%$'\n'*}"
  SOURCE_PORT="${source_parts_text#*$'\n'}"

  split_csv "$SOURCE_SCHEMAS" SOURCE_SCHEMA_LIST
  for src_schema in "${SOURCE_SCHEMA_LIST[@]}"; do
    [[ -n "$src_schema" ]] && break
  done
  [[ -n "${src_schema:-}" ]] || fail "--source-schemas is empty"

  if [[ -n "$DATA_TABLES" ]]; then
    split_csv "$DATA_TABLES" DATA_TABLE_LIST
  else
    DATA_TABLE_LIST=()
  fi

  if [[ ${#RAW_TARGET_ARGS[@]} -eq 0 && -n "${TARGETS:-}" ]]; then
    RAW_TARGET_ARGS+=("${TARGETS}")
  fi
  split_targets
  [[ ${#TARGET_HOSTS[@]} -gt 0 ]] || fail "no valid targets parsed from --target"

  validate_schema_mapping
}

validate_schema_mapping() {
  local src_count="${#SOURCE_SCHEMA_LIST[@]}"
  local idx=0
  local schema=""
  local found="false"
  local src_schema=""

  if [[ "$src_count" -gt 1 ]]; then
    for ((idx = 0; idx < ${#TARGET_SCHEMAS[@]}; idx++)); do
      schema="${TARGET_SCHEMAS[$idx]}"
      found="false"
      for src_schema in "${SOURCE_SCHEMA_LIST[@]}"; do
        if [[ "$schema" == "$src_schema" ]]; then
          found="true"
          break
        fi
      done
      if [[ "$found" != "true" ]]; then
        fail "target schema $schema does not match any source schema; multiple source schemas only support same-name targets"
      fi
    done
    return 0
  fi
}

make_local_tmp() {
  LOCAL_TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mysql_migrate.XXXXXX")"
  LOCAL_CACHE_DIR="$LOCAL_TMP_DIR/cache"
  mkdir -p "$LOCAL_CACHE_DIR"
}

cleanup_all() {
  if [[ "$KEEP_DUMP_FILES" == "true" ]]; then
    debug "skip cleanup because --keep-dump-files is enabled"
    return 0
  fi

  if [[ "$CLEANUP" == "true" && -n "$LOCAL_TMP_DIR" && -d "$LOCAL_TMP_DIR" ]]; then
    rm -rf "$LOCAL_TMP_DIR"
  fi
}

trap cleanup_all EXIT

is_local_relay() {
  [[ -z "$1" ]]
}

remote_spec() {
  local user="$1"
  local host="$2"
  if [[ -n "$user" ]]; then
    printf '%s@%s' "$user" "$host"
  else
    printf '%s' "$host"
  fi
}

run_cmd() {
  local cmd="$1"
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[DRYRUN] %s\n' "$cmd" >&2
    return 0
  fi
  debug "run: $cmd"
  bash -lc "$cmd"
}

run_remote_cmd() {
  local relay_host="$1"
  local relay_user="$2"
  local cmd="$3"
  local remote
  remote="$(remote_spec "$relay_user" "$relay_host")"
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[DRYRUN] %s %s %s\n' "$SSH_BIN" "$remote" "$(shell_quote "$cmd")" >&2
    return 0
  fi
  debug "remote run on $remote: $cmd"
  "$SSH_BIN" "$remote" "bash -lc $(shell_quote "$cmd")"
}

capture_remote_cmd() {
  local relay_host="$1"
  local relay_user="$2"
  local cmd="$3"
  local remote
  remote="$(remote_spec "$relay_user" "$relay_host")"
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[DRYRUN] %s %s %s\n' "$SSH_BIN" "$remote" "$(shell_quote "$cmd")" >&2
    return 0
  fi
  debug "remote capture on $remote: $cmd"
  "$SSH_BIN" "$remote" "bash -lc $(shell_quote "$cmd")"
}

copy_from_remote() {
  local relay_host="$1"
  local relay_user="$2"
  local remote_path="$3"
  local local_path="$4"
  local remote
  remote="$(remote_spec "$relay_user" "$relay_host")"
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[DRYRUN] %s %s:%s %s\n' "$SCP_BIN" "$remote" "$remote_path" "$local_path" >&2
    return 0
  fi
  "$SCP_BIN" "$remote:$remote_path" "$local_path"
}

copy_to_remote() {
  local local_path="$1"
  local relay_host="$2"
  local relay_user="$3"
  local remote_path="$4"
  local remote
  remote="$(remote_spec "$relay_user" "$relay_host")"
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[DRYRUN] %s %s %s:%s\n' "$SCP_BIN" "$local_path" "$remote" "$remote_path" >&2
    return 0
  fi
  "$SCP_BIN" "$local_path" "$remote:$remote_path"
}

build_dump_auth_args() {
  local side="$1"
  local __out_var="$2"
  eval "$__out_var=()"
  if [[ "$side" == "source" ]]; then
    if [[ -n "$SOURCE_LOGIN_PATH" ]]; then
      eval "$__out_var+=(\"--login-path=$SOURCE_LOGIN_PATH\")"
    elif [[ -n "$SOURCE_DEFAULTS_FILE" ]]; then
      eval "$__out_var+=(\"--defaults-file=$SOURCE_DEFAULTS_FILE\")"
    elif [[ -n "$DEFAULTS_FILE" ]]; then
      eval "$__out_var+=(\"--defaults-file=$DEFAULTS_FILE\")"
    fi
  else
    if [[ -n "$TARGET_LOGIN_PATH" ]]; then
      eval "$__out_var+=(\"--login-path=$TARGET_LOGIN_PATH\")"
    elif [[ -n "$TARGET_DEFAULTS_FILE" ]]; then
      eval "$__out_var+=(\"--defaults-file=$TARGET_DEFAULTS_FILE\")"
    elif [[ -n "$DEFAULTS_FILE" ]]; then
      eval "$__out_var+=(\"--defaults-file=$DEFAULTS_FILE\")"
    fi
  fi
}

mysql_ident() {
  local ident="$1"
  ident="${ident//\`/\`\`}"
  printf '`%s`' "$ident"
}

ensure_remote_dir() {
  local relay_host="$1"
  local relay_user="$2"
  local dir="$3"
  local cmd="mkdir -p $(shell_quote "$dir")"
  if is_local_relay "$relay_host"; then
    run_cmd "$cmd"
  else
    run_remote_cmd "$relay_host" "$relay_user" "$cmd"
  fi
}

capture_local_or_remote() {
  local relay_host="$1"
  local relay_user="$2"
  local cmd="$3"
  if is_local_relay "$relay_host"; then
    if [[ "$DRY_RUN" == "true" ]]; then
      printf '[DRYRUN] %s\n' "$cmd" >&2
      return 0
    fi
    bash -lc "$cmd"
  else
    capture_remote_cmd "$relay_host" "$relay_user" "$cmd"
  fi
}

run_local_or_remote() {
  local relay_host="$1"
  local relay_user="$2"
  local cmd="$3"
  if is_local_relay "$relay_host"; then
    run_cmd "$cmd"
  else
    run_remote_cmd "$relay_host" "$relay_user" "$cmd"
  fi
}

fetch_remote_or_local_file() {
  local relay_host="$1"
  local relay_user="$2"
  local source_path="$3"
  local local_path="$4"
  if is_local_relay "$relay_host"; then
    if [[ "$DRY_RUN" == "true" ]]; then
      printf '[DRYRUN] cp %s %s\n' "$source_path" "$local_path" >&2
      return 0
    fi
    cp "$source_path" "$local_path"
  else
    copy_from_remote "$relay_host" "$relay_user" "$source_path" "$local_path"
  fi
}

stage_file_to_target_relay() {
  local local_path="$1"
  local relay_host="$2"
  local relay_user="$3"
  local remote_path="$4"
  if is_local_relay "$relay_host"; then
    if [[ "$DRY_RUN" == "true" ]]; then
      printf '[DRYRUN] cp %s %s\n' "$local_path" "$remote_path" >&2
      return 0
    fi
    cp "$local_path" "$remote_path"
  else
    copy_to_remote "$local_path" "$relay_host" "$relay_user" "$remote_path"
  fi
}

build_structure_dump_command() {
  local schema="$1"
  local output_path="$2"
  local -a cmd=()
  local -a auth=()
  build_dump_auth_args "source" auth
  cmd=("$MYSQLDUMP_BIN")
  if [[ ${#auth[@]} -gt 0 ]]; then
    cmd+=("${auth[@]}")
  fi
  cmd+=("--host=$SOURCE_HOST" "--port=$SOURCE_PORT" "$schema" "--no-data" "--skip-comments" "--skip-set-charset" "--skip-triggers")
  if [[ "$STRUCTURE_WITH_DROP" == "true" ]]; then
    cmd+=("--add-drop-table")
  else
    cmd+=("--skip-add-drop-table")
  fi
  printf '%s > %s' "$(join_quoted "${cmd[@]}")" "$(shell_quote "$output_path")"
}

build_data_dump_command() {
  local schema="$1"
  local output_path="$2"
  local -a cmd=()
  local -a auth=()
  local table=""
  build_dump_auth_args "source" auth
  cmd=("$MYSQLDUMP_BIN")
  if [[ ${#auth[@]} -gt 0 ]]; then
    cmd+=("${auth[@]}")
  fi
  cmd+=("--host=$SOURCE_HOST" "--port=$SOURCE_PORT" "--skip-comments" "--skip-set-charset")

  if [[ "$DUMP_DATA_WITH_DROP" == "true" ]]; then
    cmd+=("--add-drop-table")
  else
    cmd+=("--skip-add-drop-table" "--no-create-info")
  fi

  cmd+=("$schema")
  if [[ ${#DATA_TABLE_LIST[@]} -gt 0 ]]; then
    for table in "${DATA_TABLE_LIST[@]}"; do
      [[ -n "$table" ]] || continue
      cmd+=("$table")
    done
  fi

  printf '%s > %s' "$(join_quoted "${cmd[@]}")" "$(shell_quote "$output_path")"
}

compress_if_needed_command() {
  local file_path="$1"
  local threshold_bytes=$((COMPRESS_THRESHOLD_MB * 1024 * 1024))
  cat <<EOF
set -e
file=$(shell_quote "$file_path")
size=\$(wc -c < "\$file")
if [ "\$size" -gt $threshold_bytes ]; then
  $COMPRESS_CMD -f -- "\$file"
  printf '%s\n' "\$file.gz"
else
  printf '%s\n' "\$file"
fi
EOF
}

export_schema_artifacts() {
  local schema="$1"
  local safe_schema
  local base_name
  local remote_base
  local structure_dump_cmd
  local data_dump_cmd
  local structure_path
  local data_path
  local final_structure_path
  local final_data_path=""
  safe_schema="$(sanitize_name "$schema")"
  base_name="${safe_schema}_$(date +%Y%m%d%H%M%S)_$$"
  remote_base="$RELAY_TMP_DIR/$base_name"
  structure_path="${remote_base}_structure.sql"
  data_path="${remote_base}_data.sql"

  ensure_remote_dir "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$RELAY_TMP_DIR"

  structure_dump_cmd="$(build_structure_dump_command "$schema" "$structure_path")"
  log "exporting structure for source schema $schema"
  run_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$structure_dump_cmd"
  if [[ "$DRY_RUN" == "true" ]]; then
    capture_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$(compress_if_needed_command "$structure_path")" >/dev/null
    final_structure_path="$structure_path"
  else
    final_structure_path="$(capture_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$(compress_if_needed_command "$structure_path")" | tail -n 1)"
  fi

  if [[ "$WITH_DATA" == "true" ]]; then
    data_dump_cmd="$(build_data_dump_command "$schema" "$data_path")"
    log "exporting data for source schema $schema"
    run_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$data_dump_cmd"
    if [[ "$DRY_RUN" == "true" ]]; then
      capture_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$(compress_if_needed_command "$data_path")" >/dev/null
      final_data_path="$data_path"
    else
      final_data_path="$(capture_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$(compress_if_needed_command "$data_path")" | tail -n 1)"
    fi
  fi

  STRUCTURE_EXPORT_RESULT="$final_structure_path"
  DATA_EXPORT_RESULT="$final_data_path"
}

import_sql_command() {
  local target_host="$1"
  local target_port="$2"
  local target_schema="$3"
  local input_path="$4"
  local -a cmd=()
  local -a auth=()
  build_dump_auth_args "target" auth
  cmd=("$MYSQL_BIN")
  if [[ ${#auth[@]} -gt 0 ]]; then
    cmd+=("${auth[@]}")
  fi
  cmd+=("--host=$target_host" "--port=$target_port" "$target_schema")

  if [[ "$input_path" == *.gz ]]; then
    printf '%s %s | %s' "$DECOMPRESS_CMD" "$(shell_quote "$input_path")" "$(join_quoted "${cmd[@]}")"
  else
    printf '%s < %s' "$(join_quoted "${cmd[@]}")" "$(shell_quote "$input_path")"
  fi
}

create_schema_command() {
  local target_host="$1"
  local target_port="$2"
  local target_schema="$3"
  local -a cmd=()
  local -a auth=()
  local ident
  ident="$(mysql_ident "$target_schema")"
  build_dump_auth_args "target" auth
  cmd=("$MYSQL_BIN")
  if [[ ${#auth[@]} -gt 0 ]]; then
    cmd+=("${auth[@]}")
  fi
  cmd+=("--host=$target_host" "--port=$target_port" "-e" "CREATE DATABASE IF NOT EXISTS $ident")
  join_quoted "${cmd[@]}"
}

target_remote_file_path() {
  local target_schema="$1"
  local local_file="$2"
  local filename
  filename="$(basename "$local_file")"
  printf '%s/%s_%s' "$RELAY_TMP_DIR" "$(sanitize_name "$target_schema")" "$filename"
}

cleanup_remote_file_command() {
  local path="$1"
  printf 'rm -f -- %s' "$(shell_quote "$path")"
}

map_target_to_source_schema() {
  local target_schema="$1"
  local src_count="${#SOURCE_SCHEMA_LIST[@]}"
  if [[ "$src_count" -eq 1 ]]; then
    printf '%s' "${SOURCE_SCHEMA_LIST[0]}"
    return 0
  fi
  printf '%s' "$target_schema"
}

source_schema_index() {
  local schema="$1"
  local idx=0
  for ((idx = 0; idx < ${#SOURCE_SCHEMA_LIST[@]}; idx++)); do
    if [[ "${SOURCE_SCHEMA_LIST[$idx]}" == "$schema" ]]; then
      printf '%s' "$idx"
      return 0
    fi
  done
  return 1
}

download_artifact_to_local() {
  local relay_host="$1"
  local relay_user="$2"
  local remote_path="$3"
  local local_path="$4"
  [[ -n "$remote_path" ]] || return 0
  if [[ -f "$local_path" ]]; then
    return 0
  fi
  fetch_remote_or_local_file "$relay_host" "$relay_user" "$remote_path" "$local_path"
}

perform_migration() {
  local schema=""
  local target_index=0
  local target_host=""
  local target_port=""
  local target_schema=""
  local source_schema=""
  local source_schema_idx=0
  local local_structure=""
  local local_data=""
  local remote_structure_on_target=""
  local remote_data_on_target=""
  local create_cmd=""
  local import_cmd=""
  local cleanup_cmd=""
  local STRUCTURE_EXPORT_RESULT=""
  local DATA_EXPORT_RESULT=""

  for schema in "${SOURCE_SCHEMA_LIST[@]}"; do
    source_schema_idx="$(source_schema_index "$schema")" || fail "source schema index not found: $schema"
    export_schema_artifacts "$schema"
    STRUCTURE_REMOTE_PATHS[$source_schema_idx]="$STRUCTURE_EXPORT_RESULT"
    DATA_REMOTE_PATHS[$source_schema_idx]="$DATA_EXPORT_RESULT"
    local_structure="$LOCAL_CACHE_DIR/$(basename "$STRUCTURE_EXPORT_RESULT")"
    STRUCTURE_LOCAL_PATHS[$source_schema_idx]="$local_structure"
    download_artifact_to_local "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$STRUCTURE_EXPORT_RESULT" "$local_structure"

    if [[ "$WITH_DATA" == "true" && -n "$DATA_EXPORT_RESULT" ]]; then
      local_data="$LOCAL_CACHE_DIR/$(basename "$DATA_EXPORT_RESULT")"
      DATA_LOCAL_PATHS[$source_schema_idx]="$local_data"
      download_artifact_to_local "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$DATA_EXPORT_RESULT" "$local_data"
    fi
  done

  for ((target_index = 0; target_index < ${#TARGET_HOSTS[@]}; target_index++)); do
    target_host="${TARGET_HOSTS[$target_index]}"
    target_port="${TARGET_PORTS[$target_index]}"
    target_schema="${TARGET_SCHEMAS[$target_index]}"
    source_schema="$(map_target_to_source_schema "$target_schema")"
    source_schema_idx="$(source_schema_index "$source_schema")" || fail "target schema $target_schema cannot map to source schema"
    local_structure="${STRUCTURE_LOCAL_PATHS[$source_schema_idx]}"
    local_data="${DATA_LOCAL_PATHS[$source_schema_idx]:-}"

    ensure_remote_dir "$TARGET_RELAY" "$TARGET_RELAY_USER" "$RELAY_TMP_DIR"

    if [[ "$CREATE_SCHEMA_IF_MISSING" == "true" ]]; then
      create_cmd="$(create_schema_command "$target_host" "$target_port" "$target_schema")"
      log "ensuring target schema $target_host:$target_port:$target_schema exists"
      run_local_or_remote "$TARGET_RELAY" "$TARGET_RELAY_USER" "$create_cmd"
    fi

    remote_structure_on_target="$(target_remote_file_path "$target_schema" "$local_structure")"
    stage_file_to_target_relay "$local_structure" "$TARGET_RELAY" "$TARGET_RELAY_USER" "$remote_structure_on_target"

    log "importing structure into target $target_host:$target_port:$target_schema"
    import_cmd="$(import_sql_command "$target_host" "$target_port" "$target_schema" "$remote_structure_on_target")"
    run_local_or_remote "$TARGET_RELAY" "$TARGET_RELAY_USER" "$import_cmd"

    if [[ "$WITH_DATA" == "true" && -n "$local_data" ]]; then
      remote_data_on_target="$(target_remote_file_path "$target_schema" "$local_data")"
      stage_file_to_target_relay "$local_data" "$TARGET_RELAY" "$TARGET_RELAY_USER" "$remote_data_on_target"
      log "importing data into target $target_host:$target_port:$target_schema"
      import_cmd="$(import_sql_command "$target_host" "$target_port" "$target_schema" "$remote_data_on_target")"
      run_local_or_remote "$TARGET_RELAY" "$TARGET_RELAY_USER" "$import_cmd"
    fi

    if [[ "$KEEP_DUMP_FILES" != "true" && "$CLEANUP" == "true" ]]; then
      run_local_or_remote "$TARGET_RELAY" "$TARGET_RELAY_USER" "$(cleanup_remote_file_command "$remote_structure_on_target")"
      if [[ -n "${remote_data_on_target:-}" ]]; then
        run_local_or_remote "$TARGET_RELAY" "$TARGET_RELAY_USER" "$(cleanup_remote_file_command "$remote_data_on_target")"
      fi
    fi
    remote_data_on_target=""
  done

  if [[ "$KEEP_DUMP_FILES" != "true" && "$CLEANUP" == "true" ]]; then
    for schema in "${SOURCE_SCHEMA_LIST[@]}"; do
      source_schema_idx="$(source_schema_index "$schema")" || fail "source schema index not found during cleanup: $schema"
      cleanup_cmd="$(cleanup_remote_file_command "${STRUCTURE_REMOTE_PATHS[$source_schema_idx]}")"
      run_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$cleanup_cmd"
      if [[ -n "${DATA_REMOTE_PATHS[$source_schema_idx]:-}" ]]; then
        cleanup_cmd="$(cleanup_remote_file_command "${DATA_REMOTE_PATHS[$source_schema_idx]}")"
        run_local_or_remote "$SOURCE_RELAY" "$SOURCE_RELAY_USER" "$cleanup_cmd"
      fi
    done
  fi
}

main() {
  preload_config_from_args "$@"
  parse_args "$@"
  apply_config_defaults
  validate_required_commands
  validate_inputs
  make_local_tmp
  perform_migration
  log "migration finished"
}

main "$@"
