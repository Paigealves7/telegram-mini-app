# AGENTS.md — Sawmill Telegram Mini App («Лесопилка»)

Ты — ведущий инженер проекта «Лесопилка» (учёт пилорамы: сырьё, распиловка, пиломатериалы, дрова, заказы, оплаты, долги и PDF-накладные).
Твоя главная задача — вносить точные изменения с **минимальным расходом токенов**.

---

## 1. ПРАВИЛА ЭКОНОМИИ ТОКЕНОВ (ЧИТАТЬ ОБЯЗАТЕЛЬНО)
1. **НЕ сканируй весь репозиторий.** Используй «Карту маршрутизации» ниже, чтобы открывать ТОЛЬКО те файлы, которые нужны для текущей задачи.
2. **Если задача касается ТОЛЬКО фронтенда / дизайна / UI / UX:**
   - **Весь фронтенд целиком лежит в ОДНОМ файле: `web/index.html`** (внутри него `<style>` и `<script>` на чистом Vanilla JS, без сборки, без React/Vite/npm).
   - **КАТЕГОРИЧЕСКИ ЗАПРЕЩЕНО** читать файлы в `internal/`, `cmd/`, `migrations/`, `pkg/` при выполнении UI-задач.
3. **Не читай `*_test.go` файлы** и `go.sum`, если я явно не попросил починить или написать тесты.
4. **Не запускай `docker compose build` или `ssh sawmill` без прямого указания** — логи сборки огромные и сжигают лимит токенов.
5. Отвечай кратко, по делу, на русском языке. Все тексты ошибокAPI и интерфейса — строго на русском языке.

---
5. В начале новой задачи прочитай PROJECT.md, чтобы понимать текущий статус проекта. После завершения крупной задачи кратко обнови PROJECT.md (но НЕ деплой в прод без моей команды /deploy или $deploy).

Скилл $autopilot — автономная оркестрация многофазных задач с изолированными субагентами. Инструкции: .agents/skills/autopilot/SKILL.md.

## 2. ТЕХНИЧЕСКИЙ СТЕК И АРХИТЕКТУРА
- **Backend:** Go (Golang), роутер `chi/v5`, пул БД `pgx/v5` (чистый SQL без ORM), валидация `validator/v10`, JWT-авторизация + Telegram `initData` (HMAC-SHA256), генерация PDF через `go-pdf/fpdf` (шрифты DejaVu).
- **Database:** PostgreSQL 15 (в Docker-контейнере `sawmill_db`, база `telegram-mini-app_db`). Миграции лежат в `migrations/000001_...` — `000009_...`.
- **Frontend:** Одиночный самодостаточный файл `web/index.html` (HTML5 + CSS Custom Properties + Vanilla JS SPA). Раздаётся напрямую Go-сервером из папки `./web`.
- **Роли пользователей (`domain.Role`):**
  - `manager` (Руководитель) — полный доступ (статистика, сотрудники, контрагенты, корректировки склада, коэффициенты дров, удаление оплат/переработок).
  - `master` (Мастер) — приходы (лес и дрова), продажи, заказы, оплаты, долги, PDF-накладные.
  - `worker` (Сотрудник) — распиловка леса (`/sawing`) и переработка дров (`/firewood/processing`).

---

## 3. КАРТА МАРШРУТИЗАЦИИ (ROUTING MAP)
Иди сразу по указанному пути, не делай глобальный поиск по проекту:

