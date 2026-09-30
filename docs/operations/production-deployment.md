# Production deployment operations

This runbook operates the single Ubuntu 24.04 x86_64 host at `103.188.83.18` for `booking-scheduler.trmaphi.work`. Run repository commands from `infra/ansible` unless a command says otherwise. Never paste passwords, SSH private keys, registry tokens, R2 credentials, Grafana tokens, Vault passwords, or rendered environment files into a terminal command, issue, screenshot, or Git-tracked file.

## One-time prerequisites

### DNS and network

1. In Cloudflare DNS, create an `A` record for `booking-scheduler.trmaphi.work` pointing to `103.188.83.18`.
2. Leave the record DNS-only for the first Caddy ACME issuance. Enable Cloudflare proxying only after direct HTTPS succeeds and re-test the readiness endpoint.
3. Confirm the VPS provider firewall permits inbound TCP 22, 80, and 443 only. Ansible applies the matching UFW policy on the host.

### SSH key and bootstrap

Generate or select a dedicated operator Ed25519 key and keep the private half outside the repository. Bootstrap prompts interactively for the initial root SSH password; the password is never placed in the command:

```sh
make collections
make bootstrap-root PUBLIC_KEY_FILE=/absolute/path/to/operator-key.pub
```

Reconnect as `deploy` before closing the original root session. Confirm key login works, root login fails, and password-only authentication fails. Re-run `make bootstrap PUBLIC_KEY_FILE=...`; it must converge without replacing the key or reopening ports.

From an external machine, verify the public surface:

```sh
nmap -Pn -p 22,80,443,3000,5432,8080 103.188.83.18
```

Only 22, 80, and 443 may be open.

### R2 backup repository

1. Create a dedicated R2 bucket for scheduler database backups.
2. Create a bucket-scoped R2 API token that can list, read, write, and delete objects only in that bucket. Retention pruning requires deletion; account administration is unnecessary.
3. Put the S3 endpoint and bucket name in untracked `group_vars/production.yml`.
4. Put the access key, secret key, and a unique restic repository password in untracked `group_vars/production.vault.yml`.
5. Encrypt the secret file in place:

```sh
ansible-vault encrypt group_vars/production.vault.yml
```

Start from the committed `.example.yml` files. Do not reuse the database password as the restic password.

### GHCR images

The workflow publishes `booking-scheduler-web` and `booking-scheduler-api` with the full Git SHA as the only production tag. Public packages need no VPS registry credentials. For private packages, create a read-packages token for a dedicated machine identity and store its username and token only in the encrypted production Vault file.

### GitHub production environment

Create a protected GitHub environment named `production`. Configure these encrypted environment secrets:

- `VPS_SSH_PRIVATE_KEY`: the dedicated deployment key.
- `PRODUCTION_VARS_YAML`: the complete non-secret production variable YAML.
- `PRODUCTION_VAULT_YAML`: the encrypted production Vault file contents.
- `ANSIBLE_VAULT_PASSWORD`: the Vault password.

Protect `main` with the repository’s normal review and verification rules. Every push to `main` builds both linux/amd64 images, publishes the exact `${GITHUB_SHA}`, and deploys that same SHA after publication succeeds. Version tags publish images without an automatic production rollout. Manual dispatch can explicitly opt into deployment.

## Grafana Cloud host monitoring

Use a Grafana Cloud access policy with metrics-publish scope only. Record the Prometheus remote-write URL and numeric instance username in untracked `group_vars/monitoring.yml`; put only the access-policy token in `group_vars/monitoring.vault.yml`, then encrypt that file with `ansible-vault encrypt`.

Deploy the metrics-only Alloy configuration:

```sh
make monitoring
make monitoring-status
make monitoring-validate
```

The committed Alloy pipeline uses the Unix exporter collectors for CPU, memory, load, filesystem, disk I/O, and network metrics at a 60-second interval. It has no Loki, Tempo, OpenTelemetry trace, or Pyroscope pipeline.

In Grafana Cloud:

1. Enable the Linux Server integration for the stable `instance=booking-scheduler-vps` and `environment=production` labels.
2. Confirm fresh CPU, memory, load, root-filesystem, disk I/O, and network series. Use the `monitoring-freshness` Make target with the query URL and username supplied as Make variables and the access-policy token supplied only through the shell environment.
3. Create or verify a root-filesystem utilization warning whose threshold is below 80%, then exercise its preview/test path.
4. Create exactly one public HTTPS synthetic check for `https://booking-scheduler.trmaphi.work/api/v1/health/ready`, one public probe location, and a five-minute frequency. Confirm one successful result.

Do not include Grafana tokens in commands, screenshots, dashboard annotations, or support output.

## First deployment

Before the first release, run the local infrastructure gate from the repository root:

```sh
pnpm verify:infrastructure
```

Push the verified commit to `main` after bootstrap and GitHub environment configuration. The workflow publishes immutable images and deploys automatically. For an explicit operator-run first deployment instead:

```sh
make deploy RELEASE_TAG=<full-40-character-git-sha>
```

The playbook validates inputs and rendered Compose before mutation, starts PostgreSQL, runs migrations, starts healthy services, verifies public HTTPS, records the release, installs the encrypted backup timer, and never removes the PostgreSQL volume.

Verify from outside the VPS:

```sh
curl --fail --show-error --silent --head https://booking-scheduler.trmaphi.work/
curl --fail --show-error --silent https://booking-scheduler.trmaphi.work/api/v1/health/ready
openssl s_client -connect booking-scheduler.trmaphi.work:443 -servername booking-scheduler.trmaphi.work </dev/null
```

Confirm HSTS, content-type, frame, referrer, and permissions headers. Re-run the external port scan and confirm 3000, 5432, and 8080 remain closed.

## Repeat deployment and rollback

A normal code change is released by merging or pushing the verified change to `main`; CI deploys its exact SHA. Watch the protected `production` environment job and verify the recorded SHA on the host after success.

Rollback requires a previously published full SHA:

```sh
make rollback ROLLBACK_TAG=<full-40-character-prior-sha>
```

Rollback uses the same migration and health gates and preserves named volumes. Database migrations must remain backward-compatible for at least one release because rollback does not reverse schema migrations.

## Backup checks

The deploy playbook enables a nightly systemd timer with randomized delay. Inspect it without exposing credentials:

```sh
ansible production --become --module-name ansible.builtin.command --args "systemctl status booking-scheduler-backup.timer --no-pager"
ansible production --become --module-name ansible.builtin.command --args "systemctl status booking-scheduler-backup.service --no-pager"
```

Run one backup explicitly with `make backup`, then list snapshots through a protected operator session that sources `/etc/booking-scheduler/restic.env` without printing it. A successful backup requires a nonempty custom-format dump, archive validation, encrypted upload, exact `7 daily / 4 weekly / 6 monthly` retention, and `restic check` success.

## Routine maintenance

- Check root filesystem use and Docker disk use weekly; investigate before the Grafana warning threshold.
- Review failed systemd units and backup timer history.
- Confirm Caddy certificate renewal and the public synthetic check.
- Review GHCR retention without deleting the active or rollback image SHA.
- Run the restore drill in [disaster-recovery.md](disaster-recovery.md) at least quarterly.
