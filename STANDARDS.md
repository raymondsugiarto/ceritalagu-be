# Go + Fiber Coding Standard

Baseline for Go HTTP services. Stack: **Go 1.26**, **Fiber v3**, `log/slog`, `go-playground/validator`, `spf13/viper` (YAML config).
The `internal/module/user` package is the reference module. Copy its shape for new modules.
The directory layout follows `receivable-api`.

> Review markers: items tagged **[DECISION]** are choices worth confirming as a team.

---

## 1. Project layout

```
cmd/
  server/main.go             entrypoint: only runs cli.NewRootCmd(). No logic.
config/                      config package at the repo root (not under internal/)
  loader.go                  Config root struct + Load(): YAML → .env → env overrides, validated
  server.go, logger.go, …    one typed struct per concern, matching one YAML file
  example/*.yml              tracked templates (also used by tests: CONFIG_DIR=example)
  resources/*.yml            per-machine values read at runtime (*.local.yml is gitignored)
internal/
  cli/                       Cobra command tree (one file per subcommand)
    root.go                  root `server` cmd: --config, --version, AddCommand(...)
    serve.go                 `serve`: config → logger → modules → app → listen/shutdown
  app/                       Fiber app factory, global middleware, error handler, health probes
    app.go                   New(cfg, log, health, modules...), Listen, Shutdown, Module interface
    middleware.go            errorHandler, accessLog
    health.go                /healthz (liveness), /readyz (readiness)
    app_test.go              HTTP tests against the real app
  apperror/                  the one client-facing error type
  response/                  JSON envelope (Response[T]) helpers
  pagination/                pagination.Input / Output / ListOutput[T] (no HTTP dependency)
  validator/                 request validation (plugged into Fiber's binder)
  logger/                    slog setup
  database/                  (when a DB is added) connection open/ping/close + migrate runner
  module/
    <module>/                one package per business module (e.g. user, invoice, customer)
      <module>.go            domain entity, named after the module — no JSON tags, no framework imports
      dto.go                 <Action><Entity>Input / <Entity>Output structs + mapping
      repository.go          storage interface + implementation(s)
      service.go             business rules
      handler.go             HTTP handlers (bind → service → respond)
      router.go              RegisterRoutes: paths + module middleware
      *_test.go
      <sub>/                 sub-packages for a module's own helpers (e.g. auth/jwt, auth/password)
db/                          (when a DB is added)
  embed.go                   embeds migrations so the server can migrate at startup
  migrations/postgres/       NNNNNN_<verb>_<scope>.up.sql / .down.sql — rules: ../docs/database-standards.md
docs/                        feature specs that code must follow
bin/, tmp/                   build output and scratch files (gitignored)
```

- **[DECISION] Package by module**, not by layer. No top-level `handlers/`, `services/`, `models/` folders.
  Business modules live under `internal/module/`; shared technical packages sit directly under `internal/`.
- A big module splits by topic inside its package (`invoice/cashflow.go`, `invoice/profitloss.go`), not into sub-layers.
- Everything except `cmd/`, `config/` and `db/` lives under `internal/` so other modules can't import it.
- Add `pkg/` only for code meant to be imported by other repositories.

## 2. Layers and dependencies

```
handler  →  service  →  repository
   ↓           ↓
response    apperror
```

| Layer      | Does                                              | Must not                                         |
|------------|---------------------------------------------------|--------------------------------------------------|
| Handler    | Bind + validate input, call service, write output | Contain business rules, touch the DB, build error JSON |
| Service    | Business rules, orchestration, error mapping      | Import Fiber, know about HTTP status codes directly |
| Repository | Persistence only                                  | Return `apperror`; contain business rules        |

- Interfaces are declared **by the consumer** (e.g. `user.Repository` is defined in the `user` module that uses it). Accept interfaces, return structs.
- **[DECISION] Manual dependency injection** in the subcommand that needs it (`internal/cli/serve.go`). No DI framework (wire/fx) until wiring becomes painful.
- `context.Context` is the first parameter of every service and repository method. Pass `c.Context()` from handlers.

## 3. HTTP API conventions

- Base path: `/api/v1`. Health probes live at the root (`/healthz`, `/readyz`).
- Resources are plural nouns: `/users`, `/users/:id`.
- Methods: `GET` list/read, `POST` create (201), `PATCH` partial update (200), `PUT` full replace, `DELETE` (204, no body).
- **[DECISION] JSON fields use `snake_case`.**
- Timestamps are RFC 3339 in UTC (`time.Time` serialized by `encoding/json`).
- IDs are UUID strings. Path IDs are validated in the handler (`400` if malformed).
- Pagination: `?page=1&page_size=20` (default 20, max 100). Search: `?search=`.
- **[DECISION] BFF via `?view=` on the same resource**: `GET /invoices?view=summary`, `GET /invoices/:id?view=detail`. No `/bff/...` or extra resource endpoints; no `view` = plain entity; unknown `view` → `400`. Read-only, own `…Output` DTO per view ([database-standards.md §5.2](../docs/database-standards.md)).

