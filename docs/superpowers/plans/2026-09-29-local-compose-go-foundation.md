# Local Compose Go Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the existing Next.js booking UI, a new Go API, and PostgreSQL as one reproducible local Docker Compose development stack with migrations, health checks, source-driven rebuilds, and a defined full-booking E2E acceptance gate.

**Architecture:** Next.js remains the frontend at repository root. A new `api/` Go module owns configuration, HTTP transport, PostgreSQL access, and migrations. Compose starts PostgreSQL, runs a one-shot Go migration service, then starts the Go API and Next.js development servers on one network.

**Tech Stack:** Go 1.27, standard `net/http`, `pgx/v5`, PostgreSQL 16, Docker Compose, Next.js 16, pnpm 10, Vitest.

**Spec:** `openspec/changes/bootstrap-service-scheduler/design.md`; capability behavior remains in `openspec/changes/bootstrap-service-scheduler/specs/service-appointment-booking/spec.md`.

## Global Constraints

- Generated files, sample data, commit messages, and product copy must not identify the organization that supplied the source brief.
- The frontend is presentation-only; Go owns all backend runtime code and database tooling.
- Docker Compose is the only selected environment; no hosted provider configuration belongs in this milestone.
- API timestamps require explicit UTC offsets, persisted instants use UTC, and appointment intervals are half-open `[start, end)`.
- PostgreSQL owns allocation consistency; application checks never replace database constraints.
- Use fictional neutral fixtures only.

## Review Focus

- A clean machine with no images or volumes should reach healthy state through `docker compose up --build --wait`.
- An occupied host PostgreSQL port should not prevent the internal stack from starting; the host mapping remains configurable and defaults to `55432`.
- Missing or malformed API configuration should stop startup with variable names but never connection-string values.
- The API must not report ready before migrations have completed and PostgreSQL accepts queries.
- Source edits in both Go and Next.js must be visible without rebuilding production images or reinstalling dependencies.

---

### Task 1: Go API module, configuration, and health server

**Files:**
- Create: `api/go.mod`
- Create: `api/cmd/server/main.go`
- Create: `api/internal/config/config.go`
- Create: `api/internal/config/config_test.go`
- Create: `api/internal/httpapi/router.go`
- Create: `api/internal/httpapi/health.go`
- Create: `api/internal/httpapi/health_test.go`
- Create: `api/internal/application/doc.go`
- Create: `api/internal/domain/doc.go`
- Create: `api/internal/postgres/doc.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces: `config.Load(getenv func(string) string) (config.Config, error)` where `Config` contains `ListenAddress`, `DatabaseURL`, and `AllowedOrigin`.
- Produces: `httpapi.NewRouter(httpapi.Dependencies) http.Handler`.
- Produces: `httpapi.Dependencies{Readiness func(context.Context) error}`.
- Produces: `GET /api/v1/health/live` returning `200 {"status":"ok"}` and `GET /api/v1/health/ready` returning 200 or 503.

- [ ] **Step 1: Write failing configuration tests**

Add `TestLoadAcceptsLocalConfiguration`, `TestLoadReportsMissingVariableNames`, and `TestLoadDoesNotLeakValues`. Assert defaults `:8080`, required `DATABASE_URL`, required `ALLOWED_ORIGIN`, and error text containing only invalid variable names.

- [ ] **Step 2: Run the configuration tests and verify RED**

Run: `cd api && go test ./internal/config -run TestLoad -v`

Expected: FAIL because `config.Load` does not exist.

- [ ] **Step 3: Implement `config.Load`**

Accept only `postgres://` and `postgresql://` database URLs, require an absolute HTTP(S) origin without a path, and return an error whose message lists invalid keys alphabetically without their values.

- [ ] **Step 4: Run the configuration tests and verify GREEN**

Run: `cd api && go test ./internal/config -run TestLoad -v`

Expected: PASS.

- [ ] **Step 5: Write failing health-handler tests**

Add `TestLivenessDoesNotCallDatabase`, `TestReadinessReturnsOKWhenDatabaseResponds`, `TestReadinessReturnsUnavailableWithoutDetails`, `TestCORSAllowsConfiguredFrontend`, and `TestUnknownRouteReturnsJSONNotFound` using `httptest`.

- [ ] **Step 6: Run the handler tests and verify RED**

Run: `cd api && go test ./internal/httpapi -v`

