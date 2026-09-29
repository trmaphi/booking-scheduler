# Booking Core and Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the database invariants, fictional reference data, domain scheduling rules, advisory availability, atomic resource allocation, idempotency, and typed PostgreSQL reads required by the booking API.

**Architecture:** Pure Go domain types own time, qualification, and ordering rules. Application services depend on narrow repository interfaces and perform one bounded availability read per request. PostgreSQL owns overlap prevention and the complete confirmation transaction; `pgx` adapters translate database results into typed application outcomes.

**Tech Stack:** Go 1.27, `pgx/v5`, PostgreSQL 16, SQL migrations, Docker Compose, Go unit and integration tests.

**Spec:** `openspec/changes/bootstrap-service-scheduler/design.md`; observable behavior is in `openspec/changes/bootstrap-service-scheduler/specs/service-appointment-booking/spec.md`.

## Global Constraints

- Generated files, fixtures, messages, and commits must not identify the organization that supplied the source brief.
- API instants require an explicit UTC offset; persisted instants use UTC; intervals are half-open `[start, end)`.
- A technician must possess every skill required by the selected service type.
- Only active dealerships, vehicles, service types, technicians, skills, and bays participate.
- Confirmation derives duration from the service type and atomically allocates both resources.
- PostgreSQL constraints remain the final concurrency authority; availability never reserves resources.
- Use stable fictional UUIDs and neutral fixture names so tests and the browser flow are deterministic.

## Review Focus

- An interval ending exactly at another start must succeed, while any positive overlap must fail for either resource.
- DST transitions and offset-bearing timestamps must not be silently reinterpreted as local wall time.
- Services requiring zero skills must not accidentally qualify every technician; reject such invalid reference data at the persistence boundary.
- Concurrent confirmations and simultaneous idempotent retries must never create duplicate appointments or partial idempotency records.
- Availability must use bounded set-based reads and deterministic ordering, without one query per candidate slot.

---

### Task 1: Database overlap constraints, indexes, and deterministic fixtures

**Files:**
- Create: `api/migrations/002_booking_invariants.sql`
- Create: `api/migrations/003_seed_demo.sql`
- Create: `api/internal/postgres/constraints_test.go`
- Create: `api/internal/postgres/seed_test.go`
- Create: `api/internal/postgres/query_plan_test.go`
- Modify: `api/internal/postgres/schema_test.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces partial GiST constraints `appointments_technician_no_overlap` and `appointments_bay_no_overlap` over confirmed half-open ranges.
- Produces stable fictional fixtures referenced by later integration and E2E tests.
- Produces indexes for bounded appointment scans, active resources, qualification joins, and identifier lookup.

- [ ] **Step 1: Write failing constraint tests**

Add tests that insert a confirmed appointment, then assert technician overlap and bay overlap return PostgreSQL exclusion violations; exact boundary-touching intervals and overlapping `CANCELLED` rows succeed. Assert stored `tstzrange(start_at, end_at, '[)')` has inclusive lower and exclusive upper bounds.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -run 'TestAppointment(Interval|Overlap|Boundary|Cancelled)' -v`

Expected: FAIL because exclusion constraints are absent.

- [ ] **Step 3: Add the invariant migration**

Create partial GiST exclusion constraints using `tstzrange(start_at, end_at, '[)') WHERE (status = 'CONFIRMED')`. Add explicit indexes for dealership/start/end scans, active technician and bay lookup, technician skills, required skills, and appointment identifiers.

- [ ] **Step 4: Add query-plan assertions**

Seed enough deterministic rows inside a rolled-back test transaction, disable sequential scans only for the assertion, and use `EXPLAIN (FORMAT JSON)` to prove bounded availability predicates can use the appointment interval and qualification indexes. Do not assert unstable cost values or exact planner node nesting.

- [ ] **Step 5: Write repeatable seed tests and verify RED**

Assert two active bays, at least two qualified technicians, one unqualified technician, inactive records of each relevant resource type, one occupied interval, complete business hours, and valid ownership/skill relations. Apply the seed twice and assert counts and identifiers do not change.

- [ ] **Step 6: Add fictional seed migration**

Use fixed UUIDs and neutral names such as `Riverside Service Centre`, `Taylor Morgan`, and `Bay A`; use `INSERT ... ON CONFLICT ... DO UPDATE` or equivalent so reruns converge. Fix the occupied appointment to a future UTC date used consistently by E2E fixtures.

- [ ] **Step 7: Verify database behavior and mark OpenSpec tasks**

Run migrations twice, then run `go test ./internal/postgres -run 'Test(Appointment|QueryPlan|Seed)' -v`. Expected: PASS. Mark 2.3, 2.4, 2.5, and 2.6 complete only after the assertions pass.

- [ ] **Step 8: Commit**

```bash
git add api/migrations api/internal/postgres openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: enforce booking database invariants"
```

