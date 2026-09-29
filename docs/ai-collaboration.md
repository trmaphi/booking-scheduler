# AI collaboration record

AI assistance was used as an implementation and review tool. The human owner chose the scheduling scenario, required a Go backend and Next.js interface, selected local Docker Compose as the development target, deferred production hosting, and required both specification-first work and confidentiality controls.

## From constraints to design

The first pass separated explicit constraints from assumptions. Explicit constraints became OpenSpec requirements and executable checks: a vehicle, service type, service centre, and desired time; a qualified technician and bay for the full duration; durable confirmation; concurrency protection; and a provider-neutral local environment. Assumptions such as a 30-minute slot grid, half-open intervals, complete business-hours containment, deterministic resource ordering, and idempotency-key semantics were documented with alternatives and trade-offs before implementation.

Human-owned decisions included:

- PostgreSQL as the authority for overlapping-resource protection.
- A synchronous Go REST API instead of a queue or worker for the booking path.
- Next.js as a separate typed client of the OpenAPI contract.
- Local Compose as the only committed deployment topology.
- Privacy-safe structured telemetry with an optional exporter seam.

Prompt categories stayed at the level of outcomes and review criteria: extract constraints, compare architecture alternatives, draft a change specification, implement one bounded task with tests, inspect concurrency behavior, review a diff against the spec, and verify the complete local journey. This document does not reproduce or name the source material.

## Implementation and review workflow

Work was divided into small plans for the local foundation, database/domain/booking core, REST and UI integration, and observability/delivery. Implementer agents worked in an isolated Git worktree. Each bounded task used failing tests before production changes where behavior was introduced. Separate review agents compared the resulting diff with the plan and specification; important findings were returned for a focused fix and re-review.

Review found concrete defects that changed the implementation, including incomplete graceful shutdown, an unenforced package-boundary rule, a malformed-origin edge case, stale dependencies in a named web volume, domain timestamp validation edge cases, transactional booking race behavior, HTTP contract mismatches, telemetry panic behavior, and missing evidence for metric/database correlation. The fixes were tested and reviewed rather than accepted from prose alone.

## Verification discipline

Claims in the README are limited to commands represented in the delivery gate and evidence recorded in `docs/verification.md`. The gate starts from empty Compose volumes and includes generated-client consistency, frontend checks and build, Go formatting/vet/build, PostgreSQL and race tests, repeated concurrency checks, smoke checks, a real browser booking with persisted reload, strict OpenSpec validation, confidentiality scanning, and a final clean-tree check.

AI output was treated as a draft until source review and executable verification supported it. The human owner retains responsibility for the design choices, delivered repository, production security review, and any future hosting decision.
