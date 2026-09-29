# VPS Production Deployment Design

## Purpose

Deploy the scheduler reproducibly to the existing Ubuntu 24.04 VPS at `103.188.83.18`, serve it at `booking-scheduler.trmaphi.work`, and keep production operations small enough for one maintainer. The deployment must preserve the local Docker Compose workflow while adding secure host configuration, immutable application releases, HTTPS, persistent PostgreSQL storage, and recoverable off-server backups.

This change replaces the earlier assumption that production hosting remains undecided. The selected topology is one existing VPS, Cloudflare-managed DNS, and Cloudflare R2 for encrypted backup storage. Product names, fixtures, and application behavior remain vendor-neutral.

## Constraints and assumptions

- The server runs Ubuntu 24.04 LTS on x86_64 with 4 vCPU, 8 GB RAM, and an 80 GB root disk.
- The DNS `A` record for `booking-scheduler.trmaphi.work` points to `103.188.83.18` without relying on Ansible to own the Cloudflare zone.
- Initial bootstrap may connect as `root`, but routine deployment must use a non-root `deploy` account with `sudo` and SSH-key authentication.
- Passwords, private keys, Cloudflare account identifiers, R2 credentials, registry credentials, and production environment files must never be committed.
- Only TCP ports 22, 80, and 443 are publicly reachable. PostgreSQL, the Go API, and Next.js bind only to the private Compose network.
- The deployment uses the existing Next.js frontend, Go API, PostgreSQL migrations, health endpoints, and standard PostgreSQL URL.
- Booking confirmation stays synchronous. This change does not add Kafka, Redis, a worker, or another queue.
- A single VPS is an accepted failure domain for this stage. Database recovery depends on tested off-server backups.

## Selected architecture

Ansible configures the existing host and deploys a production Docker Compose project. Caddy is the public entry point and obtains TLS certificates automatically after DNS resolves to the VPS. It routes `/api/*` to the Go API and all other paths to Next.js, giving the browser one origin and removing the need to expose application ports.