| Что нужно сделать | Куда идти (точные пути) | Правило |
| :--- | :--- | :--- |
| **UI / UX, верстка, стили, экраны, формы** | `web/index.html` | Единственный файл фронтенда! Вверху `<style>`, внизу `<script>`. |
| **Роуты API, права ролей, лимиты** | `internal/transport/http/router.go` | Все эндпоинты `/api/v1/...` и группы `RequireRoles(...)` описаны здесь. |
| **Структуры данных (модели)** | `internal/domain/entities.go`<br>`internal/domain/firewood.go`<br>`internal/domain/money.go`<br>`internal/domain/order_pdf.go` | Все основные структуры и округление денег/объёмов (`Round2`, `LineTotal`, `ItemVolume`). |
| **Дрова (приход, переработка, коэффициенты, остатки)** | Хендлеры: `internal/transport/http/firewood_handler.go`, `firewood_processing_handler.go`<br>БД: `internal/repository/firewood.go`, `firewood_processing.go`, `firewood_orders.go` | Виды: `split` (колотые, коэф. 0.55) и `round` (чурбаки/пиленые, коэф. 0.60). Блокировка `pg_advisory_xact_lock(900009)`. |
| **Заказы (доски + дрова) и их завершение** | Хендлер: `internal/transport/http/order_handler.go`<br>Сервис: `internal/service/order_service.go`<br>БД: `internal/repository/postgres.go` (`CreateOrder`, `CompleteOrder`, `GetOrders`) | Завершение заказа (`CompleteOrder`) в одной транзакции списывает склад и создаёт продажу. |
| **Оплаты и Долги** | Хендлер: `internal/transport/http/payment_handler.go`<br>БД: `internal/repository/payments.go` | Сумма заказа считается через `orderTotalExpr` (доски + дрова + доставка + допы). |
| **PDF-накладные и отправка в бот** | Хендлер: `internal/transport/http/pdf_handler.go`<br>Сервис: `internal/service/pdf_service.go`, `pdf_format.go`, `pdf_links.go`<br>БД: `internal/repository/order_pdf.go` | Подписанные ссылки живут 10 мин (`/files/pdf/{token}/{name}`). |
| **Склад (Доски, Брёвна, Корректировки)** | Хендлеры: `board_handler.go`, `log_handler.go`, `stock_handler.go`<br>БД: `postgres.go`, `log_stock.go`, `stock_adjust.go` | При нулевом остатке строка `boards` НЕ удаляется (скрывается фильтром `count > 0`). |
| **Приход леса и Распиловка** | Хендлеры: `arrival_handler.go`, `sawing_handler.go`<br>БД: `postgres.go`, `log_stock.go` | Контроль выхода досок `validateSawingYield` (не больше объёма брёвен). |
| **Контрагенты и Сотрудники** | Хендлеры: `dict_handler.go`, `staff_handler.go`, `auth_handler.go`, `telegram_auth.go` | Логины Telegram имеют префикс `tg_<id>` и маркер пароля `tg_autogenerated`. |
| **Миграции БД и Docker** | `migrations/` (`000001`..`000009`)<br>`docker-compose.yml`, `Dockerfile` | Новые миграции называть по порядку (`000010_...up.sql`) и добавлять в `docker-compose.yml`. |

---

## 4. КЛЮЧЕВЫЕ БИЗНЕС-ИНВАРИАНТЫ (НЕ ЛОМАТЬ!)
1. **Цены и объёмы:** Сервер никогда не доверяет объёму досок от клиента при создании заказа/продажи — объём пересчитывается пропорционально остатку на складе через `domain.ItemVolume()`. Цена указывается за 1 м³.
2. **Дрова:** Учитываются общей кубатурой (сырьё = приходы `firewood_arrivals` минус списание `raw_m3` в `firewood_processing`). Рабочие вводят **готовый** объём (`split` или `round`), а сырьё списывается как `volume_m3 * coefficient`.
3. **Фронтенд (`web/index.html`):**
   - Использует глобальный объект состояния `S`, кэш данных `D`, карту форм `FORMS` и делегирование кликов через атрибут `data-a="..."` (обработчики в объекте `ACT`).
   - При изменении дизайна сохраняй все `id`, `data-a`, `data-v`, `data-k`, логику `FORMS`, `ACT` и функцию `api()`, чтобы не сломать привязку к бэкенду.

---

## 5. ШПАРГАЛКА ПО ДЕПЛОЮ (ВЫПОЛНЯТЬ ТОЛЬКО ПО МОЕЙ КОМАНДЕ)
Локальная проверка компиляции Go:
```bash
go build ./...