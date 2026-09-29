# Tasks

## 1. Project Foundation

- [x] 1.1 Preserve the strict TypeScript Next.js frontend and scaffold a Go module with formatting, vet, lint, unit-test, and build commands; verify clean installs and quality commands for both applications.
- [x] 1.2 Create Go package boundaries for domain, application, PostgreSQL adapters, and HTTP transport plus the existing Next.js UI boundary; verify dependency rules prevent domain packages from importing HTTP or database clients.
- [x] 1.3 Add Go environment validation and documented variables for the frontend API origin and local PostgreSQL connection; verify overrides remain untracked and startup fails with a safe actionable error when required variables are absent.
- [x] 1.4 Extend Docker Compose with development services for Next.js, the Go API, and PostgreSQL plus Go migration commands; verify all services start, support source-driven rebuilds, and report ready without a hosted account.

## 2. Database Model and Invariants

- [x] 2.1 Port migration tests to Go for required extensions, reference tables, foreign keys, positive service duration, active flags, business hours, appointment status, and idempotency records; verify they fail before adding the migration.
- [x] 2.2 Add the initial SQL migration and verify it applies from an empty local PostgreSQL volume, including successful `btree_gist` creation and Go schema tests.
- [x] 2.3 Write database tests for half-open appointment ranges and technician/bay overlap rejection, including back-to-back appointments; verify they fail before adding exclusion constraints.
- [x] 2.4 Add partial GiST exclusion constraints for confirmed technician and bay allocations; verify overlapping confirmed appointments fail while boundary-touching and cancelled appointments succeed.
- [x] 2.5 Add indexes for dealership/date interval scans, active resources, technician skills, service requirements, and identifiers; verify query plans use bounded indexed access for the seeded availability workload.
- [x] 2.6 Add plainly fictional seed fixtures covering multiple bays, qualified and unqualified technicians, occupied intervals, and inactive records; verify seeds are repeatable and reference-integrity checks pass.

## 3. Domain Rules

- [x] 3.1 Write failing Go unit tests for explicit-offset timestamps, half-open overlap semantics, positive durations, slot-grid alignment, and complete containment in business hours.
- [x] 3.2 Implement focused Go time and interval value objects until the time-rule tests pass and invalid construction paths are unavailable outside the domain package.
- [x] 3.3 Write failing Go unit tests for all-required-skill qualification, inactive resource filtering, and deterministic resource ordering by future load then stable identifier.
- [x] 3.4 Implement Go qualification and ordering policies until the domain tests pass without importing HTTP or database packages.
- [x] 3.5 Write failing Go availability-service tests covering complete resource pairs, partial overlaps, no qualified technicians, no bays, inactive records, and cross-boundary durations.
- [x] 3.6 Implement Go slot generation and availability orchestration against repository interfaces until all availability-service tests pass without per-slot repository calls.

## 4. Atomic Booking and Persistence

- [x] 4.1 Write failing Go PostgreSQL integration tests for successful allocation, no eligible pair, invalid ownership, inactive records, derived end time, and full rollback after failure.
- [x] 4.2 Add the transactional booking database function and typed Go gateway until the allocation and rollback tests pass.
- [x] 4.3 Write a concurrent integration test that launches overlapping confirmations against the only eligible technician and bay; verify the unprotected implementation demonstrates contention and the final implementation permits at most one confirmation.
- [x] 4.4 Complete bounded allocation retry and exclusion-violation mapping; run the concurrent test repeatedly and verify zero double bookings and stable conflict results.
- [x] 4.5 Write failing idempotency tests for first success, identical replay, simultaneous identical requests, and key reuse with changed input.
- [x] 4.6 Implement request hashing and transactional idempotency records; verify retries return the original appointment and mismatched reuse changes no data.
- [x] 4.7 Add typed Go read adapters for booking options, bounded availability queries, and appointment retrieval; verify integration tests exclude inactive records and return persisted assignments after a new application connection.

## 5. REST API

- [x] 5.1 Define OpenAPI components for identifiers, offset timestamps, slots, appointments, and stable JSON errors; validate the document with an OpenAPI parser.
- [x] 5.2 Write failing Go HTTP tests for booking options and availability, including unknown/inactive references, missing offsets, slot misalignment, CORS, and empty results.
- [x] 5.3 Implement Go handlers for `GET /api/v1/booking-options` and `GET /api/v1/availability` with request IDs and schema validation; verify their HTTP tests and OpenAPI examples pass.
- [x] 5.4 Write failing Go HTTP tests for appointment confirmation and retrieval, including 201 success, idempotent replay, changed-input conflict, stale-availability conflict, invalid input, and not found.
- [x] 5.5 Implement Go handlers for `POST /api/v1/appointments` and `GET /api/v1/appointments/{appointmentId}` with stable error mapping; verify HTTP tests and contract examples pass.
- [x] 5.6 Add Go liveness and database-backed readiness handlers; verify liveness survives a database outage while readiness reports unavailable without leaking connection details.

## 6. Booking Interface

- [x] 6.1 Build the accessible booking form for vehicle, dealership, service type, date, and returned time slots; verify keyboard navigation, labels, loading, empty, and validation states with component tests.
- [x] 6.2 Add a confirmation review that states the service duration and selected interval without promising specific resources; verify a stale-slot conflict refreshes availability and preserves valid selections.
- [x] 6.3 Replace the mock with a generated TypeScript client for the Go API and add the confirmed appointment view showing persisted vehicle, service, dealership, technician, bay, status, and interval; verify reloading retrieves the same appointment.
- [x] 6.4 Add one Playwright journey from seeded selection through confirmation and page reload; verify it passes against a clean migrated local database.

## 7. Observability and Operational Safety

- [x] 7.1 Write Go telemetry tests that detect customer names, contact details, registrations, raw bodies, and direct customer/vehicle identifiers in booking logs.
- [x] 7.2 Add correlated structured Go request and booking logs with safe dimensions; verify success, validation failure, and database conflict events pass the privacy tests.
- [x] 7.3 Add Go request duration/error, availability result, confirmation, conflict, retry, and database duration metrics behind a small telemetry interface; verify deterministic metric events in HTTP and integration tests.
- [x] 7.4 Propagate W3C trace context in Go and document optional OpenTelemetry export without requiring a collector; verify incoming trace IDs correlate HTTP and database-span test events.

## 8. Documentation and Delivery Verification

- [x] 8.0 Verify the local Compose foundation from a clean volume with web/API health and migrated-schema checks; document the local workflow and provider-neutral status.
- [x] 8.1 Write the README with architecture summary, Docker prerequisites, local Compose setup, migration/seed commands, build/run/test commands, REST examples, reset workflow, and known trade-offs; verify every documented local command on a clean checkout.
- [x] 8.2 Add an AI collaboration narrative describing requirement extraction, assumption review, decision ownership, prompts at a high level, and checks actually performed; verify it makes no unsupported quality claims.
- [x] 8.3 Add a concise demonstration script covering architecture, booking, persistence after reload, conflict behavior, tests, observability, trade-offs, and lessons; verify a rehearsal fits within ten minutes.
- [x] 8.4 Run Go formatting, vet, lint, unit and PostgreSQL integration tests, TypeScript formatting/lint/type checks, Go HTTP tests, Playwright, both production builds, migration-from-empty, OpenSpec strict validation, and a clean-volume Compose smoke booking; record exact results in the README.
- [x] 8.5 Scan tracked files and Git history for prohibited source-identifying names and non-fictional fixtures; verify the scan is clean before marking the change ready for archive.