```text
Internet
   |
   v
Caddy :80/:443
   |-- /api/* --> Go API :8080 --> PostgreSQL :5432
   `-- /*     --> Next.js :3000

Nightly timer --> pg_dump --> encrypted restic repository in R2
```

The production Compose project contains `proxy`, `web`, `api`, `migrate`, and `postgres` services. The one-shot migration service must complete successfully before the API starts. The proxy starts only after the web and API health checks pass. PostgreSQL uses a named volume and is never published on a host port.

Application images are multi-stage production images. CI or a trusted local workstation builds versioned images and publishes them to GitHub Container Registry. The VPS pulls an explicit release tag. Ansible records the active tag, verifies health after deployment, and supports rollback by redeploying a previous tag. Building on the VPS is an emergency fallback, not the normal release path.

## Alternatives and trade-offs

### Ansible plus Docker Compose — selected

Ansible is agentless, connects over SSH, and expresses repeatable host state. Docker Compose closely matches local development and is sufficient for one host. The cost is maintaining playbooks and accepting that Compose does not provide multi-host failover.

### Manual shell setup

Manual commands would be faster for the first deployment, but they would not provide a reviewable record or reliable rebuild path. Configuration drift and undocumented recovery steps make this unsuitable.

### OpenTofu plus Ansible

OpenTofu would add value if the VPS provider exposed an API and the server itself needed to be created, replaced, or networked declaratively. The server already exists and no provider API has been selected, so provisioning code would add abstractions without controlling a resource.

### Kubernetes

Kubernetes would provide richer orchestration and future multi-node scheduling. It adds control-plane, networking, storage, and upgrade complexity that is disproportionate to one application on one VPS.

## Host bootstrap

`bootstrap.yml` is the only playbook allowed to use the initial root login. It must:

1. Verify Ubuntu 24.04 and supported x86_64 architecture.
2. Install security updates and required system packages.
3. Create the `deploy` user with a locked password, a supplied SSH public key, and passwordless sudo limited to the deployment operations required by the playbooks.
4. Configure SSH to reject password authentication, empty passwords, and direct root login after key access has been verified.
5. Configure UFW to deny unsolicited inbound traffic and allow only SSH, HTTP, and HTTPS.
6. Install and enable Fail2ban and unattended security updates.
7. Install Docker Engine and the Compose plugin from Docker's signed repository, add `deploy` to the Docker group, and configure bounded JSON log rotation.
8. Create `/opt/booking-scheduler`, `/var/lib/booking-scheduler/backups`, and required configuration directories with restrictive ownership and permissions.

SSH hardening must use a validate-then-activate sequence so a malformed configuration cannot terminate the only working session. The bootstrap must be safe to rerun.

## Application deployment

`deploy.yml` runs as `deploy` and must:

1. Validate required encrypted variables without printing values.
2. Render the production Compose file, Caddy configuration, and root-readable environment file.
3. Authenticate to the container registry only when private images require it.
4. Pull the selected immutable image tag.
5. Start PostgreSQL, wait for database health, run the migration container, and stop immediately if migration fails.
6. Start the API, web, and proxy in dependency order.
7. Verify the local readiness endpoint and public HTTPS application/API endpoints.
8. Record the release tag and deployment timestamp only after verification succeeds.
9. Remove dangling images without deleting active or rollback images.

The playbook must use Compose project name `booking-scheduler-prod`. Destructive volume removal is never part of deployment or rollback. A separate maintenance command may prune explicitly selected old images.

## Production images

The Go image compiles `cmd/server` and `cmd/migrate` as static or minimal dynamically linked binaries in a builder stage and runs them from a non-root minimal runtime stage. The same image supports separate server and migration commands.

The Next.js image uses the project's pinned Node.js 26 runtime, performs a locked dependency install and production build, and runs the standalone server as a non-root user. The browser API base URL is same-origin; server-side calls use the internal API service address.

Every image receives OCI revision metadata and an immutable Git commit tag. `latest` is not accepted as a production deployment input.

## Secrets

Ansible Vault encrypts production variables in Git. The encrypted file may contain references and encrypted values but no plaintext secret. A committed example documents required keys:

- PostgreSQL database, username, and generated password
- application origin and domain
- release image names and tag
- optional registry username and token
- R2 S3 endpoint, bucket, access-key ID, and secret access key
- restic repository password

The rendered server environment file is owned by root, readable by the deployment group only where necessary, and never included in image layers, logs, facts output, or backup metadata. Playbook tasks that touch secrets use `no_log: true`.

The initial VPS password must not be placed in inventory, command history, chat transcripts, or Vault. It is used interactively only long enough to install an SSH key, after which password login is disabled.

## TLS and DNS

Cloudflare remains the authoritative DNS provider. The operator creates or verifies the `A` record before deployment. The first release assumes the record is DNS-only while Caddy completes normal ACME HTTP validation; proxying may be enabled afterward if desired and tested.

Caddy redirects HTTP to HTTPS, sends security headers, forwards the canonical client address and request ID, and applies conservative timeouts. It does not cache API responses. Certificate state is held in a named volume so restarts do not trigger unnecessary issuance.

## Database durability and R2 backups

PostgreSQL data lives in a dedicated named volume. A systemd service and timer create a nightly custom-format `pg_dump`, verify that the dump is non-empty, and store it in an encrypted restic repository backed by R2's S3-compatible endpoint. Restic encryption occurs before objects are uploaded.

Retention keeps seven daily, four weekly, and six monthly snapshots. After a successful upload and repository check, temporary plaintext dump files are securely removed. Failed backups retain diagnostic status without printing credentials and cause the timer unit to fail visibly.

`restore.yml` requires an explicit snapshot ID and an explicit confirmation variable. It restores into a temporary database first, runs basic schema checks, and only replaces the production database through a separately confirmed cutover. A quarterly restore drill verifies that off-server backups are usable.

R2 improves durability against VPS disk loss, but it is not a substitute for restore testing. Bucket versioning or retention rules are recommended where available, with lifecycle settings coordinated with restic retention rather than deleting repository objects independently.

## Observability and resource controls

The Go API continues emitting privacy-safe JSON logs to standard output. Docker uses bounded local log files, defaulting to five 10 MB files per container. Caddy access logs use the same bounded policy and exclude sensitive headers and query values.

Compose defines restart policies, health checks, and conservative memory limits. PostgreSQL receives the largest memory allowance; the Go API remains small; Next.js has enough headroom for runtime operation because builds happen outside the VPS. Disk monitoring must alert before the root filesystem reaches 80% utilization.

No log aggregation service is introduced in this change. A future collector can ship the existing structured events without changing application logging.

## Failure handling and rollback

- A failed migration prevents the new API from starting and leaves the current persistent database volume intact.
- A container that fails health checks causes deployment verification to fail before the release is recorded.
- Rollback redeploys the previous immutable image tag. Database migrations must remain backward compatible for at least one application release because schema rollback is not automatic.
- Caddy retains certificates and continues serving the last healthy containers while a new image is pulled.
- Backup failure does not stop bookings, but it is an operational failure that must be visible and corrected before retention gaps grow.
- Complete VPS loss is recovered by bootstrapping a replacement Ubuntu host, deploying the selected release, and restoring a verified R2 snapshot.

## Verification

Repository checks must cover:

- Ansible syntax validation and `ansible-lint` for every playbook and role.
- A secret scan proving examples and rendered-test fixtures contain no credentials.
- Production Compose configuration validation with all required variables supplied from non-secret test fixtures.
- Multi-stage image builds for x86_64.
- A clean-volume local production smoke test covering migrations, API readiness, web rendering, and same-origin API routing through Caddy.
- A second Ansible check-mode/idempotence run with no unexpected changes.
- Remote verification of HTTPS, certificate validity, security headers, application health, and closed application/database ports.
- Backup creation, repository check, snapshot listing, and a restore into a disposable database.

The existing application unit, integration, concurrency, E2E, OpenSpec, and confidentiality checks remain required. The confidentiality policy must be revised narrowly so the explicitly selected deployment services are allowed while source-identifying content, PDFs, credentials, real-looking fixtures, and unapproved provider commitments remain rejected.

## Deferred work

- Automated Cloudflare DNS record management; DNS remains an operator prerequisite.
- Multi-VPS availability and database replication.
- Kafka, Redis, background workers, and notification delivery.
- Centralized log aggregation and alert delivery.
- Automated VPS provisioning through a hosting-provider API.
- Zero-downtime database schema rollback.

