# Design

## Context

The accepted production design is `docs/superpowers/specs/2026-09-30-vps-production-deployment-design.md`. The workload fits one Ubuntu 24.04 x86_64 VPS with 4 vCPU and 8 GB RAM. The governing risks are secret leakage, an unsafe release changing running containers before validation, a migration failure starting incompatible code, and backups that cannot be restored.

## Goals / Non-Goals

Goals are reproducible bootstrap, least-exposed networking, immutable releases, same-origin HTTPS, durable PostgreSQL state, encrypted off-host backups, guarded restoration, lightweight host monitoring, and operator-readable evidence. Non-goals are orchestration beyond Compose, asynchronous booking, automatic destructive recovery, and centralized application-log, trace, or profile collection.

## Architecture

The public path is browser to Caddy on ports 80/443, then to the web container or `/api/*` on the API container. Web, API, and PostgreSQL expose no host ports. A one-shot migration container must complete before the API starts. Ansible renders a release from Vault-backed inputs, validates it before mutation, pulls immutable GHCR tags, starts PostgreSQL and migrations, then starts and verifies application services. A systemd timer creates a checked `pg_dump`, snapshots it with restic to R2, applies retention, and checks the repository. Ansible also installs Grafana Alloy as a host systemd service; its built-in Unix exporter sends only 60-second host metrics to Grafana Cloud Prometheus using a Vault-backed token. Grafana Cloud independently probes the public readiness endpoint every five minutes.

## Decisions and Trade-offs

- **Ansible over host shell history:** idempotent convergence and reviewable changes cost additional YAML and contract tests.
- **Docker Compose over Kubernetes:** adequate for one host and one maintainer; host loss requires documented replacement rather than scheduler failover.
- **Caddy over application TLS:** automatic certificate renewal and one ingress boundary; certificate state becomes a persistent volume.
- **Commit SHA image tags over `latest`:** deterministic rollback and auditability; every deployment must receive an exact release tag.
- **PostgreSQL named volume on the VPS:** simple and fast; durability depends on verified off-host backups and restore drills.
- **restic in R2:** encryption, retention, and repository checking with modest operational surface; credentials and repository password require Vault-backed lifecycle management.
- **Restore verification before cutover:** catches corrupt or incomplete snapshots without touching production; recovery takes longer and cutover remains an explicit operator act.
- **Grafana Alloy on the host over another monitoring stack:** one lightweight process supplies the Linux integration metrics without running Prometheus or Grafana locally; this introduces an outbound dependency on Grafana Cloud.
- **Metrics only over broad telemetry collection:** CPU, memory, load, filesystem, disk I/O, and network data cover the immediate VPS risk at low volume; application logs, traces, and profiles remain local or disabled until an incident or measured need justifies their cost.
- **Cloud-side synthetic check over an on-host uptime probe:** one public HTTPS check every five minutes verifies the user-visible path independently of the VPS; Grafana Cloud configuration remains a separately verified operator prerequisite.

## Reliability and Observability

Health checks gate dependency order and public verification gates release-state updates. Failed migrations leave the prior application release and database volume intact. Docker JSON logs are bounded. Grafana Alloy remote-writes host metrics every 60 seconds with stable instance/environment labels; the Linux Server integration supplies dashboards and host alert rules, including root-filesystem warning before 80% utilization. A Grafana Cloud public probe verifies the readiness endpoint every five minutes. Operators can inspect Compose health, Caddy certificate state, Alloy service state, Grafana metric freshness, synthetic results, systemd backup results, restic snapshots/checks, firewall rules, disk usage, and the recorded release tag.

## Security

SSH key installation precedes disabling root and password login. UFW permits only 22, 80, and 443; Fail2ban and unattended upgrades are enabled. Routine automation uses the locked-password `deploy` user. Secrets render with restrictive permissions and `no_log`; CI only publishes images and contains no VPS deployment credentials. The Grafana Cloud access-policy token remains in Ansible Vault and a root-owned `0600` environment file; the non-secret Alloy configuration reads it from the environment.

## Verification Strategy

Contract tests pressure-test topology, validation order, idempotent bootstrap, migration gating, volume preservation, backup failure handling, restore guards, Alloy repository signing/configuration/credential modes, metrics-only collection, and secret boundaries. Static checks include Ansible syntax/lint, image builds, Compose rendering, OpenSpec validation, and confidentiality scanning. Live acceptance checks HTTPS, headers, public health, closed private ports, fresh Linux host metrics, the disk alert rule, the five-minute synthetic check, a real backup snapshot, and a disposable restore without cutover.

## Migration and Rollback

Bootstrap converges the existing host, then the first immutable release initializes the persistent database volume and runs migrations. Later releases follow the same path. Rollback requires an explicit prior commit-shaped tag and reuses the same application role; it never removes the PostgreSQL volume. Schema changes must remain forward compatible with the explicitly supported rollback window.