Expected: FAIL because `NewRouter` and the health handlers do not exist.

- [ ] **Step 7: Implement the minimal router and server**

Use `http.ServeMux`, JSON content types, request IDs, exact-origin CORS, graceful shutdown on `SIGINT`/`SIGTERM`, and a readiness callback. `cmd/server` loads configuration, creates a bounded `pgxpool`, and passes `pool.Ping` as readiness.

- [ ] **Step 8: Verify Go package boundaries and tests**

Run: `cd api && gofmt -w . && go vet ./... && go test ./...`

Expected: PASS; `api/internal/domain` imports only the Go standard library.

- [ ] **Step 9: Mark OpenSpec task 1.2 complete and commit**

```bash
git add api openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: scaffold Go booking API"
```

### Task 2: Go migration runner and schema integration tests

**Files:**
- Create: `api/cmd/migrate/main.go`
- Create: `api/internal/postgres/migrate.go`
- Create: `api/internal/postgres/migrate_test.go`
- Create: `api/internal/postgres/schema_test.go`
- Move: `db/migrations/001_initial_schema.sql` → `api/migrations/001_initial_schema.sql`
- Delete: `scripts/db/migrate.mjs`
- Delete: `scripts/db/schema.test.ts`
- Delete: `src/config/env.ts`
- Delete: `src/config/env.test.ts`
- Modify: `package.json`
- Modify: `pnpm-lock.yaml`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Consumes: `config.Load` from Task 1.
- Produces: `postgres.Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) error`.
- Produces: `go run ./cmd/migrate` as the only migration command.

- [ ] **Step 1: Write failing migration-runner tests**

Add `TestMigrateAppliesFilesInLexicalOrder`, `TestMigrateSkipsAppliedFiles`, and `TestMigrateRollsBackFailedFile`. Use a fresh test schema and assert `schema_migrations` records only committed files.