### Task 2: Domain time, qualification, and deterministic ordering

**Files:**
- Create: `api/internal/domain/time.go`
- Create: `api/internal/domain/time_test.go`
- Create: `api/internal/domain/resources.go`
- Create: `api/internal/domain/resources_test.go`
- Modify: `api/internal/domain/dependencies_test.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces `domain.ParseOffsetDateTime(raw string) (time.Time, error)`.
- Produces `domain.NewDuration(minutes int) (domain.Duration, error)` and `Duration.End(start time.Time) time.Time`.
- Produces `domain.NewInterval(start, end time.Time) (domain.Interval, error)`, `Interval.Overlaps(Interval) bool`, and `Interval.Within(Interval) bool`.
- Produces `domain.IsGridAligned(time.Time, intervalMinutes int) bool`.
- Produces `domain.Technician.QualifiedFor(required []domain.SkillID) bool` and deterministic `domain.OrderTechnicians` / `domain.OrderBays` by future load then stable identifier.

- [ ] **Step 1: Write failing time-rule tests**

Cover `Z`, positive, and negative offsets; reject offset-free strings and malformed offsets; reject nonpositive duration and reversed/zero intervals; prove half-open boundary behavior; prove 30-minute grid alignment; prove full containment within business hours including exact open/close boundaries.

- [ ] **Step 2: Run time tests and verify RED**

Run: `cd api && go test ./internal/domain -run 'Test(ParseOffset|Duration|Interval|Grid|BusinessHours)' -v`

Expected: FAIL because the value objects do not exist.

- [ ] **Step 3: Implement immutable time value objects**

Keep fields private where invalid zero values would bypass validation. Normalize instants to UTC after parsing while retaining the rule that the input text contains `Z` or a numeric offset.

- [ ] **Step 4: Write failing resource-policy tests**

Cover all-required-skills qualification, missing one skill, inactive technician/bay exclusion, zero required skills rejected by policy input, load ordering, stable-ID tie breaking, and deterministic results from differently ordered input slices.

- [ ] **Step 5: Implement resource policies**

Use set membership for skills and copy before sorting so caller slices are not mutated. Keep the package standard-library-only.

- [ ] **Step 6: Verify domain boundaries and mark OpenSpec tasks**

Run: `cd api && gofmt -w internal/domain && go test ./internal/domain -v && go vet ./internal/domain`. Expected: PASS, including the dependency guard. Mark 3.1–3.4 complete.

- [ ] **Step 7: Commit**

```bash
git add api/internal/domain openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: add booking domain rules"
```

### Task 3: Set-based advisory availability service

**Files:**
- Create: `api/internal/application/availability.go`
- Create: `api/internal/application/availability_test.go`
- Create: `api/internal/application/errors.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces `application.AvailabilityRepository.LoadAvailabilityContext(ctx context.Context, query application.AvailabilityQuery) (application.AvailabilityContext, error)` as the single repository call per request.
- Produces `application.AvailabilityService.AvailableSlots(ctx context.Context, query application.AvailabilityQuery) ([]domain.Interval, error)`.
- `AvailabilityContext` contains validated active references, dealership-local business hours, service duration/required skills, active technicians/bays with future load, and confirmed busy intervals for the bounded date window.

- [ ] **Step 1: Write the failing availability tests**

Use a counting fake repository. Cover complete resource pairs, technician partial overlap, bay partial overlap, boundary touching, no qualified technician, no active bay, inactive resources, duration crossing closing time, empty hours, multiple required skills, deterministic results, and exactly one repository call regardless of slot count.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd api && go test ./internal/application -run TestAvailability -v`

Expected: FAIL because the service and interface do not exist.

- [ ] **Step 3: Implement slot generation and orchestration**

Generate 30-minute starts across each applicable dealership-local business interval and include only intervals fully contained in hours with at least one qualified technician and one bay free for the full duration. Return UTC instants in stable ascending order. Validate active references through the loaded context and return typed validation errors.

- [ ] **Step 4: Verify service behavior and package direction**

Run: `cd api && gofmt -w internal/application && go test ./internal/application ./internal/domain -v && go vet ./...`. Expected: PASS; the repository call-count test remains one.

- [ ] **Step 5: Mark OpenSpec tasks and commit**

Mark 3.5 and 3.6 complete.

```bash
git add api/internal/application openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: compute booking availability"
```

### Task 4: Atomic PostgreSQL allocation and bounded contention handling

**Files:**
- Create: `api/migrations/004_confirm_appointment.sql`
- Create: `api/internal/application/booking.go`
- Create: `api/internal/postgres/booking.go`
- Create: `api/internal/postgres/booking_test.go`
- Create: `api/internal/postgres/concurrency_test.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces `application.ConfirmCommand{VehicleID, DealershipID, ServiceTypeID string; StartAt time.Time}`.
- Produces `application.BookingGateway.Confirm(ctx context.Context, command application.ConfirmCommand) (application.Appointment, error)`.
- Produces typed outcomes `application.ErrInvalidReference`, `application.ErrResourceConflict`, and `application.ErrPersistence` without exposing PostgreSQL error text.

