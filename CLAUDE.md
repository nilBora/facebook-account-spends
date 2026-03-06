# Claude Memory

## ⚠️ ВАЖЛИВІ ІНСТРУКЦІЇ ⚠️

- **НІКОЛИ** не згадуй Claude, Claude Code або Anthropic у повідомленнях комітів чи у згенерованому коді.
- **НІКОЛИ** не додавай теги на кшталт "Generated with Claude Code" у будь-які матеріали.

## Додаткові ресурси та посібники

- [Uber Go Style Guide](./UBER_GO_CODESTYLE.md) — посібник зі стилю коду Go від Uber
- [Effective Go](https://golang.org/doc/effective_go) — офіційний посібник з ідіоматичного Go

Під час роботи з Go-проєктами завжди дотримуйся принципів **Effective Go** та посібника зі стилю **Uber Go**.

---

## Інструкція зі створення Git-комітів

Під час написання повідомлень комітів дотримуйся таких правил:

### Стандарт повідомлень комітів

Мета: створити одне повідомлення у форматі **Conventional Commit**.

### Структура повідомлення

**ПЕРШИЙ РЯДОК (обов’язковий, зверху):**
Шаблон: `<тип>(<необов’язкова_область>): <короткий_опис>`

**ПРАВИЛО:** довжина першого рядка **НЕ ПОВИННА** перевищувати 72 символи. Оптимально — близько 50 символів.

`<тип>`: Проаналізуй **увесь diff** і вибери **один тип** для основної зміни:
- feat: новий функціонал
- fix: виправлення помилки
- chore: технічне обслуговування коду
- refactor: рефакторинг коду
- test: додавання або зміна тестів
- docs: зміна документації
- style: форматування, відступи тощо
- perf: покращення продуктивності
- ci: зміни у CI
- build: зміни у процесі збирання

`<необов’язкова_область>`: якщо зміна стосується певного компонента — вкажи його; якщо ні — пропусти.

`<короткий_опис>`:
- Використовуй наказовий спосіб, теперішній час (наприклад, *"add taskfile utility"*, *"fix login bug"*)
- **Не** починай з великої літери (наприклад, *"add"*, а не *"Add"*), якщо це не власна назва чи абревіатура
- **Не** став крапку наприкінці
- Коротко підсумуй основну мету **всіх** змін

---

**ТІЛО (необов’язкове; якщо додаєш — відділи однією порожньою лінією):**
Поясни, **що саме** змінилося і **чому** — для всіх змін у diff.

**ПРАВИЛО:** кожен рядок у тілі (включно зі списками) **не повинен перевищувати 72 символи**.

Якщо diff охоплює кілька аспектів:
- Деталізуй кожен із них у маркованому списку, починаючи з "- ".
- Приклад:
  ```
  - introduce Taskfile.yml to automate common development
    workflows, like building, running, and testing.
  - update .gitignore to exclude temporary build files.
  - refactor user tests for clarity.
  ```
- Не створюй нових рядків, подібних до першого (з type:scope), у тілі.

---

**НИЖНІЙ КОЛОНТИТУЛ (необов’язковий; відділи однією порожньою лінією):**
Шаблон: `BREAKING CHANGE: <опис>` або `Closes #<issue_id>`

**ПРАВИЛО:** кожен рядок у нижньому колонтитулі також не повинен перевищувати 72 символи.

---

### Приклад повного повідомлення коміту:

```
feat(devworkflow): introduce Taskfile and streamline development environment

This commit introduces a Taskfile to automate common development
tasks and includes related improvements to the project's development
environment and test consistency.

- add Taskfile.yml defining tasks for:
  - building project binaries and mock servers
  - running the application and associated services
  - executing functional test suites with automated setup/teardown
- modify .gitignore to exclude build artifacts, log files,
  and common IDE configuration files.
- adjust test messages in bot_test.go to ensure consistent
  casing and fix minor sensitivity issues.

Closes #135
```

---

## Стиль коду та комунікація

### Загальні принципи написання коду:

- Суворо дотримуйся стилю та домовленостей, уже прийнятих у проєкті
- Використовуй рядки до 80–100 символів, якщо не вказано інше
- Дотримуйся принципів «чистого коду»: читабельність і зрозумілість понад усе
- Уникай надмірних коментарів, але документуй складну логіку та API

---

### Повідомлення про помилки та логи:

- Починай із малої літери
- Будь лаконічним та інформативним
- Приклад:
  ```go
  log.Error("failed to connect to api", zap.Error(err))
  ```

---

### Мова коду та коментарів:

- Увесь код, коментарі, назви змінних і функцій повинні бути англійською мовою
- Дотримуйся галузевих стандартів для відповідної мови програмування
- Коментарі мають бути короткими та зосередженими на суті функціональності

---

### Для Go-проєктів:

- Суворо дотримуйся принципів [Effective Go](https://golang.org/doc/effective_go)
- Використовуй рекомендації [Uber Go Style Guide](./UBER_GO_CODESTYLE.md)
- Пиши ідіоматичний Go, включно з такими практиками:
  - Обробка помилок через повернення значень
  - Використання інтерфейсів для абстракції
  - Дотримання стандартних конвенцій іменування
  - Використання стандартних пакетів бібліотеки Go

---

## Проєкт: Facebook Account Parser

### Призначення

Збір spend-даних з рекламних акаунтів Facebook. Токени надають фармери
(багато BM, багато акаунтів). Система збирає витрати по кожному акаунту
за день і нормалізує до USD.

### Технічний стек

- **Мова:** Go 1.22+
- **Router:** `github.com/go-chi/chi/v5`
- **БД (dev):** SQLite (`modernc.org/sqlite`, CGo-free)
- **БД (prod):** PostgreSQL (та сама схема, лише DSN)
- **Міграції:** `golang-migrate/migrate`
- **Frontend:** `html/template` + HTMX + вбудований CSS (embed FS)
- **Scheduler:** `robfig/cron`
- **Логи:** `log/slog` (stdlib)
- **Конфіг:** env змінні

### Референс для Frontend

Стиль та патерни взяті з:
`/Users/nilborodulia/Sites/servers-manager/app/server/web`

Ключові патерни, яких суворо дотримуємось:
- `//go:embed static` та `//go:embed templates` через `embed.FS`
- `Handler` struct з полем `store` та `tmpl *template.Template`
- Один файл на сторінку: `dashboard.go`, `tokens.go`, `accounts.go` тощо
- Роути: сторінки (`GET /`) + HTMX-партіали (`GET /web/...`)
- `templateData` — єдиний struct для всіх шаблонів
- Партіали першими, потім сторінки у `parseTemplates()`
- `renderError(w, status, message)` для помилок
- Modal pattern: `hx-get="/web/.../new"` → `hx-target="#modal-content"`
- Confirm delete: `onclick="confirmDelete(...)"` через `app.js`
- Теми: CSS змінні `data-theme` у `<html>`, cookie `theme`
- `maskApiKey` templateFunc для токенів

### Структура CSS класів (з servers-manager)

```
.data-table, .table-container   — таблиці
.stat-card, .stats-grid         — статистика на дашборді
.btn, .btn-primary/secondary/danger/small — кнопки
.status-badge, .status-active   — статуси
.form-group, .form-row          — форми
.modal-backdrop, .modal         — модальні вікна
.page-header, .header-actions   — заголовки сторінок
.empty-state                    — порожній стан
.htmx-indicator                 — спінер під час запитів
```

### Структура проєкту

```
facebook-account-parser/
├── cmd/server/main.go
├── internal/
│   ├── facebook/
│   │   ├── client.go      # HTTP + rate limit headers
│   │   ├── accounts.go    # /me/adaccounts discovery
│   │   ├── insights.go    # sync + async insights
│   │   └── errors.go      # FB error types + wait times
│   ├── token/
│   │   ├── manager.go     # quota tracking, best token selection
│   │   └── lifecycle.go   # expires_at alerts
│   ├── sync/
│   │   ├── scheduler.go   # cron jobs
│   │   └── pipeline.go    # discover → verify → fetch → backfill
│   ├── queue/
│   │   └── worker.go      # goroutine pool з semaphore per token
│   ├── currency/
│   │   └── fx.go          # FX rates + кешування
│   ├── db/
│   │   ├── db.go
│   │   └── migrations/
│   └── web/
│       ├── handler.go     # Handler struct, Register, parseTemplates
│       ├── dashboard.go
│       ├── tokens.go
│       ├── accounts.go
│       ├── spend.go
│       ├── static/        # style.css, htmx.min.js, app.js
│       └── templates/
│           ├── base.html
│           ├── dashboard.html
│           ├── tokens.html
│           ├── accounts.html
│           ├── spend.html
│           └── partials/
│               ├── nav.html
│               ├── token-table.html
│               ├── token-form.html
│               ├── account-table.html
│               ├── spend-table.html
│               ├── dashboard-stats.html
│               └── status-badge.html
├── config/config.go
├── spend_plan.md
└── CLAUDE.md
```

### База даних (спрощена схема)

```sql
-- Токени від фармерів
CREATE TABLE tokens (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,      -- "Фармер Vasya"
    access_token TEXT NOT NULL,      -- зашифрований AES-256
    is_active    INTEGER DEFAULT 1,
    expires_at   DATETIME,           -- NULL = безстроковий
    last_error   TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Рекламні акаунти (з /me/adaccounts)
CREATE TABLE ad_accounts (
    id             TEXT PRIMARY KEY,  -- "act_XXXXXXX"
    name           TEXT,
    currency       TEXT,
    status         INTEGER,           -- 1=active
    token_id       TEXT NOT NULL REFERENCES tokens(id),
    last_synced_at DATETIME
);

-- Spend дані (сирі)
CREATE TABLE spend_raw (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES ad_accounts(id),
    date        TEXT NOT NULL,        -- YYYY-MM-DD
    campaign_id TEXT,
    adset_id    TEXT,
    adset_name  TEXT,
    impressions INTEGER DEFAULT 0,
    clicks      INTEGER DEFAULT 0,
    spend       REAL NOT NULL,
    currency    TEXT NOT NULL,
    synced_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_id, date, adset_id)
);

-- Стан синхронізації per account
CREATE TABLE sync_state (
    account_id    TEXT PRIMARY KEY REFERENCES ad_accounts(id),
    last_ok_date  TEXT,
    next_retry_at DATETIME
);

-- FX курси для нормалізації
CREATE TABLE fx_rates (
    date         TEXT NOT NULL,
    currency     TEXT NOT NULL,
    rate_to_usd  REAL NOT NULL,
    PRIMARY KEY (date, currency)
);
```

### Facebook API — ключові обмеження

- Rate limits: 3 рівні — App (`X-App-Usage`), User/token, Ad Account BDC
  (`X-Business-Use-Case-Usage`) — читати після кожної відповіді
- `X-FB-Ads-Insights-Throttle` — окремий header для Insights
- Errors: 4=App, 17=Token, 80000/80004=AdAccount → різні wait times
- Async insights при великих акаунтах (polling кожні 10-30с, timeout 15хв)
- Attribution lag: backfill останніх 3 днів щодня
- Pagination: cursor-based (`paging.next`), limit=500 для adaccounts
- Batch requests: до 50 sub-requests (не знімає ліміти, але зменшує HTTP)
- Concurrency: max 3-5 паралельних запитів per token (semaphore)

### Фази реалізації

**Фаза 1 — MVP:**
1. DB schema + migrations (SQLite)
2. `internal/facebook/client.go` — HTTP client + rate limit headers
3. `internal/facebook/accounts.go` — discovery з pagination
4. `internal/token/manager.go` — quota tracking
5. `internal/queue/worker.go` — goroutine pool
6. `internal/facebook/insights.go` — sync fetch
7. `internal/sync/pipeline.go` — базовий pipeline
8. Web UI: tokens CRUD + accounts list + базовий dashboard

**Фаза 2 — Надійність:**
9. Async insights + polling
10. Attribution backfill (rolling 3 дні)
11. Token lifecycle alerts (expires_at)
12. Error handling per error code (backoff map)

**Фаза 3 — Повнота:**
13. FX нормалізація (spend_normalized)
14. Batch requests optimization
15. PostgreSQL migration support
16. Quota usage у UI (індикатори per token)
