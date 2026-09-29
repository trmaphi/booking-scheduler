# Proposal

## Why

The scheduler has a verified local delivery path but no reproducible production home. This change defines a small, secure operating model for the existing Ubuntu VPS so releases, recovery, and routine maintenance do not depend on undocumented shell history.

## What Changes

- Bootstrap and harden the production host idempotently with Ansible.
- Build immutable web and API images and publish commit-addressed releases to GHCR.
- Serve the application through Caddy at `booking-scheduler.trmaphi.work` with same-origin API routing and automatic HTTPS.
- Keep PostgreSQL private on the Compose network with a persistent named volume.
- Deploy only after validation and migrations succeed, verify public health, and retain an explicit rollback target.
- Back up PostgreSQL nightly into an encrypted restic repository in R2 and provide a guarded restore drill and cutover path.
- Integrate infrastructure contracts into the local delivery gate and document operator workflows.

## Non-goals

- Kubernetes, a managed container platform, Kafka, Redis, background workers, or a second production database.
- Zero-downtime database migrations or multi-region failover.
- Automatic production data cutover during restore verification.
- Storing production credentials, rendered environments, SSH keys, or Vault passwords in Git or CI logs.

## Capabilities

### New Capabilities

- `production-deployment`: Secure host bootstrap, immutable release deployment and rollback, HTTPS routing, encrypted backups, and guarded restoration.

### Modified Capabilities

None. Booking semantics and concurrency guarantees are unchanged.

## Impact

- Adds production container definitions, Ansible automation, a release workflow, backup/restore tooling, and runbooks.
- Commits to the approved VPS, GHCR, Cloudflare DNS/R2, Caddy, PostgreSQL, and restic topology.
- Requires operator-managed SSH, Vault, registry, DNS, and R2 credentials outside the repository.