### Response format

Every endpoint (success **and** error, including health probes) returns the same envelope,
defined in `internal/response`:

```go
type Response[T any] struct {
	HTTPCode  int                    `json:"-"`          // HTTP status, not serialized
	Code      string                 `json:"code"`       // "SUCCESS" or an apperror code
	Message   string                 `json:"message"`    // human-readable summary
	Errors    []apperror.ErrorDetail `json:"errors"`     // always an array, [] on success
	Data      T                      `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`  // UTC, set when the response is written
}

type ErrorDetail struct {
	Field   string `json:"field,omitempty"` // set for validation errors
	Message string `json:"message"`
}
```

> `Errors` is `[]ErrorDetail`, not `[]error`: Go's `error` interface serializes to `{}` in JSON,
> so a concrete struct is required to send the messages.

Success (single resource):
```json
{
  "code": "SUCCESS",
  "message": "success",
  "errors": [],
  "data": { "id": "…", "name": "Ada", "email": "ada@example.com", "created_at": "…", "updated_at": "…" },
  "timestamp": "2026-10-29T05:00:00Z"
}
```

Success (list) — `data` is always an object with `items` + `pagination`, never a bare array:
```json
{
  "code": "SUCCESS",
  "message": "success",
  "errors": [],
  "data": {
    "items": [ { "id": "…" } ],
    "pagination": { "page": 1, "page_size": 20, "total_items": 42, "total_pages": 3 }
  },
  "timestamp": "2026-10-29T05:00:00Z"
}
```

Success without data (e.g. delete → `200`, not `204`, so the envelope is kept):
```json
{ "code": "SUCCESS", "message": "user deleted", "errors": [], "timestamp": "…" }
```

Error (`data` is omitted):
```json
{
  "code": "VALIDATION_ERROR",
  "message": "request validation failed",
  "errors": [
    { "field": "name", "message": "must be at least 2" },
    { "field": "email", "message": "must be a valid email address" }
  ],
  "timestamp": "2026-10-29T05:00:00Z"
}
```

- Handlers use only `response.OK`, `response.Created`, `response.Message`, or `response.Success` (custom status/message).
  Never call `c.JSON` with ad-hoc shapes.
- The request ID is returned in the `X-Request-ID` header (not in the body) and is logged with every request.

### DTO naming: `Input` / `Output`

| Kind | Name | Example |
|---|---|---|
| Payload the handler receives (body, query) | `<Action><Entity>Input` | `CreateUserInput`, `UpdateUserInput`, `ListUsersInput` |
| Data the handler returns in `data` | `<Entity>Output` | `UserOutput` |
| List data | `pagination.ListOutput[<Entity>Output]` | `pagination.ListOutput[UserOutput]` |
| Shared pagination | `pagination.Input` (embedded), `pagination.Output` | |

- DTOs live in the feature's `dto.go`. Domain entities (`model.go`) are never returned directly.
- Map with `to<Entity>Output(...)` functions in `dto.go`.
- Services accept `…Input` and return `…Output`, so handlers stay one-liners: bind → call → respond.
- Variable names in handlers: `in` for the input, `out` for the result.
- **[DECISION]** Names include the entity even inside its own package (`user.CreateUserInput`) for clarity at call sites.

## 4. Errors

- Handlers only `return err`. The global `errorHandler` in `internal/app/middleware.go` is the **only** place errors become HTTP responses.
- Client-facing failures are `*apperror.Error` (`NotFound`, `Conflict`, `Validation`, `BadRequest`, `Unauthorized`, `Forbidden`).
- Any other error becomes `500 INTERNAL_ERROR`. Its message is logged, never sent to the client.
- Repositories return sentinel errors (`ErrNotFound`, `ErrEmailTaken`). The service maps them in one function (`mapRepoErr`).
- Wrap with context: `fmt.Errorf("load user %s: %w", id, err)`. Compare with `errors.Is` / `errors.As`, never `==` on wrapped errors or string matching.
- No `panic` for control flow. `recover` middleware is a safety net only.

| Code                  | HTTP |
|-----------------------|------|
| `SUCCESS`             | 200 / 201 |
| `BAD_REQUEST`         | 400  |
| `UNAUTHORIZED`        | 401  |
| `FORBIDDEN`           | 403  |
| `NOT_FOUND`           | 404  |
| `CONFLICT`            | 409  |
| `VALIDATION_ERROR`    | 422  |
| `TOO_MANY_REQUESTS`   | 429  |
| `INTERNAL_ERROR`      | 500  |
| `SERVICE_UNAVAILABLE` | 503  |

