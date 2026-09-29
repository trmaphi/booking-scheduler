# Scheduler demonstration script

Target duration: **8 minutes 45 seconds**. Start from a clean delivery-gate run with the local stack left healthy. Keep one terminal on API JSON logs and one browser at `http://localhost:3000`.

## 0:00–0:50 — Architecture

- Show the README diagram: Next.js, Go REST API, and PostgreSQL.
- Explain that the OpenAPI contract generates the TypeScript client.
- Point out that availability is advisory and confirmation is the transactional correctness boundary.

## 0:50–2:40 — Book and prove persistence

- Select the fictional vehicle, service centre, service, and `2030-01-03`.
- Find slots, select the first slot, and review the duration and interval.
- Note that resources are intentionally absent before confirmation.
- Confirm, show the assigned technician and bay, copy the appointment URL, then reload it.
- Explain that reload calls the retrieval endpoint and proves durable state rather than browser-only state.

## 2:40–3:50 — Stale and concurrent conflict behavior

- Explain the half-open interval rule: adjacent work can share a boundary, overlapping confirmed work cannot.
- Describe the browser’s stale-slot behavior: a 409 refreshes availability while preserving still-valid selections.
- Show the repeated concurrency command in the README. It launches overlapping confirmations against constrained resources and checks that at most one wins.
- Point to PostgreSQL exclusion constraints and bounded allocation retry as the final guard against double booking.

## 3:50–5:05 — Test pyramid and release gate

- Show `pnpm verify:delivery` in the README and the latest evidence in `docs/verification.md`.
- Summarize unit tests for interval, qualification, ordering, HTTP validation, trace parsing, and privacy.
- Summarize PostgreSQL integration tests for migrations, query plans, rollback, idempotency, and contention.
- Summarize Playwright’s full selection, confirmation, reload, and API read-back journey.

## 5:05–6:10 — Privacy-safe observability

- Trigger `curl --fail http://localhost:8080/api/v1/health/ready` and one availability request.
- Show correlated JSON events in `pnpm stack:logs`.
- Highlight route template, result, request ID, trace ID, and duration.
- Explain that tests reject personal details, registrations, raw bodies, identifiers, keys, raw database errors, and connection strings.

## 6:10–7:35 — Decisions, alternatives, and limits

- Database constraints beat application-only locks for cross-process correctness, but use PostgreSQL-specific features.
- Synchronous confirmation is simpler than a worker for this bounded transaction; a queue becomes useful for reminders or external side effects.
- Deterministic allocation is testable; a fairness policy would require explicit operational goals.
- Local JSON telemetry needs no collector; production export is an optional adapter.
- Production hosting remains undecided so the repository makes no provider commitment.

## 7:35–8:45 — Lessons and close

- The key lesson is that checking availability and inserting later is unsafe; the database must arbitrate the final allocation.
- Specification assumptions were made explicit so alternatives could be reviewed before code.
- Generated contracts, empty-volume verification, confidentiality scanning, and independent reviews turn narrative claims into repeatable evidence.
- End on the reloaded confirmed appointment and invite questions.

The timeboxes total 8:45, leaving 1:15 of a ten-minute slot for normal navigation variation.