- [ ] **Step 2: Run migration tests and verify RED**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -run TestMigrate -v`

Expected: FAIL because `postgres.Migrate` does not exist.

- [ ] **Step 3: Implement the migration runner**

Embed `api/migrations/*.sql` in `cmd/migrate`, use a PostgreSQL advisory lock to prevent concurrent migrators, wrap each file in one transaction, and store its filename in `schema_migrations` after success.

- [ ] **Step 4: Port the existing schema contract to Go**

Add Go tests named `TestSchemaHasRequiredExtensionAndTables`, `TestSchemaHasOwnershipForeignKeys`, `TestSchemaProtectsDurationsHoursAndStatus`, and `TestSchemaStoresIdempotencyHashes`. Preserve the exact assertions currently expressed in `scripts/db/schema.test.ts`.

- [ ] **Step 5: Prove tests fail on an empty database**

Run against a clean volume before migration: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -run TestSchema -v`

Expected: FAIL because the extension and tables are absent.

- [ ] **Step 6: Apply the migration and verify GREEN**

Run: `cd api && DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go run ./cmd/migrate && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -v`

Expected: PASS, including a second idempotent migration run.

- [ ] **Step 7: Remove Node backend dependencies and scripts**

Remove `postgres` and backend environment/migration scripts from `package.json`; retain only frontend/OpenAPI dependencies and tests. Run `pnpm install` to update the lockfile.

- [ ] **Step 8: Mark OpenSpec tasks 1.3, 2.1, and 2.2 complete and commit**

```bash
git add api package.json pnpm-lock.yaml scripts src/config openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: move database tooling to Go"
```

### Task 3: Three-service development Compose stack

**Files:**
- Create: `api/Dockerfile.dev`
- Create: `api/.air.toml`
- Create: `Dockerfile.web.dev`
- Create: `.dockerignore`
- Create: `compose.test.yaml`
- Modify: `compose.yaml`
- Modify: `.env.example`
- Modify: `scripts/local-database.test.ts`
- Modify: `package.json`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Consumes: Go server and migrator from Tasks 1–2.
- Produces: Compose services `postgres`, `migrate`, `api`, and `web`.
- Produces: host endpoints `http://localhost:${WEB_PORT:-3000}` and `http://localhost:${API_PORT:-8080}`.
- Produces: `pnpm stack:start`, `pnpm stack:stop`, `pnpm stack:reset`, `pnpm stack:logs`, and `pnpm stack:smoke`.

- [ ] **Step 1: Expand the failing Compose contract test**

Assert that `compose.yaml` defines all four services, service dependencies use `service_healthy` and `service_completed_successfully`, the web/API source directories are mounted, dependency caches use named volumes, and no provider-specific hostname or identifier appears.

- [ ] **Step 2: Run the Compose contract test and verify RED**

Run: `pnpm exec vitest run scripts/local-database.test.ts`

Expected: FAIL because `web`, `api`, and `migrate` services are absent.

- [ ] **Step 3: Add development Dockerfiles and Compose services**

Use `golang:1.27-alpine` for API build/dev, pin Air in the image, use `node:26.10.0-alpine` with pinned pnpm for the web service, mount source read-write, and keep `node_modules`, `.next`, Go build cache, and Go module cache in named volumes. Do not mount the Docker socket.

- [ ] **Step 4: Add lifecycle commands and environment examples**

Set Compose-internal values `DATABASE_URL=postgresql://scheduler:scheduler@postgres:5432/scheduler` and `NEXT_PUBLIC_API_BASE_URL=http://localhost:${API_PORT:-8080}`. Keep the configurable host database port at `55432`.

- [ ] **Step 5: Validate Compose syntax and contract tests**

Run: `docker compose config --quiet && pnpm exec vitest run scripts/local-database.test.ts`

Expected: PASS.

- [ ] **Step 6: Start from a clean volume and verify service health**

Run: `pnpm stack:reset && pnpm stack:start && docker compose ps`

Expected: PostgreSQL and API report healthy, `migrate` exits 0, and web remains running.

- [ ] **Step 7: Verify source-driven rebuilds**

Record the API liveness response and web heading, touch a Go source file and a React source file, then poll container logs/endpoints. Expected: Air rebuilds the API and Next.js recompiles without rebuilding images.

- [ ] **Step 8: Mark OpenSpec tasks 1.1 and 1.4 complete and commit**

```bash
git add api/Dockerfile.dev api/.air.toml Dockerfile.web.dev .dockerignore compose.yaml compose.test.yaml .env.example scripts/local-database.test.ts package.json openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: add local development compose stack"
```

### Task 4: Local stack smoke verification and developer handoff

**Files:**
- Create: `scripts/smoke-local.mjs`
- Create: `README.md`
- Modify: `next.config.ts`
- Modify: `package.json`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Consumes: Compose endpoints from Task 3.
- Produces: one command that verifies web HTML, API liveness, API readiness, and migrated schema availability.

- [ ] **Step 1: Write a failing smoke test script**

Implement assertions first for `GET /`, `GET /api/v1/health/live`, and `GET /api/v1/health/ready`; require HTTP 200, JSON status `ok` for health endpoints, and the booking page heading in HTML.

- [ ] **Step 2: Run the smoke command and verify RED**

Run: `pnpm stack:smoke`

Expected: FAIL until the script and stack command are wired together.

- [ ] **Step 3: Configure frontend development access**

Configure only the local public API base URL needed by the browser. Do not add hosted domains or provider settings. Ensure the Go API CORS test from Task 1 permits exactly that origin.

- [ ] **Step 4: Document the local workflow**

Document prerequisites, `stack:start`, logs, tests, migration behavior, ports, reset semantics, and troubleshooting for port conflicts. State explicitly that production hosting remains undecided.

- [ ] **Step 5: Run the complete clean-volume verification**

Run: `pnpm stack:reset && pnpm stack:start && pnpm stack:smoke && pnpm format:check && pnpm lint && pnpm test && pnpm typecheck && pnpm build && (cd api && gofmt -l . && go vet ./... && go test ./... && go build ./cmd/server ./cmd/migrate) && openspec validate bootstrap-service-scheduler --strict`

Expected: every command exits 0; `gofmt -l .` prints nothing.

- [ ] **Step 6: Scan confidentiality and clean up**

Verify tracked files contain no PDFs, source-identifying terms, hosted-provider commitments, secrets, or non-fictional fixtures. Run `pnpm stack:stop` without deleting the verified volume.

- [ ] **Step 7: Commit**

```bash
git add README.md scripts/smoke-local.mjs next.config.ts package.json openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "docs: add local stack workflow"
```

### Task 5: Full browser-to-database booking E2E acceptance

> **Dependency gate:** Execute this task after OpenSpec tasks 2.6, 3.6, 4.7, 5.5, and 6.3 are complete. Tasks 1–4 establish and smoke-test the stack; those booking tasks provide the real behavior this acceptance test exercises.

**Files:**
- Create: `playwright.config.ts`
- Create: `e2e/booking.spec.ts`
- Create: `e2e/fixtures.ts`
- Create: `api/internal/postgres/concurrency_test.go`
- Create: `src/app/appointments/[appointmentId]/page.tsx`
- Modify: `compose.test.yaml`
- Modify: `package.json`
- Modify: `pnpm-lock.yaml`
- Modify: `README.md`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Consumes: seeded vehicle, dealership, service type, qualified technician, and bay from OpenSpec task 2.6.
- Consumes: real Go endpoints from OpenSpec tasks 5.3 and 5.5.
- Consumes: generated frontend API client and persisted confirmation view from OpenSpec task 6.3.
- Produces: `pnpm test:e2e` for a clean-stack Playwright journey.
- Produces: `pnpm verify:e2e` for clean-volume startup, smoke checks, booking journey, reload verification, and repeated Go concurrency checks.

- [x] **Step 1: Add Playwright with a matching container image**

Pin `@playwright/test` in `devDependencies`, add `test:e2e` and `verify:e2e` scripts, and define an `e2e` service under the test Compose profile using the Playwright image version that matches the package. The service uses `http://web:3000` and never connects directly to PostgreSQL.

- [x] **Step 2: Write the failing persisted-booking browser test**

Create `booking.spec.ts` with one journey named `persists a confirmed appointment across reload`:

1. Open the booking page.
2. Select the seeded vehicle, service centre, service type, and date by accessible labels.
3. Request real availability and choose the first returned slot.
4. Verify the review states the service duration and interval without naming a technician or bay.
5. Confirm once and assert status `CONFIRMED`, technician, bay, vehicle, service, dealership, and interval are visible.
6. Capture the appointment identifier from `/appointments/{appointmentId}`.
7. Reload that URL and assert the same identifier and assignment fields remain visible.
8. Make a direct `GET /api/v1/appointments/{appointmentId}` request and assert its identifier and interval match the page.

- [x] **Step 3: Run the browser test and verify RED**

Run: `pnpm stack:reset && pnpm stack:start && pnpm test:e2e`

Expected: FAIL until the real API client, seeded reference data, confirmation endpoint, and appointment route are connected.

- [x] **Step 4: Complete the URL-based confirmation route**

After a successful confirmation, navigate to `/appointments/{appointmentId}`. The server-rendered page fetches `GET /api/v1/appointments/{appointmentId}` from the Go API and renders explicit loading, not-found, unavailable, and confirmed states. Do not persist the appointment payload in browser storage as a substitute for API retrieval.

- [x] **Step 5: Run the browser test and verify GREEN**

Run: `pnpm test:e2e`

Expected: PASS against the running Compose stack; a page reload performs a new appointment retrieval request.

- [x] **Step 6: Write the failing real-concurrency integration test**

Add `TestConcurrentConfirmationAllowsAtMostOneWinner`. Seed exactly one qualified technician and one active bay, release 20 goroutines simultaneously against overlapping confirmations with distinct idempotency keys, and assert exactly one confirmed appointment, 19 resource conflicts, and one overlapping database row.

- [x] **Step 7: Run the concurrency test repeatedly and verify GREEN**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./internal/postgres -run TestConcurrentConfirmationAllowsAtMostOneWinner -count=20`

Expected: PASS under the race detector with zero double bookings across all repetitions.

- [x] **Step 8: Run the clean-volume E2E release gate**

Run: `pnpm stack:reset && pnpm stack:start && pnpm stack:smoke && pnpm test:e2e && (cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./... -count=1)`

Expected: every command exits 0. Querying the confirmed appointment after the browser closes returns the persisted assignment from PostgreSQL.

- [x] **Step 9: Mark OpenSpec tasks 6.4 and the concurrency verification tasks complete and commit**

Mark only tasks whose exact assertions have passed; do not mark broader delivery verification complete until its README evidence is recorded.

```bash
git add playwright.config.ts e2e api/internal/postgres/concurrency_test.go src/app/appointments compose.test.yaml package.json pnpm-lock.yaml README.md openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "test: add persisted booking e2e verification"
```
