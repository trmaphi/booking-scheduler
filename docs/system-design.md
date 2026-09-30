# Fleet booking system design

## Status and scope

**Status: Proposed replacement.** This design is not implemented by the current repository.

The current implementation remains the service-appointment scheduler defined by the completed OpenSpec change. This target architecture replaces it with fleet booking: users request a quantity of vehicles by type, may use different pickup and return locations, and may receive asynchronous backfill for missing quantity.

The core is intentionally resource-neutral:

- `resource_type`: a category such as compact car, van, or truck.
- `resource`: one individually bookable vehicle.
- `booking_policy`: eligibility, location, turnaround, and allocation rules.
- `booking`: customer demand for a type, quantity, interval, pickup, and return.
- `allocation`: assignment of one individual resource to a booking.

## Specification traceability

The implemented service scheduler is defined by the completed OpenSpec change:

- [Proposal](../openspec/changes/bootstrap-service-scheduler/proposal.md)
- [Capability specification](../openspec/changes/bootstrap-service-scheduler/specs/service-appointment-booking/spec.md)
- [Implementation design](../openspec/changes/bootstrap-service-scheduler/design.md)
- [Completed tasks](../openspec/changes/bootstrap-service-scheduler/tasks.md)

The fleet replacement described below is **Proposed** and is not represented by that completed change. It requires a separate OpenSpec change before implementation. **Alternative** sections describe optional future approaches rather than committed scope.

## Proposed: hard booking invariant

> One individual resource cannot have overlapping confirmed allocations.

Multiple resources of the same type may be booked concurrently. Type capacity for an interval is the count of eligible individual resources that can satisfy it. Qualification, routing, identity, partial fulfillment, and fairness are policies or requirements; they do not replace the database invariant.

## Proposed: architecture

```mermaid
flowchart LR
    User[User] --> Web[Next.js web]
    Web --> IdP[OIDC provider]
    Web --> API[Go API container]
    API --> DB[(PostgreSQL)]
    Worker[Go allocation worker container] --> DB
    API --> Telemetry[Logs and metrics]
    Worker --> Telemetry
```

- The API performs synchronous search and confirmation.
- PostgreSQL stores bookings, allocations, movements, and durable backfill work.
- The worker retries missing quantity and may scale to multiple containers.
- The API and worker are stateless; PostgreSQL is the initial consistency and queue authority.

## Proposed: data model

```mermaid
erDiagram
    RESOURCE_TYPE ||--o{ RESOURCE : classifies
    RESOURCE_TYPE ||--o{ BOOKING : requested_as
    BOOKING ||--o{ ALLOCATION : receives
    RESOURCE ||--o{ ALLOCATION : assigned_to
    RESOURCE ||--o{ RESOURCE_MOVEMENT : follows
    LOCATION ||--o{ BOOKING : pickup_or_return

    RESOURCE_TYPE {
        uuid id
        string name
        jsonb attributes
    }

    RESOURCE {
        uuid id
        uuid resource_type_id
        uuid scheduling_owner_id
        uuid home_location_id
        string status
        jsonb attributes
    }

    BOOKING {
        uuid id
        uuid requested_type_id
        uuid pickup_location_id
        uuid return_location_id
        timestamp start_at
        timestamp end_at
        int requested_quantity
        int minimum_quantity
        string status
        timestamp fulfillment_deadline
    }

    ALLOCATION {
        uuid booking_id
        uuid resource_id
        timestamp start_at
        timestamp end_at
        string status
    }
```

PostgreSQL uses a partial GiST exclusion constraint on `resource_id` and the half-open interval `[start_at, end_at)` for allocation statuses that consume capacity.

## Proposed: identity

The system accepts any standards-compliant OpenID Connect provider.

```mermaid
sequenceDiagram
    actor User
    participant Web as Next.js
    participant IdP as OIDC provider
    participant API as Go API
    participant DB as PostgreSQL

    User->>Web: Sign in
    Web->>IdP: Authorization Code with PKCE
    IdP-->>Web: Access token
    Web->>API: Request with access token
    API->>API: Validate signature, issuer, audience, and expiry
    API->>DB: Resolve issuer and subject to customer
```

`(issuer, subject)` is the stable external identity. The API derives the customer identifier and applies ownership or staff authorization locally.

