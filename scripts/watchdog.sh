#!/usr/bin/env bash
# Сторож: раз в 5 минут проверяет, что сайт отвечает (nginx + приложение + база), и пишет в Telegram.
# Сообщает о падении после двух неудач подряд (~10 минут) и о восстановлении. Следит за местом на диске.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "${BACKUP_ENV:-$DIR/backup.env}"
: "${WATCH_URL:?Задайте WATCH_URL в backup.env}"
: "${DISK_WARN_PCT:=85}"
STATE="${STATE_FILE:-/var/tmp/sawmill-watchdog.state}"

TG_TOKEN=""
if [[ -n "${APP_ENV_FILE:-}" && -r "$APP_ENV_FILE" ]]; then
  TG_TOKEN="$(grep -E '^TELEGRAM_BOT_TOKEN=' "$APP_ENV_FILE" | head -1 | cut -d= -f2- | tr -d "\"'\r" || true)"
fi
notify() {
  [[ -n "${TG_CHAT_ID:-}" && -n "$TG_TOKEN" ]] || { echo "$1"; return 0; }
  curl -fsS --max-time 15 -X POST "https://api.telegram.org/bot${TG_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TG_CHAT_ID}" --data-urlencode "text=$1" >/dev/null 2>&1 || echo "не удалось отправить уведомление" >&2
}

fails="$(cat "$STATE" 2>/dev/null || echo 0)"
if curl -fsS --max-time 10 -o /dev/null "$WATCH_URL"; then
  (( fails >= 2 )) && notify "✅ Лесопилка снова доступна"
  echo 0 > "$STATE"
else
  fails=$((fails + 1)); echo "$fails" > "$STATE"
  if (( fails == 2 )); then
    CONT="$(docker ps -a --format '{{.Names}}: {{.Status}}' 2>/dev/null | grep -i sawmill | tr '\n' ';' || true)"
    notify "🔴 Лесопилка недоступна уже ~10 минут (${WATCH_URL}). Контейнеры: ${CONT:-не удалось получить}"
  fi
fi

# Место на диске (одно предупреждение в сутки)
pct="$(df --output=pcent / | tail -1 | tr -dc '0-9')"
MARK="/var/tmp/sawmill-disk-alert-$(date +%F)"
if [[ -n "$pct" ]] && (( pct >= DISK_WARN_PCT )) && [[ ! -e "$MARK" ]]; then
  : > "$MARK"
  notify "⚠️ Диск сервера заполнен на ${pct}%. Проверьте бэкапы, логи Docker и docker system df"
fi