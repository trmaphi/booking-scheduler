# Spec Delta

## Purpose

Defines the observable behavior of a persistent vehicle-service scheduler that coordinates a qualified technician and a service bay for the full duration of an appointment, including under concurrent confirmation attempts.

## Source Constraints

The following constraints are direct requirements of the supplied brief:

- A user requests an appointment for a specific vehicle, service type, dealership, and desired time.
- Confirmation checks both a qualified technician and a service bay for the entire service duration.
- A successful appointment persistently associates the customer, vehicle, technician, and service bay.
- The implemented backend exposes a REST API and uses a persistent database.
- Core business logic has automated tests.
- The solution addresses scalability, performance, reliability, maintainability, and observability.

## Documented Assumptions

- Each service type has one fixed positive duration and one or more required skills.
- Each appointment contains one vehicle and one service type.
- A technician works at one dealership in this version and must have every skill required by the service type.
- Every active service bay at the selected dealership is compatible with every service type in this version.
- Dealership business hours determine which start times may be offered.
- API timestamps contain an explicit UTC offset; persisted instants are UTC.
- Appointment intervals are half-open: `[start, end)`. An appointment may begin exactly when another ends.
- Confirmed appointments reserve resources; cancelled appointments do not.
- Availability is advisory. Confirmation is the authoritative allocation attempt.
- The service assigns resources deterministically; customers do not select a technician or bay.

## ADDED Requirements

### Requirement: Reference data can drive a booking request
The system SHALL provide the active dealerships, customer vehicles, and service types required to form a valid booking request without exposing inactive records.

#### Scenario: Load booking reference data
- **WHEN** a client requests booking reference data
- **THEN** the system returns active dealerships, vehicles associated with their customers, and service types with positive durations

### Requirement: Availability covers the complete service interval
The system SHALL offer only start times for which at least one active service bay and at least one active technician with every required skill are simultaneously available at the selected dealership for the complete service duration.

#### Scenario: A complete resource pair is available
- **WHEN** a valid availability request has a full-duration gap for a qualified technician and an active bay
- **THEN** the requested start time is returned with its computed end time

#### Scenario: Technician is busy during part of the service
- **WHEN** every qualified technician overlaps any portion of the requested service interval
- **THEN** the requested start time is not returned

#### Scenario: Bay is busy during part of the service
- **WHEN** every active bay overlaps any portion of the requested service interval
- **THEN** the requested start time is not returned

#### Scenario: Appointment touches an existing boundary
- **WHEN** a candidate starts exactly when an existing appointment ends, or ends exactly when an existing appointment starts
- **THEN** the existing appointment does not count as an overlap

#### Scenario: Candidate exceeds business hours
- **WHEN** any part of the service interval falls outside the dealership's business hours
- **THEN** the requested start time is not returned

### Requirement: Availability requests are validated
The system MUST reject availability requests with an unknown or inactive dealership, vehicle, or service type; a timestamp without an explicit UTC offset; or a start time that does not align with the configured slot interval.

#### Scenario: Timestamp has no UTC offset
- **WHEN** a client requests availability using a local timestamp without an offset
- **THEN** the system returns a validation error and does not guess a timezone

#### Scenario: Reference record is inactive
- **WHEN** a client requests availability using an inactive dealership or service type
- **THEN** the system returns a validation error

### Requirement: Confirmation allocates both constrained resources atomically
The system MUST confirm an appointment only when it can persistently allocate one qualified technician and one active service bay for the complete interval in the same database transaction.

#### Scenario: Confirmation succeeds
- **WHEN** a valid confirmation request has at least one resource pair available at commit time
- **THEN** the system persists one confirmed appointment with its customer, vehicle, dealership, service type, technician, bay, start time, and computed end time

#### Scenario: Availability becomes stale
- **WHEN** a resource shown as available has been reserved before confirmation commits and no other resource pair remains
- **THEN** confirmation returns a resource-conflict response and persists no partial appointment

#### Scenario: Concurrent requests target the same resources
- **WHEN** two confirmation transactions contend for the same only-qualified technician or only-available bay over overlapping intervals
- **THEN** at most one transaction confirms an appointment using that resource over that interval

### Requirement: Confirmation is idempotent
The system MUST accept an idempotency key for confirmation and SHALL return the original successful result when the same key and same request are retried.

#### Scenario: Successful request is retried
- **WHEN** a client repeats an already successful confirmation using the same idempotency key and identical request data
- **THEN** the system returns the original appointment and creates no additional appointment

#### Scenario: Key is reused for different input
- **WHEN** a client reuses an idempotency key with different request data
- **THEN** the system returns a conflict and does not alter the original appointment

### Requirement: Confirmed appointments are durable and retrievable
The system SHALL expose a confirmed appointment after the confirming request ends and after the application process restarts.

#### Scenario: Retrieve a confirmed appointment
- **WHEN** a client requests an existing appointment by identifier
- **THEN** the system returns its customer, vehicle, dealership, service type, assigned technician, assigned bay, status, start time, and end time

#### Scenario: Appointment does not exist
- **WHEN** a client requests an unknown appointment identifier
- **THEN** the system returns a not-found response

### Requirement: REST errors are stable and diagnosable
The REST API SHALL return JSON errors with a stable machine-readable code, a safe human-readable message, and a request identifier.

#### Scenario: Resource conflict response
- **WHEN** confirmation cannot reserve a complete resource pair
- **THEN** the API returns HTTP 409 with code `RESOURCE_CONFLICT` and a request identifier

#### Scenario: Invalid request response
- **WHEN** request validation fails
- **THEN** the API returns HTTP 400 with code `VALIDATION_ERROR` and field-level details

### Requirement: Booking operations emit operational evidence
The system SHALL emit structured operational signals for availability and confirmation without logging customer names, registration numbers, or other unnecessary personal data.

#### Scenario: Confirmation is processed
- **WHEN** a confirmation attempt completes
- **THEN** logs and metrics record the request identifier, result category, duration, dealership identifier, service-type identifier, and retry count without direct customer or vehicle identifiers

#### Scenario: Database conflict occurs
- **WHEN** the database rejects an overlapping allocation
- **THEN** the system records a conflict metric and a structured warning linked to the request identifier
