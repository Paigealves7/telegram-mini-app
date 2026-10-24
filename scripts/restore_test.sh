#!/usr/bin/env bash
# Проверка, что бэкап реально восстанавливается: разворачивает последний файл во ВРЕМЕННУЮ базу
# (рабочая база не трогается), показывает число строк в таблицах и удаляет временную базу.
# Рекомендуется запускать раз в месяц и после любых изменений схемы.
set -Eeuo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "${BACKUP_ENV:-$DIR/backup.env}"
: "${BACKUP_DIR:=/var/backups/sawmill}"
: "${DB_CONTAINER:=sawmill_db}"
: "${DB_USER:=postgres}"

FILE="${1:-$(ls -1t "$BACKUP_DIR"/daily/*.dump 2>/dev/null | head -1)}"
[[ -n "$FILE" && -r "$FILE" ]] || { echo "Не найден файл бэкапа"; exit 1; }
TESTDB="sawmill_restore_test"
psqlc() { docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -v ON_ERROR_STOP=1 "$@"; }

echo "Файл: $FILE"
psqlc -d postgres -c "DROP DATABASE IF EXISTS $TESTDB"
psqlc -d postgres -c "CREATE DATABASE $TESTDB"
trap 'psqlc -d postgres -c "DROP DATABASE IF EXISTS '"$TESTDB"'" >/dev/null 2>&1 || true' EXIT

docker exec -i "$DB_CONTAINER" pg_restore -U "$DB_USER" -d "$TESTDB" --no-owner --exit-on-error < "$FILE"

echo "Строк в таблицах восстановленной копии:"
COUNTS="$(psqlc -d "$TESTDB" -At <<'SQL'
SELECT format('SELECT %L AS tbl, count(*) FROM %I', table_name, table_name)
FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name
\gexec
SQL
)"
echo "$COUNTS" | sed 's/|/: /' | sed 's/^/  /'

USERS="$(echo "$COUNTS" | awk -F'|' '$1=="users"{print $2}')"
[[ -n "$USERS" && "$USERS" -gt 0 ]] || { echo "ОШИБКА: в копии нет пользователей"; exit 1; }
echo "OK: бэкап восстанавливается. Временная база удалена."