## Proposed: confirmation and partial fulfillment

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant DB as PostgreSQL
    participant Worker

    Client->>API: Type, quantity, minimum, interval, pickup, return
    API->>DB: Begin allocation transaction
    DB->>DB: Find eligible, reachable, non-overlapping resources
    DB->>DB: Lock candidates and insert allocations
    alt requested quantity allocated
        DB-->>API: CONFIRMED
    else minimum quantity allocated
        DB->>DB: Store remaining backfill demand
        DB-->>API: PARTIALLY_CONFIRMED
        Worker->>DB: Claim work with SKIP LOCKED
        Worker->>DB: Add available allocations transactionally
    else below minimum
        DB-->>API: Roll back and reject
    end
```

- `requested_quantity` is the preferred fleet size.
- `minimum_quantity` is the smallest acceptable immediate allocation.
- `remaining_quantity` is derived from requested minus allocated.
- Backfill is best effort until `fulfillment_deadline`.
- Worker claims and retries are idempotent.
- Every added allocation is revalidated by PostgreSQL.

## Proposed: location continuity

A resource keeps one immutable scheduling owner even when its physical location changes. This keeps its complete allocation timeline under one transactional authority.

Version-one eligibility is deliberately simple:

1. The resource has no overlapping allocation.
2. Its type and attributes satisfy the booking policy.
3. Its preceding scheduled movement ends at the requested pickup location.
4. The turnaround interval is sufficient.
5. Its next allocation remains reachable after the requested return.

An explicit transfer movement may reposition a resource. Version one does not optimize transfers or routes.

## Proposed: Docker services

```mermaid
flowchart LR
    Web[web container] --> API[api container]
    API --> DB[(database container)]
    Worker[allocation-worker container] --> DB
    Migrate[migration container] --> DB
```

The API and worker may use the same Go image with different commands. Multiple workers claim distinct pending rows with `FOR UPDATE SKIP LOCKED`. Graceful shutdown commits or rolls back the active transaction before exit.

## Proposed: allocation strategy

### Version one: deterministic allocator

1. Filter active resources by requested type and policy.
2. Remove overlapping or unreachable resources.
3. Sort by stable policy, initially utilization then identifier.
4. Lock candidates and allocate up to the requested quantity.
5. Commit only when at least the minimum quantity is allocated.

Capacity counters alone are insufficient because the system must identify and route individual vehicles.

### Alternative: OR-Tools adapter

```mermaid
flowchart LR
    Core[Booking core] --> Port[Optimization interface]
    Simple[Deterministic allocator] --> Port
    ORTools[Future OR-Tools service] --> Port
    Port --> Plan[Proposed allocation and movement plan]
    Plan --> DB[(PostgreSQL validation)]
```

OR-Tools may later propose vehicle assignment, transfers, multi-location routing, and rebalancing that minimizes distance, cost, or unmet demand. PostgreSQL still validates every proposal and remains the no-overlap authority. Optimization improves utilization; it does not create physical fleet capacity.

## Proposed scaling and alternatives

| Stage | Architecture                        | Guarantee                          | Trade-off                                     |
| ----- | ----------------------------------- | ---------------------------------- | --------------------------------------------- |
| 1     | One PostgreSQL primary              | Strong local no-overlap constraint | One write authority                           |
| 2     | Horizontal APIs and workers         | Same database guarantee            | Writes still reach one primary                |
| 3     | Cache and read replicas             | Confirmation remains authoritative | Cached availability may be stale              |
| 4     | Shard by immutable scheduling owner | Strong within each owner shard     | One resource must never have multiple writers |
| 5     | OR-Tools optimization service       | Proposals only; database validates | More runtime and model complexity             |

Scale traffic independently from fleet capacity. More API or worker containers can process more demand, but only additional eligible vehicles or better routing can increase fulfillable bookings.

## Proposed: failure handling and verification

- Use idempotency keys for confirmation and worker retries.
- Return the allocated and remaining quantities explicitly.
- Expire unfinished backfill at its fulfillment deadline.
- Test boundary-touching intervals, overlapping allocations, partial allocation, backfill races, worker restarts, and location continuity.
- Monitor allocation conflicts, partial-fill rate, backfill age, worker retries, database pool saturation, and fleet utilization.
