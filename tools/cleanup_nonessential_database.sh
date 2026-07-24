#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
SQL_FILE="$SCRIPT_DIR/sql/cleanup_nonessential_database.sql"

MODE="report"
CONNECTION_MODE="docker"
CONTAINER_NAME="${SUB2API_POSTGRES_CONTAINER:-sub2api-postgres}"
DB_HOST=""
DB_PORT="${PGPORT:-5432}"
DB_USER="${PGUSER:-sub2api}"
DB_NAME="${PGDATABASE:-sub2api}"
METADATA_RETENTION_DAYS=7
PURGE_OPS_LOGS=false
LOG_RETENTION_DAYS=30
VACUUM_FULL=false
ASSUME_YES=false

usage() {
  cat <<'EOF'
Usage: tools/cleanup_nonessential_database.sh [options]

Reports database growth by default. No data is changed unless --apply is used.

Connection options:
  --container NAME                 PostgreSQL container (default: sub2api-postgres)
  --host HOST                      Connect with local psql instead of Docker
  --port PORT                      PostgreSQL port for --host (default: 5432)
  --user USER                      PostgreSQL user (default: sub2api)
  --database NAME                  PostgreSQL database (default: sub2api)

Cleanup options:
  --apply                          Clear raw video payloads and expired metadata
  --metadata-retention-days DAYS   Keep expired/deleted metadata this long (default: 7)
  --purge-ops-logs                 Also remove old operations logs and metrics
  --log-retention-days DAYS        Operations data retention (default: 30)
  --vacuum-full                    Exclusively lock and compact video_tasks after cleanup
  --yes                            Skip the interactive database-name confirmation
  -h, --help                       Show this help

The script never deletes video task rows, usage/billing records, users, API keys,
groups, accounts, or pricing rules. Passwords are not
accepted as arguments; direct connections use normal psql authentication.
EOF
}

require_value() {
  if [[ $# -lt 2 || -z "$2" ]]; then
    echo "Missing value for $1" >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --container)
      require_value "$@"
      CONNECTION_MODE="docker"
      CONTAINER_NAME="$2"
      shift 2
      ;;
    --host)
      require_value "$@"
      CONNECTION_MODE="direct"
      DB_HOST="$2"
      shift 2
      ;;
    --port)
      require_value "$@"
      DB_PORT="$2"
      shift 2
      ;;
    --user)
      require_value "$@"
      DB_USER="$2"
      shift 2
      ;;
    --database)
      require_value "$@"
      DB_NAME="$2"
      shift 2
      ;;
    --metadata-retention-days)
      require_value "$@"
      METADATA_RETENTION_DAYS="$2"
      shift 2
      ;;
    --purge-ops-logs)
      PURGE_OPS_LOGS=true
      shift
      ;;
    --log-retention-days)
      require_value "$@"
      LOG_RETENTION_DAYS="$2"
      shift 2
      ;;
    --apply)
      MODE="apply"
      shift
      ;;
    --vacuum-full)
      VACUUM_FULL=true
      shift
      ;;
    --yes)
      ASSUME_YES=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! "$DB_PORT" =~ ^[0-9]+$ ]] || (( DB_PORT < 1 || DB_PORT > 65535 )); then
  echo "--port must be an integer between 1 and 65535" >&2
  exit 2
fi
if [[ ! "$METADATA_RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "--metadata-retention-days must be a non-negative integer" >&2
  exit 2
fi
if [[ ! "$LOG_RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "--log-retention-days must be a non-negative integer" >&2
  exit 2
fi
if [[ "$VACUUM_FULL" == true && "$MODE" != "apply" ]]; then
  echo "--vacuum-full requires --apply" >&2
  exit 2
fi
if [[ "$DB_NAME" == "postgres" || "$DB_NAME" == "template0" || "$DB_NAME" == "template1" ]]; then
  echo "Refusing to clean PostgreSQL system database: $DB_NAME" >&2
  exit 2
fi
if [[ ! -f "$SQL_FILE" ]]; then
  echo "Cleanup SQL not found: $SQL_FILE" >&2
  exit 1
fi
if [[ "$CONNECTION_MODE" == "docker" ]]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required for --container mode; use --host for direct psql" >&2
    exit 1
  fi
  if [[ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || true)" != "true" ]]; then
    echo "PostgreSQL container is not running: $CONTAINER_NAME" >&2
    exit 1
  fi
  TARGET="container $CONTAINER_NAME, database $DB_NAME"
else
  if ! command -v psql >/dev/null 2>&1; then
    echo "psql is required for --host mode" >&2
    exit 1
  fi
  TARGET="$DB_HOST:$DB_PORT/$DB_NAME"
fi

if [[ "$MODE" == "apply" ]]; then
  echo "Target: $TARGET"
  echo "This will clear raw video request/provider payloads and delete expired metadata."
  if [[ "$PURGE_OPS_LOGS" == true ]]; then
    echo "Operations logs and metrics older than $LOG_RETENTION_DAYS days will also be deleted."
  fi
  if [[ "$VACUUM_FULL" == true ]]; then
    echo "VACUUM FULL will exclusively lock video_tasks and may require substantial temporary disk space."
  fi
  if [[ "$ASSUME_YES" != true ]]; then
    if [[ ! -t 0 ]]; then
      echo "Interactive confirmation is unavailable; rerun with --yes after reviewing the report" >&2
      exit 2
    fi
    read -r -p "Type the database name '$DB_NAME' to continue: " confirmation
    if [[ "$confirmation" != "$DB_NAME" ]]; then
      echo "Confirmation did not match; no changes were made" >&2
      exit 2
    fi
  fi
else
  echo "Report only. Target: $TARGET"
fi

PSQL_OPTIONS=(
  -X
  --set=ON_ERROR_STOP=1
  --set="apply=$([[ "$MODE" == "apply" ]] && echo true || echo false)"
  --set="metadata_retention_days=$METADATA_RETENTION_DAYS"
  --set="purge_ops_logs=$PURGE_OPS_LOGS"
  --set="log_retention_days=$LOG_RETENTION_DAYS"
  --set="vacuum_full=$VACUUM_FULL"
  --username="$DB_USER"
  --dbname="$DB_NAME"
)

if [[ "$CONNECTION_MODE" == "docker" ]]; then
  docker exec -i "$CONTAINER_NAME" psql "${PSQL_OPTIONS[@]}" --file=- < "$SQL_FILE"
else
  psql "${PSQL_OPTIONS[@]}" --host="$DB_HOST" --port="$DB_PORT" --file="$SQL_FILE"
fi
