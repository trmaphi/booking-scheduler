# Proposal

## Why

Manual service booking cannot reliably coordinate a vehicle, service duration, qualified technician, and service bay under concurrent demand. This change establishes a small but production-minded scheduler whose correctness, persistence, and engineering decisions can be demonstrated locally and on free hosting tiers.

## What Changes

- Add a customer-facing flow for selecting a vehicle, dealership, service type, date, and available start time.
- Add availability calculation across business hours, service duration, technician qualifications, existing appointments, and service-bay occupancy.
- Add atomic appointment confirmation that selects a qualified technician and compatible bay without allowing either resource to be double-booked.
- Persist customers, vehicles, dealerships, service types, technicians, skills, bays, and confirmed appointments in PostgreSQL.
- Expose a documented REST API and a minimal web interface that exercises the complete booking journey.
- Add repeatable seed data, migrations, automated business-rule and concurrency tests, operational telemetry, and build/run/test documentation.
- Record architectural constraints, assumptions, alternatives, decision trade-offs, and the process used to direct and verify AI-assisted work.

## Non-goals

- Authentication, authorization, staff administration, payments, reminders, calendar synchronization, and customer notifications.
- Appointment rescheduling, waitlists, multi-service bundles, technician preferences, cross-location staffing, or dynamic pricing.
- A native mobile client, multiple independently deployed services, or infrastructure beyond a small demonstration workload.

## Capabilities

### New Capabilities

- `service-appointment-booking`: Request, discover, atomically confirm, retrieve, and persist resource-constrained vehicle service appointments.

### Modified Capabilities

None. This is a greenfield project.

## Impact

- Creates a Next.js TypeScript frontend and a separately deployable Go REST API.
- Creates a PostgreSQL schema, migrations, seed data, transactional booking function, and overlap constraints verified against local PostgreSQL.
- Adds runtime and development dependencies for validation, database access, testing, and structured telemetry.
- Establishes Docker Compose as the documented development environment for the Next.js frontend, Go API, and PostgreSQL while leaving production providers undecided.
- Adds public API contracts, system-design documentation, an AI collaboration narrative, and test evidence.
