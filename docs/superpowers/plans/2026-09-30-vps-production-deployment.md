# VPS Production Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provision and harden the existing Ubuntu VPS, deploy the scheduler at `booking-scheduler.trmaphi.work`, monitor its Linux host and public readiness through Grafana Cloud, and maintain encrypted PostgreSQL backups in Cloudflare R2 through repeatable Ansible workflows.

**Architecture:** Ansible configures the existing host, installs Grafana Alloy as a metrics-only systemd service, and renders a production Docker Compose project. Caddy terminates HTTPS and routes same-origin browser and API traffic; immutable GHCR images run Next.js and Go; PostgreSQL stays private and persists on a named volume; Alloy remote-writes 60-second Linux host metrics to Grafana Cloud; a public Grafana probe checks readiness every five minutes; and a systemd timer writes verified database dumps into an encrypted restic repository backed by R2.

**Tech Stack:** Ansible Core, ansible-lint, Docker Engine, Docker Compose, Caddy 2, GitHub Container Registry, PostgreSQL 16, Grafana Alloy, Grafana Cloud Prometheus and Synthetic Monitoring, restic, Cloudflare DNS and R2, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-30-vps-production-deployment-design.md`

## Global Constraints

- Target Ubuntu 24.04 LTS on x86_64 at `103.188.83.18`; routine automation uses non-root user `deploy`.
- Public inbound ports are TCP 22, 80, and 443 only; web, API, and PostgreSQL publish no host ports.
- Serve `booking-scheduler.trmaphi.work` over automatically renewed HTTPS with same-origin `/api/*` routing.
- Use immutable Git commit image tags; reject `latest` as a deployment input.
- Never commit or print passwords, private keys, Cloudflare account identifiers, R2 credentials, registry credentials, Grafana access-policy tokens, Vault passwords, or rendered production environment files.
- Keep booking synchronous; do not add Kafka, Redis, a worker, Kubernetes, or another queue.
- Retain seven daily, four weekly, and six monthly encrypted restic snapshots in R2.
- Deployment and rollback never remove the PostgreSQL volume.
- Grafana Alloy collects host metrics only at a 60-second interval; do not enable log, trace, or profile pipelines.
- Grafana Cloud checks the public readiness endpoint from one location every five minutes and warns before the root filesystem reaches 80% utilization.
- Preserve the existing application, concurrency, E2E, OpenSpec, and confidentiality checks.

## Review Focus

- A missing, malformed, or `latest` release tag must stop deployment before any running container changes.
- An unreachable database or failed migration must prevent the new API release from starting while preserving the database volume.
- Missing or invalid Grafana Cloud inputs must stop monitoring configuration without leaking the token; a valid configuration must remain metrics-only and converge without restarting healthy Alloy unnecessarily.
- A backup with an empty or failed `pg_dump` must never be uploaded as a successful snapshot; credentials must remain absent from logs.
- A restore request without an explicit snapshot ID and cutover confirmation must leave production data unchanged.

---

### Task 1: Add the OpenSpec deployment change and revise provider policy

**Files:**
- Create: `openspec/changes/deploy-production-vps/proposal.md`
- Create: `openspec/changes/deploy-production-vps/design.md`
- Create: `openspec/changes/deploy-production-vps/specs/production-deployment/spec.md`
- Create: `openspec/changes/deploy-production-vps/tasks.md`
- Modify: `openspec/config.yaml`
- Modify: `scripts/confidentiality-scan.mjs`
- Modify: `scripts/delivery-scripts.test.ts`

**Interfaces:**
- Consumes: approved deployment design and existing confidentiality scanner categories.
- Produces: strict OpenSpec requirements for host bootstrap, release, HTTPS, monitoring, backups, restoration, and a scanner allowlist limited to the explicitly selected deployment services.

- [ ] **Step 1: Write failing scanner tests for the revised policy**

Add tests named `accepts approved production deployment services`, `rejects unapproved provider commitment`, and `still rejects provider credentials`. Use neutral fake endpoints and unmistakably fake secret markers; assert approved deployment files pass while arbitrary providers and credential-shaped values fail.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `pnpm vitest run scripts/delivery-scripts.test.ts`

Expected: FAIL because the current scanner rejects every provider commitment.

- [ ] **Step 3: Create the OpenSpec artifacts and narrow scanner policy**

Specify observable requirements and scenarios for idempotent bootstrap, locked-down networking, immutable release tags, migration ordering, HTTPS routing, metrics-only Grafana Alloy, public synthetic readiness monitoring, encrypted R2 backups, guarded restoration, and rollback. Update `openspec/config.yaml` so the selected production topology is explicit. Change only the provider-commitment policy required by the approved design; retain all source-identity, PDF, credential, fixture, history, and unapproved-provider protections.

- [ ] **Step 4: Validate tests and OpenSpec**

Run: `pnpm vitest run scripts/delivery-scripts.test.ts && openspec validate deploy-production-vps --strict && pnpm scan:confidentiality`

Expected: PASS, including the approved design document already committed.

- [ ] **Step 5: Commit**

```bash
git add openspec scripts
git commit -m "docs: specify production VPS deployment"
```

### Task 2: Build immutable production images and Compose topology

**Files:**
- Create: `Dockerfile.web`
- Create: `api/Dockerfile`
- Create: `compose.production.yaml`
- Create: `deploy/Caddyfile`
- Create: `deploy/production.env.example`
- Create: `scripts/check-production-deployment.mjs`
- Create: `scripts/production-deployment.test.ts`
- Modify: `next.config.ts`
- Modify: `package.json`
- Modify: `.dockerignore`

**Interfaces:**
- Consumes: `IMAGE_TAG`, `WEB_IMAGE`, `API_IMAGE`, database values, and `APP_DOMAIN` from the rendered environment.
- Produces: `booking-scheduler-prod` services `postgres`, `migrate`, `api`, `web`, and `proxy`; commands `pnpm production:check`, `pnpm production:start`, `pnpm production:stop`, and `pnpm production:smoke`.

- [ ] **Step 1: Write failing topology and secret-boundary tests**

Test that production Compose rejects missing or `latest` tags, publishes only proxy ports 80/443, assigns no host ports to web/API/PostgreSQL, waits for migration completion, uses named database and Caddy volumes, bounds logs, defines health checks and restart policies, and contains no secret values in committed examples.

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `pnpm vitest run scripts/production-deployment.test.ts`

Expected: FAIL because production images and Compose files do not exist.

- [ ] **Step 3: Add production images and standalone Next.js output**

Build Go server and migration binaries in one builder and run them as a non-root user in a minimal runtime. Build Next.js with Node 26.10.0 and pnpm 10.17.1, enable `output: "standalone"`, copy standalone/static/public assets, and run as a non-root user. Accept OCI revision metadata through build arguments.

- [ ] **Step 4: Add production Compose and Caddy routing**

Define private networks, named volumes, dependency conditions, resource limits suited to 4 vCPU/8 GB, five 10 MB JSON log files, same-origin browser configuration, Caddy `/api/*` routing, security headers, and persistent certificate state. Do not publish PostgreSQL, API, or web ports.

- [ ] **Step 5: Implement production validation and smoke commands**

`check-production-deployment.mjs` must reject blank variables, `latest`, non-commit-shaped tags, unsafe domain values, and Compose output that exposes private ports. The smoke path must use non-secret test values and verify web rendering plus API readiness through the proxy.

- [ ] **Step 6: Run image and topology verification**

Run: `pnpm production:check && docker build -f Dockerfile.web . && docker build -f api/Dockerfile api && pnpm vitest run scripts/production-deployment.test.ts`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add Dockerfile.web api/Dockerfile compose.production.yaml deploy next.config.ts package.json .dockerignore scripts
git commit -m "feat: add production container topology"
```

### Task 3: Add idempotent Ansible host bootstrap

**Files:**
- Create: `infra/ansible/ansible.cfg`
- Create: `infra/ansible/requirements.yml`
- Create: `infra/ansible/inventory/production.yml`
- Create: `infra/ansible/group_vars/all.yml`
- Create: `infra/ansible/playbooks/bootstrap.yml`
- Create: `infra/ansible/roles/common/tasks/main.yml`
- Create: `infra/ansible/roles/security/tasks/main.yml`
- Create: `infra/ansible/roles/security/handlers/main.yml`
- Create: `infra/ansible/roles/security/templates/99-booking-scheduler.conf.j2`
- Create: `infra/ansible/roles/docker/tasks/main.yml`
- Create: `infra/ansible/roles/docker/templates/daemon.json.j2`
- Create: `infra/ansible/roles/directories/tasks/main.yml`
- Create: `infra/ansible/tests/bootstrap_contract_test.py`
- Create: `infra/ansible/Makefile`

**Interfaces:**
- Consumes: `bootstrap_public_key` supplied at runtime and the static production inventory address.
- Produces: locked-password `deploy` user, validated hardened SSH, UFW rules 22/80/443, Fail2ban, unattended upgrades, Docker/Compose, bounded daemon logs, and protected application directories.

- [ ] **Step 1: Write failing bootstrap contract tests**

Assert the play refuses non-Ubuntu-24.04 or non-x86_64 hosts, never embeds a password, validates SSH configuration before restart, disables root/password login only after key installation, defaults UFW deny-incoming, permits exactly 22/80/443, pins Docker's signed repository, and creates required directories with restrictive modes.

- [ ] **Step 2: Run tests and verify they fail**

Run: `python3 -m unittest infra/ansible/tests/bootstrap_contract_test.py`

Expected: FAIL because the roles do not exist.

- [ ] **Step 3: Implement bootstrap roles and playbook**

Use fully qualified Ansible module names, handlers for service restarts, `validate` for sshd configuration, and tags `common`, `security`, `docker`, and `directories`. Keep the inventory free of credentials and set `ansible_user: deploy` only for routine playbooks; document the root bootstrap override in the Makefile target.

- [ ] **Step 4: Run static and syntax verification**

Run: `ansible-galaxy collection install -r infra/ansible/requirements.yml && ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/bootstrap.yml && ansible-lint infra/ansible && python3 -m unittest infra/ansible/tests/bootstrap_contract_test.py`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add infra/ansible
git commit -m "feat: automate secure VPS bootstrap"
```

### Task 4: Add lightweight Grafana Cloud host monitoring

**Files:**
- Create: `infra/ansible/playbooks/monitoring.yml`
- Create: `infra/ansible/group_vars/monitoring.example.yml`
- Create: `infra/ansible/group_vars/monitoring.vault.yml.example`
- Create: `infra/ansible/roles/monitoring/defaults/main.yml`
- Create: `infra/ansible/roles/monitoring/tasks/main.yml`
- Create: `infra/ansible/roles/monitoring/handlers/main.yml`
- Create: `infra/ansible/roles/monitoring/templates/config.alloy.j2`
- Create: `infra/ansible/roles/monitoring/templates/alloy.env.j2`
- Create: `infra/ansible/tests/monitoring_contract_test.py`
- Modify: `infra/ansible/Makefile`
- Modify: `scripts/confidentiality-scan.mjs`
- Modify: `scripts/delivery-scripts.test.ts`

**Interfaces:**
- Consumes: `grafana_prometheus_url`, `grafana_prometheus_username`, Vault-backed `grafana_access_policy_token`, and stable `monitoring_instance`/`monitoring_environment` labels.
- Produces: validated metrics-only Grafana Alloy service, 60-second Linux host metrics in Grafana Cloud, and operator commands for service/configuration/metric-freshness verification.

- [ ] **Step 1: Write failing monitoring contract tests**

Add tests named `test_alloy_uses_signed_repository`, `test_alloy_collects_only_approved_host_metrics_every_sixty_seconds`, `test_alloy_credentials_are_root_only_and_suppressed`, `test_alloy_configuration_is_validated_before_restart`, and `test_alloy_service_is_idempotently_enabled`. Assert the configuration contains the built-in Unix exporter, stable instance/environment labels, `scrape_interval = "60s"`, and Prometheus remote write, while excluding Loki, Tempo, OpenTelemetry trace, and Pyroscope pipelines. Extend the confidentiality scanner tests so only Grafana commitments inside the approved production-monitoring artifacts pass while arbitrary monitoring providers and credential-shaped values still fail.

- [ ] **Step 2: Run tests and verify they fail**

Run: `python3 -m unittest infra/ansible/tests/monitoring_contract_test.py && pnpm vitest run scripts/delivery-scripts.test.ts`

Expected: FAIL because the monitoring role does not exist.

- [ ] **Step 3: Implement the Grafana Alloy role and playbook**

Install Alloy from Grafana's signed APT repository. Render `/etc/alloy/config.alloy` without secrets and `/etc/alloy/alloy.env` as root-owned mode `0600` under `no_log: true`. Configure the built-in Unix exporter for CPU, memory, load, filesystem, disk I/O, and network metrics only; scrape every 60 seconds; attach stable labels; and remote-write with basic authentication read from the environment. Validate configuration before notifying a restart, and start/enable the systemd service through a handler only when inputs change. Narrowly extend the approved-provider scanner policy for these monitoring artifacts without weakening credential or unapproved-provider detection.

- [ ] **Step 4: Add guarded variables and operator targets**

Keep endpoint, username, and stable labels in the non-secret example; keep only the access-policy token in the Vault example. Add Makefile targets for monitoring syntax, deployment, `systemctl is-active alloy`, configuration validation, and explicit Grafana metric-freshness verification without printing credentials.

- [ ] **Step 5: Run monitoring verification**

Run: `ANSIBLE_CONFIG=infra/ansible/ansible.cfg ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/monitoring.yml && ANSIBLE_CONFIG=infra/ansible/ansible.cfg ansible-lint infra/ansible && python3 -m unittest infra/ansible/tests/monitoring_contract_test.py && pnpm vitest run scripts/delivery-scripts.test.ts && pnpm scan:confidentiality`

Expected: PASS with no secret value or expensive telemetry pipeline in output.

- [ ] **Step 6: Commit**

```bash
git add infra/ansible scripts/confidentiality-scan.mjs scripts/delivery-scripts.test.ts
git commit -m "feat: add lightweight VPS monitoring"
```

### Task 5: Add deployment, health verification, and rollback automation

**Files:**
- Create: `infra/ansible/playbooks/deploy.yml`
- Create: `infra/ansible/playbooks/rollback.yml`
- Create: `infra/ansible/group_vars/production.example.yml`
- Create: `infra/ansible/group_vars/production.vault.yml.example`
- Create: `infra/ansible/roles/application/defaults/main.yml`
- Create: `infra/ansible/roles/application/tasks/main.yml`
- Create: `infra/ansible/roles/application/handlers/main.yml`
- Create: `infra/ansible/roles/application/templates/compose.production.yaml.j2`
- Create: `infra/ansible/roles/application/templates/Caddyfile.j2`
- Create: `infra/ansible/roles/application/templates/production.env.j2`
- Create: `infra/ansible/tests/application_contract_test.py`
- Create: `.github/workflows/publish-images.yml`
- Modify: `infra/ansible/Makefile`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: immutable `release_tag`, image names, domain, PostgreSQL values, optional GHCR credentials, and deployed/monitored host state from Tasks 3–4.
- Produces: verified active release under `/opt/booking-scheduler`, `/opt/booking-scheduler/releases/current`, public health assertions, and rollback to an explicitly supplied prior tag without volume deletion.

- [ ] **Step 1: Write failing deployment contract tests**

Assert validation precedes mutation; secret-rendering tasks use `no_log`; environment mode is `0640`; migration failure stops deployment; release state is written only after HTTPS checks pass; rollback requires an explicit immutable tag; and no task invokes Compose with `--volumes` or removes the database volume.

- [ ] **Step 2: Run tests and verify they fail**

Run: `python3 -m unittest infra/ansible/tests/application_contract_test.py`

Expected: FAIL because deployment automation does not exist.

- [ ] **Step 3: Implement release publication workflow**

On pushes of version tags and manual dispatch, build x86_64 web/API images, label them with the Git SHA, publish SHA tags to GHCR, and run production topology checks before publishing. Grant only `contents: read` and `packages: write`; do not embed deployment credentials.

- [ ] **Step 4: Implement deploy and rollback roles**

Validate inputs, render files atomically, log into GHCR only when configured, pull images, bring up PostgreSQL, run migrations, start services, verify local and public health, record release state, and prune dangling layers. Rollback calls the same role with an explicit prior tag and never changes volumes.

- [ ] **Step 5: Run automation verification**

Run: `ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/deploy.yml && ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/rollback.yml && ansible-lint infra/ansible && python3 -m unittest infra/ansible/tests/application_contract_test.py`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add .github .gitignore infra/ansible
git commit -m "feat: automate immutable VPS releases"
```

### Task 6: Add encrypted R2 backup and guarded restore automation

**Files:**
- Create: `infra/ansible/playbooks/backup.yml`
- Create: `infra/ansible/playbooks/restore.yml`
- Create: `infra/ansible/roles/backup/defaults/main.yml`
- Create: `infra/ansible/roles/backup/tasks/main.yml`
- Create: `infra/ansible/roles/backup/templates/booking-scheduler-backup.service.j2`
- Create: `infra/ansible/roles/backup/templates/booking-scheduler-backup.timer.j2`
- Create: `infra/ansible/roles/backup/templates/backup-postgres.sh.j2`
- Create: `infra/ansible/roles/backup/templates/restic.env.j2`
- Create: `infra/ansible/tests/backup_contract_test.py`
- Modify: `infra/ansible/playbooks/deploy.yml`
- Modify: `infra/ansible/Makefile`

**Interfaces:**
- Consumes: R2 S3 endpoint, bucket, access key, secret key, restic password, database container identity, explicit restore snapshot ID, and `confirm_restore_cutover`.
- Produces: nightly systemd backup timer, encrypted restic snapshots with `7 daily / 4 weekly / 6 monthly` retention, repository checks, disposable restore verification, and separately confirmed production cutover.

- [ ] **Step 1: Write failing backup and restore contract tests**

Assert the backup exits before upload when `pg_dump` fails or is empty, uses a `trap` to remove temporary dumps, keeps credentials in a root-only environment file, initializes restic safely, applies exact retention, and fails the service on repository-check errors. Assert restore rejects blank/latest snapshot selectors and missing confirmation, restores into a temporary database first, and checks required booking tables before any cutover.

- [ ] **Step 2: Run tests and verify they fail**

Run: `python3 -m unittest infra/ansible/tests/backup_contract_test.py`

Expected: FAIL because backup automation does not exist.

- [ ] **Step 3: Implement backup role and systemd units**

Install pinned distribution packages for restic and PostgreSQL client tools, render credentials with mode `0600`, schedule the timer with randomized delay, create a custom-format dump, validate it, send it to restic, run retention and repository checks, and remove temporary material on every exit path.

- [ ] **Step 4: Implement guarded restore playbook**

Require an exact snapshot ID and `confirm_restore_cutover: true`. Restore into a uniquely named temporary database, run `pg_restore --list` and schema/table probes, report the verified candidate, and make production cutover a separately tagged `restore_cutover` task.

- [ ] **Step 5: Run backup automation verification**

Run: `ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/backup.yml && ansible-playbook --syntax-check -i infra/ansible/inventory/production.yml infra/ansible/playbooks/restore.yml && ansible-lint infra/ansible && python3 -m unittest infra/ansible/tests/backup_contract_test.py`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add infra/ansible
git commit -m "feat: add encrypted database backups"
```

### Task 7: Integrate delivery verification and operator documentation

**Files:**
- Create: `docs/operations/production-deployment.md`
- Create: `docs/operations/disaster-recovery.md`
- Create: `scripts/verify-infrastructure.mjs`
- Create: `scripts/verify-infrastructure.test.ts`
- Modify: `README.md`
- Modify: `.env.example`
- Modify: `scripts/verify-delivery.mjs`
- Modify: `scripts/delivery-scripts.test.ts`
- Modify: `openspec/changes/deploy-production-vps/tasks.md`

**Interfaces:**
- Consumes: infrastructure checks and commands created in Tasks 1–6.
- Produces: `pnpm verify:infrastructure`, updated sequential delivery gate, bootstrap/deploy/rollback/backup/restore runbooks, DNS and R2 setup checklist, and checked OpenSpec completion state.

- [ ] **Step 1: Write failing delivery-step tests**

Assert the infrastructure verifier runs production Compose validation, production image builds, Ansible syntax including the monitoring playbook, ansible-lint, bootstrap/application/monitoring/backup Python contract tests, secret/confidentiality scans, and OpenSpec strict validation in a fixed order and stops on first failure.

- [ ] **Step 2: Run tests and verify they fail**

Run: `pnpm vitest run scripts/verify-infrastructure.test.ts scripts/delivery-scripts.test.ts`

Expected: FAIL because the infrastructure verifier and delivery steps are absent.

- [ ] **Step 3: Implement infrastructure verification and delivery integration**

Add `verify:infrastructure` to `package.json` and invoke it before the final confidentiality scan in `verify:delivery`. Keep remote mutation outside the standard local delivery gate; remote checks require an explicit operator command.

- [ ] **Step 4: Write operator and recovery documentation**

Document SSH key bootstrap without placing the password in commands, Cloudflare DNS prerequisites, R2 bucket/token scope, Vault creation, GHCR visibility/authentication, Grafana Cloud Prometheus endpoint/instance ID/access-policy scope, metrics-only Alloy deployment and status checks, Linux Server integration dashboards/alerts, root-filesystem alert verification before 80% utilization, creation of one public HTTPS readiness check every five minutes, first deployment, repeat deployment, rollback, backup status, snapshot listing, quarterly restore drill, firewall verification, and complete VPS replacement. Mark destructive restore cutover steps clearly and keep Grafana tokens out of commands and screenshots.

- [ ] **Step 5: Run the complete local release gate**

Run: `pnpm verify:infrastructure && pnpm verify:delivery`

Expected: PASS from a clean tree and fresh local volumes.

- [ ] **Step 6: Perform explicit remote bootstrap and deployment checks**

Run the documented bootstrap with the operator-supplied SSH public key and interactive initial password, reconnect as `deploy`, confirm root/password SSH are disabled, configure Vault-backed Grafana credentials, deploy Alloy and an immutable application release, then verify HTTPS, certificate validity, health endpoints, headers, closed external ports 3000/5432/8080, current CPU/memory/load/filesystem/disk/network metrics in the Linux Server integration, active root-filesystem alerting before 80%, and one successful public HTTPS readiness probe configured at five-minute intervals. Initialize R2 backup, create one snapshot, and restore it into a disposable database without cutting over production.

Expected: every remote and Grafana Cloud assertion passes; Alloy sends no logs, traces, or profiles; no secret appears in console output or Git status.

- [ ] **Step 7: Complete OpenSpec tasks and commit**

```bash
git add README.md .env.example package.json scripts docs/operations openspec/changes/deploy-production-vps/tasks.md
git commit -m "docs: add production operations runbook"
```

- [ ] **Step 8: Push the verified branch**

Run: `git push origin main`

Expected: remote `main` contains the verified infrastructure commits.
