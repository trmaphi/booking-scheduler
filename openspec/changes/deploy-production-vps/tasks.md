# Tasks

## 1. Deployment specification and policy

- [x] 1.1 Add the production deployment proposal, design, observable requirements, and task ledger.
- [x] 1.2 Allow only the explicitly approved production provider commitment while retaining credential and unapproved-provider detection.

## 2. Immutable production topology

- [ ] 2.1 Build non-root web and API images with commit revision metadata.
- [ ] 2.2 Add private Compose networking, Caddy HTTPS routing, migration ordering, persistent volumes, health checks, and bounded logs.
- [ ] 2.3 Add executable topology and secret-boundary validation.

## 3. Host bootstrap

- [ ] 3.1 Add idempotent Ansible roles for common packages, SSH/firewall security, Docker, and protected directories.
- [ ] 3.2 Verify platform guards, key-first hardening, Docker repository pinning, and rerun behavior.

## 4. Host monitoring

- [ ] 4.1 Add a metrics-only Grafana Alloy role with signed package installation, 60-second Unix host metrics, Vault-backed credentials, configuration validation, and idempotent systemd management.
- [ ] 4.2 Verify Grafana credential secrecy, root-only file modes, stable labels, allowed metric collectors, and absence of log, trace, and profile pipelines.

## 5. Release and rollback

- [ ] 5.1 Publish SHA-tagged x86_64 images to GHCR from a least-privilege workflow.
- [ ] 5.2 Add validated deploy and rollback playbooks with migration and public-health gates.

## 6. Backup and restore

- [ ] 6.1 Add encrypted R2 backups, retention, cleanup, repository checks, and a systemd timer.
- [ ] 6.2 Add disposable restore verification and separately confirmed cutover.

## 7. Delivery and operations

- [ ] 7.1 Integrate infrastructure verification into the sequential delivery gate.
- [ ] 7.2 Document bootstrap, deployment, rollback, backup, restore drill, Alloy status/metric freshness, disk alerting, the five-minute HTTPS synthetic check, and VPS replacement.
- [ ] 7.3 Verify the live production deployment, fresh Linux host metrics, active disk alerting, successful public synthetic check, off-host snapshot, and disposable restore.