## 5. Validation

- Declare rules with `validate:"…"` tags on `…Input` DTOs. `c.Bind().Body(&req)` / `.Query(&q)` validates automatically (registered as Fiber's `StructValidator`).
- Field names in errors come from `json` / `query` tags.
- Use pointer fields in PATCH inputs to tell "not sent" from "sent empty".
- Validation is shape only (required, length, format). Rules needing data (e.g. "email unique") belong in the service.

## 6. Configuration

- All config is loaded once by `config.Load()` (`config/loader.go`, viper). No `os.Getenv` anywhere else.
- Precedence, highest first: **shell env → `.env` (repo root) → `config/resources/*.yml`**.
  Env keys are the YAML path with `.` → `_`, upper-cased: `server.rest.port` → `SERVER_REST_PORT` (`PORT`/`HOST` are aliases).
- One YAML file per concern (`server.yml`, `logger.yml`, later `database.yml`, …), each with a typed struct in
  `config/<concern>.go` (`mapstructure` tags, camelCase YAML keys) and a field on `Config`. Register the file in `loader.go`.
- Every key has a value in `config/example/*.yml` (tracked template). `config/resources/` holds per-machine values.
  `CONFIG_DIR=example` reads the templates; `CONFIG_PATH=/dir` reads any directory (the container sets it).
- Invalid config fails startup with a clear error (`Config.validate`).
- Secrets never live in tracked YAML and are never logged; set them by env (`SERVER_REST_SECRETKEY=…`).
- Mail is one concern (`mail.yml`) with three drivers (`log`, `smtp`, `brevo`); `internal/mail` implements them
  behind one `Sender` interface. Secrets (`MAIL_SMTP_PASSWORD`, `MAIL_BREVO_APIKEY`) come from env only.
- "Your song is ready" is a separate concern (`notification.yml`) so it can be turned off without touching mail
  config. It is wired in the **worker** (`internal/cli/worker.go`), not the API.

## 7. Logging

- `log/slog` only. JSON in deployed environments, text locally (`logger.format`).
- Pass `*slog.Logger` explicitly; don't use the global logger in feature code.
- Use key/value attributes, not formatted strings: `log.Info("user created", "user_id", id)`.
- Always include `request_id` for request-scoped logs.
- Never log passwords, tokens, full request bodies, or personal data.
- Levels: `Debug` dev detail · `Info` normal events · `Warn` client errors (4xx) / recoverable · `Error` server errors (5xx).

## 8. Middleware (order matters)

1. `requestid` — sets/propagates `X-Request-ID`
2. `accessLog` — one log line per request with final status and latency
3. `recover` — panics → 500
4. `helmet` — security headers
5. `cors` — allowlist from `server.rest.corsOrigins` (never `*` with credentials)

Module-specific middleware (auth, permissions, rate limit) is applied on the route group inside the module's `router.go`.

## 9. Lifecycle

- `main` runs the Cobra root; subcommands return errors from `RunE`; only `main` calls `os.Exit`.
- Graceful shutdown on `SIGINT`/`SIGTERM` with `server.rest.shutdownTimeout`.
- Background goroutines take a `context.Context` and stop when it's cancelled. No fire-and-forget goroutines.

## 10. Testing

- Standard `testing` package, table-driven tests, `t.Parallel()` where safe.
- **Service tests** use the in-memory repository (`internal/module/user/service_test.go`).
- **HTTP tests** build the real app (with `CONFIG_DIR=example`) and call `Fiber().Test(req)` (`internal/app/app_test.go`), checking status and that every response has `code`, `message`, `errors` (array) and `timestamp`.
- Database repositories get integration tests against a real DB (e.g. testcontainers), behind a build tag.
- Always run with `-race`. **[DECISION]** No assertion library by default (add `testify` if the team prefers).

## 11. Style and tooling

- `gofmt` + `goimports` (local imports grouped last). Enforced by `golangci-lint` (`.golangci.yml`).
- Naming: short package names, no `util`/`common`/`helpers` packages; no stutter (`user.Service`, not `user.UserService`).
  Exception: DTOs keep the entity name (`user.UserOutput`, `user.CreateUserInput`) so they match the TypeScript types; the lint config allows this.
- Every exported identifier has a doc comment starting with its name (revive enforces it). Say what it is for or what it returns, not how.
- Keep functions small; return early instead of nesting.
- `make test`, `make lint`, `make run` must pass before every PR.

## 12. Database

**[DECISION] Every database design follows [database-standards.md](../docs/database-standards.md).** In short:

- Tables singular, names English `snake_case`; `id VARCHAR(36) PRIMARY KEY` with the UUID minted in Go (no DB default);
  references `VARCHAR(36)`; timestamps `TIMESTAMPTZ`; soft delete via `deleted_at`.
- The DB is a dumb store: no `FOREIGN KEY`, `UNIQUE`, `NOT NULL`, `CHECK`, `DEFAULT`, index, trigger, function or generated column.
- So the service owns those rules: required fields via `validate` tags, uniqueness and references checked inside the
  write transaction (→ `apperror.Conflict` / `NotFound`), `deleted_at IS NULL` on every read — each one unit-tested.
- Migrations are **DDL only**. `INSERT`/`UPDATE` need the owner's approval (own `seed_*` migration); `DELETE`, `TRUNCATE`,
  `DROP` and row-specific DML are never written by tools/AI — suggest them, the owner adds them (§4.2 of the standard).
- **No deletion anywhere** ([§4.5](../docs/database-standards.md)): no `rm`/storage/DB delete commands or scripts, no hard-delete code
  (`os.Remove*`, `Unscoped().Delete`, object-storage deletes). Deletes in the app are soft (`deleted_at`).
- **[DECISION] GORM for all standard CRUD** (`Create`, `First`, `Find`, `Updates`, `Save`, soft `Delete`). Never `tx.Exec`/`Raw` for
  `INSERT`/`UPDATE`/`DELETE`. Raw SQL only for complex read-only `SELECT` (joins, aggregates, `?view=` BFF views) with `?`
  placeholders. No `AutoMigrate` ([§5.1](../docs/database-standards.md)).
- Migrations: golang-migrate up/down pairs via `make migrate-create name=<verb>_<scope>`, applied with `make migrate-up` (→ `server db migrate up`), header comment, `IF [NOT] EXISTS`,
  never edit a merged migration.

## 13. CLI (Cobra)

- **[DECISION] One binary, many subcommands** via [Cobra](https://github.com/spf13/cobra): `server serve`, `server db migrate up|down|version|force`, `server seed`, … No extra `cmd/<tool>/main.go` binaries.
- `cmd/server/main.go` only calls `cli.NewRootCmd().ExecuteContext(ctx)` and exits 1 on error.
- New subcommand → `internal/cli/<name>.go` with `newXxxCmd() *cobra.Command`, registered in `root.go`. Use `RunE` (never `os.Exit` inside), `Args: cobra.NoArgs`/`ExactArgs`, and `cmd.Context()`.
- Flags are for one-off overrides only (`--config`). App settings still come from `config.Load()`; a flag maps to its env var, it never adds a second config source.
- Root sets `SilenceUsage`/`SilenceErrors`: usage prints only for bad flags/args, the error prints once from `main`.
- `version` is injected at build time: `make build` passes `-X …/internal/cli.version=$(VERSION)`; check with `server --version`.
- Database commands live under `db` (`internal/cli/migrate.go`): `db migrate up [step]`, `down [step]` (default 1), `version`, `force <version>`, with a `--schema` flag (empty = `database.main.schema`). `make migrate-up step=1 schema=x` is exactly `go run ./cmd/$(APP) db migrate up $(step) --schema=$(schema)`. The runner is `database.Migrator` (`internal/database/migrate.go`), shared with the startup auto-migrate.
- Run: `make run` (= `go run ./cmd/server serve`). The container image uses `ENTRYPOINT ["/server"]` + `CMD ["serve"]`, so `docker run img db migrate up` works too.

## 14. Adding a new module (checklist)

1. Copy `internal/module/user` → `internal/module/<module>`, rename types.
2. Define `<module>.go` (entity) and `dto.go` (`…Input` with `validate` tags, `…Output` + mapper).
3. Define the `Repository` interface; implement it (memory first, then DB). New tables → `make migrate-create name=create_<entity>`, designed per [database-standards.md](../docs/database-standards.md) (run its review checklist).
4. Write service + tests.
5. Write `handler.go`, then `router.go` with `RegisterRoutes`.
6. Wire it in `internal/cli/serve.go` (pass the handler to `app.New`). New config → `config/<concern>.go` + `config/example/<concern>.yml`.
7. Add HTTP tests in `internal/app/app_test.go` (or a module-level `handler_test.go`).

## Not included yet (add when needed)

Database (`internal/database` + `config/database.yml`; migrations as SQL files in `db/migrations/postgres/`, embedded via `db/embed.go`), auth (JWT/session middleware),
rate limiting (`middleware/limiter`), OpenAPI docs, metrics/tracing (OpenTelemetry).
