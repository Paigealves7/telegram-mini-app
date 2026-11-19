---
name: feature
description: Пошаговое добавление новой функции (БД + Go Backend + Frontend в web/index.html).
---
# New Feature Skill
Когда пользователь просит добавить новую функцию:
1. Сверься с `AGENTS.md` и открой только нужные файлы.
2. Если нужна БД — создай миграцию в `migrations/` и пропиши её в `docker-compose.yml`.
3. Добавь структуры в `internal/domain/`, SQL-запросы в `internal/repository/`, хендлер и маршрут в `internal/transport/http/router.go`.
4. Обнови интерфейс в `web/index.html`.
5. Проверь компиляцию через `go build ./...` (тесты не запускай без просьбы).
