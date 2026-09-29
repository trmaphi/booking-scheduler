# Observability and Delivery Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add privacy-safe operational evidence to booking requests, document the finished system and collaboration process, and produce a clean, reproducible delivery whose tracked files and Git history contain no source-identifying material.

**Architecture:** A small Go telemetry port receives typed events from HTTP and PostgreSQL boundaries. The default local implementation writes structured JSON logs and in-process metric events while propagating standards-compliant W3C trace context; optional OpenTelemetry export remains an adapter point rather than a runtime dependency. Documentation records only checks actually rerun on the final commit.

**Tech Stack:** Go 1.27 standard `log/slog`, W3C Trace Context, existing `net/http`/`pgx`, Docker Compose, Markdown, Git history inspection.

**Spec:** `openspec/changes/bootstrap-service-scheduler/design.md`; observable behavior is in `openspec/changes/bootstrap-service-scheduler/specs/service-appointment-booking/spec.md`.

## Global Constraints

- Logs and metric attributes must never include customer names, contact details, registrations, raw request bodies, direct customer IDs, direct vehicle IDs, database URLs, or idempotency keys.
- Safe dimensions are bounded enums or identifiers already permitted by D9: route, status/result, request ID, trace ID, dealership ID, service-type ID, conflict class, retry count, duration, and availability result count.
- Trace context follows W3C `traceparent`; invalid or unsupported input is replaced safely and never echoed blindly.
- Telemetry failure must not fail booking requests or expose internals.
- No collector, SaaS account, worker, or hosted provider is required for the local system.
- Documentation must distinguish verified behavior from future options and must not identify the source organization.
- Final verification must run against the exact final commit from empty local volumes and leave the working tree clean.

## Review Focus

- Validation failures occurring before JSON decode must still emit one safe completion event without request contents.
- Concurrent requests must not mix request, trace, database, or metric attributes.
- Attacker-controlled request IDs, trace headers, paths, and errors must be bounded/validated before logging.
- Database cancellation/outage/conflict events must preserve safe classification while never serializing raw `pgx` errors or connection details.
- History cleanup must preserve the finished source and tags/branches intended for delivery while removing prohibited blobs and text from every reachable commit.

---

### Task 1: Privacy-safe structured request and booking logs

**Files:**
- Create: `api/internal/telemetry/telemetry.go`
- Create: `api/internal/telemetry/logger.go`
- Create: `api/internal/telemetry/logger_test.go`
- Create: `api/internal/httpapi/telemetry_test.go`
- Modify: `api/internal/httpapi/router.go`
- Modify: `api/internal/httpapi/booking_options.go`
- Modify: `api/internal/httpapi/availability.go`
- Modify: `api/internal/httpapi/appointments.go`
- Modify: `api/internal/application/booking.go`
- Modify: `api/internal/postgres/booking.go`
- Modify: `api/cmd/server/main.go`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces a narrow `telemetry.Recorder` with typed request, availability, confirmation, database, and metric methods; `telemetry.Nop()` is safe by default.
- Produces JSON `slog` events with bounded event names and typed safe fields.
- Carries request ID and trace correlation through context, never as package globals.

- [ ] **Step 1: Write failing privacy tests**

Construct success, malformed JSON, validation failure, not found, database outage, and resource conflict requests containing unique sentinel customer names, emails, registrations, customer/vehicle IDs, idempotency keys, raw bodies, URL credentials, and raw database errors. Capture all logs and assert none of the sentinels appear in keys or values.

- [ ] **Step 2: Write failing event-shape tests**

Assert exactly one bounded request-completion event per request with request ID, method, route template, HTTP status, result category, and nonnegative duration. Assert availability/confirmation events include only dealership ID, service-type ID, result, count/retry, request ID, and trace ID; validation and conflict paths remain correlated.

- [ ] **Step 3: Run tests and verify RED**

Run: `cd api && go test ./internal/telemetry ./internal/httpapi -run 'Test(Telemetry|Privacy|Structured)' -v`