- [ ] **Step 1: Write failing allocation integration tests**

Cover successful allocation, customer derived from vehicle, computed end time, all-required-skill selection, deterministic least-load assignment, invalid ownership, inactive records, no eligible pair, occupied technician, occupied bay, and transaction rollback leaving zero appointments after any failure.

- [ ] **Step 2: Run allocation tests and verify RED**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -run TestConfirm -v`

Expected: FAIL because the booking gateway/function does not exist.

- [ ] **Step 3: Implement the transactional booking function and adapter**

Lock and validate the vehicle/customer, dealership, and service type; derive duration; select active qualified technicians and active bays ordered by future confirmed load then UUID; attempt pairs in stable order; insert a confirmed appointment; return the full typed row. Never accept `end_at`, customer, technician, or bay from the caller.

- [ ] **Step 4: Write the concurrent contention test**

Seed exactly one eligible technician and bay, release 20 goroutines together with distinct commands for the same interval, and assert one success, 19 `ErrResourceConflict` results, one persisted row, and no leaked raw database error.

- [ ] **Step 5: Add bounded retry and exclusion mapping**

Retry alternative deterministic pairs only on the named exclusion constraints, cap attempts to the finite candidate-pair count, and map exhaustion to `ErrResourceConflict`. Preserve context cancellation and map unrelated database errors to `ErrPersistence`.

- [ ] **Step 6: Verify repeatedly and mark OpenSpec tasks**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./internal/postgres -run 'Test(Confirm|Concurrent)' -count=20`. Expected: PASS with at most one winner in every run. Mark 4.1–4.4 complete.

- [ ] **Step 7: Commit**

```bash
git add api/migrations api/internal/application api/internal/postgres openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: confirm appointments atomically"
```

### Task 5: Transactional idempotency and typed read adapters

**Files:**
- Create: `api/internal/application/idempotency.go`
- Create: `api/internal/postgres/idempotency_test.go`
- Create: `api/internal/postgres/queries.go`
- Create: `api/internal/postgres/queries_test.go`
- Modify: `api/internal/postgres/booking.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Extends `application.ConfirmCommand` with `IdempotencyKey string` at the application boundary.
- Produces canonical `application.HashConfirmCommand(command ConfirmCommand) [32]byte`, excluding the key itself and normalizing the instant to UTC.
- Produces `postgres.Repository.BookingOptions`, `LoadAvailabilityContext`, `AppointmentByID`, and idempotent `Confirm` methods consumed by later HTTP handlers.
- Produces typed `application.ErrIdempotencyConflict` and `application.ErrNotFound`.

- [ ] **Step 1: Write failing idempotency tests**

Cover first success, sequential identical replay, simultaneous identical replay, same key with changed vehicle/dealership/service/start, failed allocation leaving no committed key, and canonical equality for equivalent offset representations of one instant.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test ./internal/postgres -run TestIdempotency -v`

Expected: FAIL because confirmation does not coordinate idempotency records.

- [ ] **Step 3: Implement transactional idempotency**

Hash length-prefixed normalized fields with SHA-256. Claim the key inside the same transaction as allocation; serialize identical-key requests; return the original persisted appointment for the same hash and `ErrIdempotencyConflict` for a different hash. Never leave a null appointment record after failure.

- [ ] **Step 4: Write failing read-adapter integration tests**

Assert booking options exclude inactive rows; availability context is bounded to one dealership/date window and carries all qualification/busy data in set-based queries; appointment retrieval returns persisted names and interval after closing the creating connection and opening a new pool; unknown identifiers return `ErrNotFound`.

- [ ] **Step 5: Implement typed read adapters**

Use explicit SQL and row scanners. Keep registration available only for the API response model and out of logs. Avoid N+1 reads by aggregating skills and intervals in bounded queries.

- [ ] **Step 6: Run the booking-core release gate**

Run: `pnpm stack:reset && pnpm stack:start && (cd api && TEST_DATABASE_URL=postgresql://scheduler:scheduler@localhost:55432/scheduler go test -race ./... -count=1 && go vet ./... && go build ./cmd/server ./cmd/migrate) && openspec validate bootstrap-service-scheduler --strict`

Expected: every command exits 0 and a second migration/seed run changes no fixture counts.

- [ ] **Step 7: Mark OpenSpec tasks and commit**

Mark 4.5–4.7 complete only after the full integration assertions pass.

```bash
git add api/internal/application api/internal/postgres openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: add idempotent booking persistence"
```
