#!/usr/bin/env bash
# Ежедневный бэкап БД «Лесопилки». Запускается из cron.
# Настройки читаются из backup.env рядом со скриптом (шаблон: backup.env.example).
set -Eeuo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "${BACKUP_ENV:-$DIR/backup.env}"

: "${BACKUP_DIR:=/var/backups/sawmill}"
: "${DB_CONTAINER:=sawmill_db}"
: "${DB_USER:=postgres}"
: "${DB_NAME:=telegram-mini-app_db}"
: "${KEEP_DAILY_DAYS:=14}"
: "${KEEP_WEEKLY_DAYS:=60}"
: "${KEEP_MONTHLY_DAYS:=400}"
: "${KEEP_REMOTE_DAYS:=30}"

# Токен бота берём из .env приложения, чтобы не хранить его второй раз
TG_TOKEN=""
if [[ -n "${APP_ENV_FILE:-}" && -r "$APP_ENV_FILE" ]]; then
  TG_TOKEN="$(grep -E '^TELEGRAM_BOT_TOKEN=' "$APP_ENV_FILE" | head -1 | cut -d= -f2- | tr -d "\"'\r" || true)"
fi

notify() {
  [[ -n "${TG_CHAT_ID:-}" && -n "$TG_TOKEN" ]] || return 0
  curl -fsS --max-time 15 -X POST "https://api.telegram.org/bot${TG_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TG_CHAT_ID}" --data-urlencode "text=$1" >/dev/null 2>&1 \
    || echo "не удалось отправить уведомление в Telegram" >&2
}

die() {
  echo "ОШИБКА: $*" >&2
  notify "❌ Бэкап Лесопилки не выполнен: $*"
  trap - ERR
  exit 1
}

TMP=""
trap 'rm -f "${TMP:-}"' EXIT
trap 'rc=$?; notify "❌ Бэкап Лесопилки не выполнен (код $rc, строка $LINENO). Смотрите лог на сервере."; exit $rc' ERR

STAMP="$(date +%F_%H%M)"
mkdir -p "$BACKUP_DIR"/{daily,weekly,monthly}
chmod 700 "$BACKUP_DIR"

AVAIL="$(df --output=avail -B1 "$BACKUP_DIR" | tail -1 | tr -d ' ')"
(( AVAIL > 524288000 )) || die "на диске меньше 500 МБ свободного места"

OUT="$BACKUP_DIR/daily/sawmill_${STAMP}.dump"
TMP="$OUT.partial"

# custom-формат: сжатый, восстанавливается через pg_restore (в том числе выборочно)
docker exec "$DB_CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" --format=custom --compress=9 --no-owner > "$TMP"

# Проверка: файл не пустой и pg_restore может прочитать его оглавление
SIZE_B="$(stat -c%s "$TMP")"
(( SIZE_B > 2048 )) || die "файл бэкапа подозрительно мал ($SIZE_B байт)"
docker exec -i "$DB_CONTAINER" pg_restore --list < "$TMP" >/dev/null || die "pg_restore не смог прочитать созданный файл"

mv "$TMP" "$OUT"; TMP=""
chmod 600 "$OUT"

# Недельные и месячные копии
[[ "$(date +%u)" == "7" ]]  && cp -p "$OUT" "$BACKUP_DIR/weekly/"
[[ "$(date +%d)" == "01" ]] && cp -p "$OUT" "$BACKUP_DIR/monthly/"

# Удаление старого
find "$BACKUP_DIR/daily"   -name '*.dump' -mtime +"$KEEP_DAILY_DAYS"   -delete
find "$BACKUP_DIR/weekly"  -name '*.dump' -mtime +"$KEEP_WEEKLY_DAYS"  -delete
find "$BACKUP_DIR/monthly" -name '*.dump' -mtime +"$KEEP_MONTHLY_DAYS" -delete

# Копия вне сервера
if [[ -n "${RCLONE_REMOTE:-}" ]]; then
  command -v rclone >/dev/null || die "RCLONE_REMOTE задан, но rclone не установлен"
  rclone copy "$OUT" "${RCLONE_REMOTE}/daily/" --quiet || die "не удалось скопировать бэкап в $RCLONE_REMOTE"
  rclone delete "${RCLONE_REMOTE}/daily/" --min-age "${KEEP_REMOTE_DAYS}d" --quiet || true
fi

if [[ "${TG_SEND_FILE:-0}" == "1" && -n "${TG_CHAT_ID:-}" && -n "$TG_TOKEN" ]]; then
  command -v gpg >/dev/null || die "TG_SEND_FILE=1, но gpg не установлен"
  [[ -r "${GPG_PASSPHRASE_FILE:-}" ]] || die "не найден файл с паролем шифрования GPG_PASSPHRASE_FILE"
  ENC="$OUT.gpg"
  gpg --batch --yes --pinentry-mode loopback --passphrase-file "$GPG_PASSPHRASE_FILE" \
      --symmetric --cipher-algo AES256 -o "$ENC" "$OUT"
  if (( $(stat -c%s "$ENC") < 45000000 )); then
    curl -fsS --max-time 180 -F "chat_id=${TG_CHAT_ID}" -F "document=@${ENC};filename=sawmill_${STAMP}.dump.gpg" \
      -F "caption=🔐 Зашифрованный бэкап Лесопилки" \
      "https://api.telegram.org/bot${TG_TOKEN}/sendDocument" >/dev/null 2>&1 \
      || echo "не удалось отправить файл в Telegram" >&2
  else
    notify "⚠️ Бэкап слишком велик для отправки в Telegram, используйте RCLONE_REMOTE"
  fi
  rm -f "$ENC"
fi

SIZE_H="$(du -h "$OUT" | cut -f1)"
COUNT="$(find "$BACKUP_DIR/daily" -name '*.dump' | wc -l | tr -d ' ')"
echo "[$(date '+%F %T')] OK: $OUT ($SIZE_H), копий в daily: $COUNT"
notify "✅ Бэкап Лесопилки: $SIZE_H, копий на сервере: $COUNT"