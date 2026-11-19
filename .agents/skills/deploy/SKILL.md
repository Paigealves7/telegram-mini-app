---
name: deploy
description: Быстрый и экономный по токенам коммит, пуш и деплой проекта на сервер sawmill по SSH.
---
# Deploy Skill
Когда пользователь вызывает `/deploy`:
1. Если менялся Go-код, проверь сборку командой `go build ./...`.
2. Выполни локально: `git add . && git commit -m "<краткое описание изменений на русском>" && git push`.
3. Выполни на сервере в тихом режиме (чтобы не засорять контекст логами сборки):
   `ssh sawmill "cd ~/apps/telegram-mini-app && git pull && docker compose up -d --build --quiet-pull >/dev/null 2>&1 && docker ps --filter name=sawmill"`
   (Если была создана новая SQL-миграция, перед `docker compose` выполни её через `docker exec -i sawmill_db psql -U postgres -d telegram-mini-app_db < migrations/0000XX_...sql`).
4. Кратко сообщи в 2 строках, что деплой завершён.
