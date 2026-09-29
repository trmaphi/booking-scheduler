# Service appointment scheduler

This repository demonstrates a resource-constrained service booking system. A Next.js interface calls a separate Go REST API, and PostgreSQL persists appointments and enforces the final allocation invariants. All committed fixtures are fictional.

Production hosting, domains, and managed database choices remain undecided. The application runs locally with Docker Compose and standard PostgreSQL URLs; no hosted account or paid service is required.

## Architecture

```text
Browser → Next.js web (3000) → Go REST API (8080) → PostgreSQL (55432)
                                  │
                                  └─ JSON logs, metrics, W3C trace context
```

The API is split into HTTP transport, application orchestration, domain rules, and PostgreSQL adapters. Availability returns slots only when a qualified technician and a bay can cover the full service interval. Confirmation runs in a database transaction, derives the end time from the service type, allocates both resources, and relies on partial GiST exclusion constraints to reject overlapping confirmed work. Idempotency records make safe retries return the original appointment.

The OpenAPI contract is the source for the TypeScript client in `src/features/booking/api/generated`. `pnpm api:check` regenerates it and fails when committed output is stale.

## Prerequisites

- Docker with Compose v2
- Node.js 26.10.0 or newer
- pnpm 10.17.1
- Go at the version declared in `api/go.mod`
- OpenSpec CLI for strict specification validation

## Run locally

From the repository root:

```sh
corepack enable
pnpm install --frozen-lockfile
pnpm stack:start
pnpm stack:smoke
```

`stack:start` builds PostgreSQL, a one-shot Go migration service, the API, and the Next.js development server. Compose waits for database-backed health checks. The migration service applies versioned SQL in order, records each file in `schema_migrations`, and skips already applied files. The seed migration is repeatable and restores the same fictional reference records.

Open the web interface at <http://localhost:3000>. The API is at <http://localhost:8080>, and PostgreSQL is exposed at `localhost:55432` for local tests. Compose supplies its own internal addresses.

Useful lifecycle commands:

```sh
pnpm stack:logs
pnpm stack:stop
pnpm stack:reset
```

`stack:stop` preserves named volumes. `stack:reset` removes PostgreSQL data, generated E2E evidence, dependency caches, and all other Compose volumes. **Reset permanently deletes local appointment data.** Run `pnpm stack:start` afterward to migrate and seed a new database.

Host-side defaults are documented in `.env.example`. Keep overrides in the shell or an untracked `.env` file. To avoid occupied ports, apply the same overrides to every lifecycle command:

```sh
WEB_PORT=3001 API_PORT=8081 POSTGRES_PORT=55433 pnpm stack:start
WEB_PORT=3001 API_PORT=8081 POSTGRES_PORT=55433 pnpm stack:smoke
```

## REST API

The complete contract and examples are in `openapi/booking-api.yaml`. These examples use only the committed fictional fixtures.

```sh
curl --fail http://localhost:8080/api/v1/booking-options

curl --fail --get http://localhost:8080/api/v1/availability \
  --data-urlencode 'vehicleId=20000000-0000-0000-0000-000000000011' \
  --data-urlencode 'dealershipId=20000000-0000-0000-0000-000000000021' \
  --data-urlencode 'serviceTypeId=20000000-0000-0000-0000-000000000041' \
  --data-urlencode 'date=2030-01-03'

curl --fail-with-body http://localhost:8080/api/v1/appointments \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: demo-request-0001' \
  --data '{"vehicleId":"20000000-0000-0000-0000-000000000011","dealershipId":"20000000-0000-0000-0000-000000000021","serviceTypeId":"20000000-0000-0000-0000-000000000041","startAt":"2030-01-03T09:00:00Z"}'
```

Confirmation returns HTTP 201 on the first request and HTTP 200 with the same record for an identical idempotent replay. Reusing a key for changed input or confirming stale availability returns a stable HTTP 409 error. Retrieve a confirmed record with `GET /api/v1/appointments/{appointmentId}`. Liveness is `GET /api/v1/health/live`; readiness is `GET /api/v1/health/ready` and returns HTTP 503 while PostgreSQL is unavailable.

## Development checks

Run frontend and contract checks from the repository root:

```sh
pnpm api:check
pnpm format:check
pnpm lint
pnpm test
pnpm typecheck
pnpm build:check
```

Run Go checks:

```sh
pnpm go:format:check
pnpm go:vet
pnpm go:test
pnpm go:build
```

With the stack running on its default database port, include PostgreSQL integrations and the race detector:

