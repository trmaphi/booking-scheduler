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

## 4. Release and rollback

- [ ] 4.1 Publish SHA-tagged x86_64 images to GHCR from a least-privilege workflow.
- [ ] 4.2 Add validated deploy and rollback playbooks with migration and public-health gates.

## 5. Backup and restore

- [ ] 5.1 Add encrypted R2 backups, retention, cleanup, repository checks, and a systemd timer.
- [ ] 5.2 Add disposable restore verification and separately confirmed cutover.

## 6. Delivery and operations

- [ ] 6.1 Integrate infrastructure verification into the sequential delivery gate.
- [ ] 6.2 Document bootstrap, deployment, rollback, backup, restore drill, monitoring, and VPS replacement.
- [ ] 6.3 Verify the live production deployment, off-host snapshot, and disposable restore.