Expected: FAIL because telemetry ports and events do not exist.

- [ ] **Step 4: Implement typed telemetry and request middleware**

Use immutable context values and `slog` JSON output. Map raw paths to known route templates before recording; cap string dimensions; classify errors through typed application outcomes. Recover from recorder panics or failures so telemetry cannot change HTTP behavior.

- [ ] **Step 5: Emit availability, confirmation, conflict, retry, and database events**

Instrument boundaries where the values are known. Pass only safe IDs and enums, never whole commands, DTOs, errors, or request objects. Ensure an idempotent replay is distinguishable from a first confirmation without logging the key.

- [ ] **Step 6: Verify concurrency and privacy**

Run the event tests with `-race` and parallel requests; assert each request/trace pair stays isolated and every response retains the matching request ID.

- [ ] **Step 7: Mark OpenSpec tasks and commit**

Run full Go tests with PostgreSQL, race checks, vet/build, and strict OpenSpec validation. Mark 7.1 and 7.2 complete.

```bash
git add api openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: add privacy-safe booking telemetry"
```

### Task 2: Deterministic metrics and W3C trace propagation

**Files:**
- Create: `api/internal/telemetry/metrics.go`
- Create: `api/internal/telemetry/metrics_test.go`
- Create: `api/internal/telemetry/trace.go`
- Create: `api/internal/telemetry/trace_test.go`
- Modify: `api/internal/telemetry/telemetry.go`
- Modify: `api/internal/httpapi/router.go`
- Modify: `api/internal/postgres/booking.go`
- Modify: `api/internal/postgres/queries.go`
- Modify: `api/cmd/server/main.go`
- Modify: `README.md`
- Modify: `.env.example`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces deterministic counter/histogram events for HTTP duration/errors, availability result count, confirmation/replay, resource/idempotency conflict, allocation retry, and database duration.
- Produces strict W3C `traceparent` parsing/formatting and context propagation; valid incoming trace IDs are preserved while a new span ID is generated.
- Documents an optional telemetry adapter/exporter seam without requiring an OpenTelemetry collector or changing local startup.

- [ ] **Step 1: Write failing metric-event tests**

Use an in-memory recorder to assert exact event name, unit, value, and bounded attributes for success, validation, no availability, created, replay, resource conflict, idempotency conflict, retry, not found, cancellation, and database error paths. Use a fake clock for deterministic duration values.

- [ ] **Step 2: Write failing trace-context tests**

Cover valid version `00`, lowercase canonical output, all-zero trace/span rejection, unsupported versions, malformed length/hex/flags, forbidden extra fields for version `00`, sampled flag propagation, generated child span uniqueness, and response `traceparent` correlation.

- [ ] **Step 3: Run tests and verify RED**

Run: `cd api && go test ./internal/telemetry ./internal/httpapi ./internal/postgres -run 'Test(Metric|Trace)' -v`

Expected: FAIL because metric recording and trace parsing are absent.

- [ ] **Step 4: Implement metrics behind the existing recorder**

Record duration in milliseconds and counters as unitless increments. Keep attribute cardinality bounded; route uses templates, status is numeric, and database operation is a fixed enum. No direct customer/vehicle identifier may enter metric attributes.

- [ ] **Step 5: Implement trace propagation**

Parse one unambiguous `traceparent`, create a new trace for missing/invalid input, create a child span ID per request, return the canonical header, and attach trace ID/span ID to context for HTTP and database event correlation. Do not accept `tracestate` as an attribute or log value.

- [ ] **Step 6: Document the optional exporter seam**

Explain that local JSON events require no collector. Document where a future OpenTelemetry adapter would implement `telemetry.Recorder`, with trade-offs and no unsupported claim that export is already configured.

- [ ] **Step 7: Verify and commit**

Run full Go DB/race suites and a live request with a known trace ID; prove HTTP and database events share it, secrets are absent, and local Compose works without telemetry environment values. Mark 7.3 and 7.4 complete.

