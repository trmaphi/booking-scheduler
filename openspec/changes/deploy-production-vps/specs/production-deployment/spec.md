# Spec Delta

## Purpose

Defines observable production deployment, recovery, and operational-safety behavior for the scheduler on the approved VPS topology.

## Documented Assumptions

- Production runs on Ubuntu 24.04 LTS x86_64 at the approved VPS address with 4 vCPU and 8 GB RAM.
- DNS for `booking-scheduler.trmaphi.work` is managed in Cloudflare and points to the VPS.
- Images are published to GHCR and identified by full Git commit SHA.
- R2 provides the S3-compatible target for an encrypted restic repository.

## ADDED Requirements

### Requirement: Host bootstrap is safe and idempotent
The system MUST converge a supported host to the required user, packages, firewall, SSH, Docker, and directory state without replacing valid keys or restarting healthy services unnecessarily.

#### Scenario: Bootstrap is repeated
- **WHEN** the bootstrap playbook runs twice with the same public key
- **THEN** the second run preserves access and reports no unnecessary configuration changes

#### Scenario: Host platform is unsupported
- **WHEN** bootstrap targets a host other than Ubuntu 24.04 x86_64
- **THEN** it fails before hardening or package mutation

### Requirement: Production networking exposes only intended ingress
The host SHALL permit inbound TCP 22, 80, and 443 only, and web, API, and PostgreSQL containers MUST NOT publish host ports.

#### Scenario: Private services are probed externally
- **WHEN** an external client probes ports 3000, 5432, and 8080
- **THEN** none of those ports accepts a connection

### Requirement: Releases are immutable and validated before mutation
Deployment MUST require a nonblank full commit-shaped image tag, reject `latest`, and validate rendered configuration before changing running services.

#### Scenario: Release tag is invalid
- **WHEN** deployment receives a missing, malformed, or `latest` tag
- **THEN** it stops before pulling images or changing containers

### Requirement: Migrations gate application startup
The deployment SHALL start PostgreSQL, wait for readiness, and require the one-shot migration service to succeed before starting the new API and web release.

#### Scenario: Migration fails
- **WHEN** the migration process exits unsuccessfully
- **THEN** the new API does not start and the PostgreSQL volume remains intact

### Requirement: HTTPS provides same-origin application routing
Caddy SHALL obtain and renew a trusted certificate for the production domain, route `/api/*` to the API, route other requests to the web application, and set the documented security headers.

#### Scenario: Public health is verified
- **WHEN** deployment completes
- **THEN** HTTPS web rendering and API readiness succeed through the production domain before release state is recorded

### Requirement: Rollback is explicit and volume-preserving
Rollback MUST require an explicit immutable prior tag, use the same verified release path, and MUST NOT remove the PostgreSQL volume.

#### Scenario: Rollback tag is absent
- **WHEN** an operator requests rollback without a prior commit tag
- **THEN** no running service or volume changes

### Requirement: Backups are encrypted and verified
The system SHALL create a nonempty successful PostgreSQL dump before restic upload, encrypt it in R2, retain seven daily, four weekly, and six monthly snapshots, and fail when repository checking fails.

#### Scenario: Database dump fails or is empty
- **WHEN** `pg_dump` fails or produces an empty artifact
- **THEN** no successful snapshot is uploaded and temporary material is removed

### Requirement: Restore verification precedes cutover
Restore MUST require an exact snapshot identifier, restore into a disposable database, validate the archive and required booking tables, and require separate explicit confirmation before production cutover.

#### Scenario: Restore is requested without cutover confirmation
- **WHEN** a snapshot is verified but `confirm_restore_cutover` is not true
- **THEN** production data remains unchanged and the verified candidate is reported

### Requirement: Provider credentials remain confidential
Secrets, private keys, account identifiers, registry credentials, rendered environments, and Vault passwords MUST remain outside tracked files and MUST NOT appear in automation logs.

#### Scenario: Automation renders production secrets
- **WHEN** Ansible writes the environment or backup credential file
- **THEN** the task suppresses secret output and applies the required restrictive file mode