```sh
cd api
TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./... -count=1
TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./internal/postgres -run 'TestConcurrentConfirmationAllowsAtMostOneWinner|TestIdempotency' -count=20
cd ..
```

Validate the specification and confidentiality policy:

```sh
openspec validate bootstrap-service-scheduler --strict
pnpm scan:confidentiality
```

The optional dependency-volume regression test rebuilds temporary Compose services and proves a dependency refresh preserves PostgreSQL data:

```sh
RUN_COMPOSE_INTEGRATION=1 pnpm vitest run scripts/web-dependency-sync.integration.test.ts
```

## Browser and delivery verification

With the stack running, the pinned Playwright container exercises booking, durable reload, and API retrieval:

```sh
pnpm test:e2e
```

The complete release gate is:

```sh
pnpm verify:delivery
```

It runs sequentially and stops on the first failure. It removes all Compose volumes, verifies the generated client, runs frontend formatting/lint/unit/type/build checks, checks Go formatting and vet, starts a newly migrated and seeded stack, runs the full Go suite with PostgreSQL and the race detector, repeats concurrency and idempotency tests, builds both Go commands, runs smoke and Playwright checks, validates OpenSpec, scans tracked files and every reachable Git object for confidential artifacts, and requires a clean Git tree. Because it begins by deleting volumes, do not run it when local database contents must be retained.

Recorded evidence and tool versions are in [docs/verification.md](docs/verification.md). The concise review walkthrough is in [docs/demo-script.md](docs/demo-script.md), and [docs/ai-collaboration.md](docs/ai-collaboration.md) describes how assisted work was reviewed.

### Latest verification

On 2026-09-29 UTC, sanitized candidate `58b0109c6b9c` passed all 18 delivery steps from empty volumes: generated-client consistency; formatting, lint, 85 frontend and delivery tests with 1 opt-in integration test skipped, explicit Next type generation, and the production build; Go formatting, vet, the complete PostgreSQL race suite, 20 repetitions of the concurrency/idempotency group, and both Go command builds; migration and fictional seed startup; four smoke assertions; one Playwright booking/reload/API-read-back journey; strict OpenSpec validation; the all-ref confidentiality scan; and the final clean-tree check. The same full gate was rerun after the evidence-only documentation update.

## Observability

The API emits structured JSON operational events to standard output. Request, availability, confirmation, conflict, retry, database, counter, and duration events use bounded dimensions such as route templates, result categories, service-centre/service-type identifiers, request IDs, and trace IDs. Tests reject customer details, registrations, request bodies, direct customer/vehicle identifiers, idempotency keys, raw database errors, and connection strings in telemetry.

The API accepts one canonical W3C `traceparent` header, preserves a valid incoming trace ID and sampling flag, creates a new request span, returns the canonical child header, and correlates HTTP and database events. Invalid or ambiguous headers are replaced. Local JSON events need no collector. A future exporter can implement the `telemetry.Recorder` interface; no OpenTelemetry exporter is currently configured.

## Troubleshooting

- **A port is occupied:** set `WEB_PORT`, `API_PORT`, and `POSTGRES_PORT` consistently, or stop the conflicting process.
- **A container does not become healthy:** inspect `pnpm stack:logs` and `docker compose ps`; the readiness endpoint reports dependency availability without exposing connection details.
- **Generated-client check fails:** run `pnpm api:generate`, review the OpenAPI-derived diff, then commit it.
- **Database tests skip:** start Compose and provide `TEST_DATABASE_URL` as shown above.
- **E2E reports a stale artifact:** use `pnpm stack:reset`; the delivery gate requires a fresh artifact volume.
- **Dependencies changed inside the web container:** restart `pnpm stack:start`; startup refreshes the named dependency volume from the lockfile.

## Trade-offs and alternatives

- PostgreSQL exclusion constraints and transactions make correctness durable and observable, at the cost of database-specific SQL. An application-only lock is more portable but cannot protect writes from every process.
- Availability is advisory and confirmation rechecks constraints. Holding a reservation would reduce stale selections but adds expiry, cleanup, and user-state complexity.
- Deterministic load-then-identifier ordering is easy to test. A richer assignment policy could improve fairness but needs operational data and explicit business rules.
- A single Go API process is enough for this bounded synchronous workflow. A worker or queue becomes useful for slow external integrations, reminders, or retryable side effects; none are required for confirmation itself.
- JSON telemetry keeps local development self-contained. A production exporter and collector would improve aggregation while adding deployment and cost decisions.
- The browser calls the Go API directly in local development. A same-origin proxy can simplify production CORS, but its hosting topology remains deliberately undecided.