```bash
git add api README.md .env.example openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "feat: add booking metrics and trace context"
```

### Task 3: Delivery documentation, final verification, and confidentiality audit

**Files:**
- Modify: `README.md`
- Create: `docs/ai-collaboration.md`
- Create: `docs/demo-script.md`
- Create: `docs/verification.md`
- Create: `scripts/verify-delivery.mjs`
- Create: `scripts/confidentiality-scan.mjs`
- Modify: `package.json`
- Modify: `.gitignore`
- Modify: `openspec/changes/bootstrap-service-scheduler/tasks.md`

**Interfaces:**
- Produces `pnpm verify:delivery`, the single empty-volume final gate covering generated client consistency, frontend checks/build, Go formatting/vet/unit/HTTP/PostgreSQL/race/build, migrations/seeds, smoke, Playwright booking, OpenSpec strict validation, confidentiality scan, and clean Git state.
- Produces `pnpm scan:confidentiality` over tracked files and all reachable Git objects, rejecting PDFs, configured prohibited names/case variants, source-document fingerprints, non-fictional fixtures, secrets, and provider commitments.
- Produces a reviewer-ready README, truthful AI collaboration narrative, and a timed sub-ten-minute demo script.

- [ ] **Step 1: Write failing delivery-script contract tests**

Assert the verification script includes every OpenSpec 8.4 command class, runs sequentially, propagates failures, starts from empty volumes, and checks the tree after generated/build artifacts. Assert the confidentiality scanner detects forbidden text in a tracked file, commit message/blob, PDF path/blob, credential pattern, and non-fictional fixture allowlist violation using disposable Git repositories.

- [ ] **Step 2: Implement the delivery and confidentiality scripts**

Use argument arrays rather than shell interpolation for untrusted text. Keep prohibited tokens in a local untracked input or encoded detector so the repository itself does not contain them. Scan commit messages, paths, and blob content from every reachable ref; return only safe category/path/object summaries.

- [ ] **Step 3: Complete the README**

Document architecture, prerequisites, local Compose setup, ports, migration/seed behavior, generated client, every build/test command, REST examples with fictional values, reset data-loss semantics, observability, troubleshooting, known trade-offs, and undecided production hosting. Test every documented command.

- [ ] **Step 4: Write the AI collaboration narrative**

Describe constraint extraction, assumptions and alternatives, human-owned design choices, high-level prompt categories, subagent review workflow, defects found during review, and checks actually performed. Do not reproduce or name the source brief and do not claim checks beyond recorded evidence.

- [ ] **Step 5: Write and rehearse the demo script**

Cover architecture, booking, persisted reload, stale/concurrent conflict behavior, test pyramid, privacy-safe observability, trade-offs, alternatives, and lessons. Record section timeboxes totaling no more than ten minutes and rehearse commands against the final stack.

- [ ] **Step 6: Clean tracked files and Git history**

Run the scanner. If any reachable commit contains prohibited source-identifying text, PDFs, or obsolete provider/team configuration, rewrite local history once so the delivered branch begins from a clean root or contains only sanitized commits. Preserve the current final tree, conventional commit subjects, and a backup ref outside pushed refs until validation succeeds. Re-run the scanner over all refs intended for push.

- [ ] **Step 7: Run the exact final gate from empty volumes**

Run `pnpm verify:delivery` on the final commit. Record UTC date, commit hash, tool versions, command groups, test counts, and pass/fail results in `docs/verification.md` and a concise README section. Re-run the documentation-only checks if recording results changes tracked files, then commit and verify the tree is clean.

- [ ] **Step 8: Mark OpenSpec tasks and commit**

Mark 8.1–8.5 complete only when documentation, rehearsal, exact gate evidence, and all-ref confidentiality scan pass.

```bash
git add README.md docs scripts package.json .gitignore openspec/changes/bootstrap-service-scheduler/tasks.md
git commit -m "docs: complete scheduler delivery evidence"
```
