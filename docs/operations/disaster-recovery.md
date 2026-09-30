# Production disaster recovery

Production recovery depends on immutable GHCR images, the Ansible bootstrap/deployment playbooks, and encrypted restic snapshots stored off-host in R2. Never test recovery by overwriting the active database.

## Quarterly restore drill

1. Confirm the current backup service and timer succeeded.
2. List restic snapshots from a protected root session without printing `/etc/booking-scheduler/restic.env`.
3. Select an exact hexadecimal snapshot ID; `latest` is intentionally rejected.
4. Verify the snapshot into a disposable PostgreSQL database:

```sh
make restore SNAPSHOT_ID=<exact-snapshot-id>
```

The playbook downloads the encrypted snapshot into a root-only workspace, validates its `pg_restore` catalog, creates a uniquely named disposable database, restores with stop-on-error behavior, and probes `public.appointments`. The normal restore target is tagged so production cutover does not run. Record the snapshot ID, execution time, archive size, and table probe result without recording credentials or customer data.

5. Remove the disposable database after recording evidence, or retain it briefly for additional integrity checks. Production remains unchanged.

## Destructive restore cutover

**Warning: the following action stops the API and web containers and replaces the active database name. It is destructive production maintenance. Take a fresh backup, announce the outage, and obtain explicit operator confirmation before continuing.**

First run the normal restore verification for the exact snapshot. Only after it succeeds, run:

```sh
make restore-cutover SNAPSHOT_ID=<same-exact-snapshot-id>
```

The cutover task is tagged `never,restore_cutover`; it cannot run during an ordinary restore. It stops application traffic, terminates database sessions, keeps the prior database under a pre-restore name, promotes the verified candidate, and restarts health-checked services. Verify public HTTPS, a read-only appointment lookup, Grafana host metrics, and the synthetic check immediately afterward.

If verification fails, stop application traffic and reverse the database names through a reviewed maintenance procedure. Do not delete the pre-restore database until the recovery is accepted.

## Complete VPS replacement

1. Provision a new Ubuntu 24.04 x86_64 VPS with sufficient disk and memory.
2. Restrict the provider firewall to TCP 22, 80, and 443.
3. Bootstrap with the operator public key using the interactive `bootstrap-root` target; verify `deploy` key access before ending the root session.
4. Update Cloudflare DNS to the replacement address with proxying disabled during initial certificate issuance.
5. Configure the encrypted production and monitoring Vault inputs locally; never copy plaintext secrets into Git.
6. Deploy Grafana Alloy and confirm current labeled host metrics.
7. Deploy the selected immutable application SHA. This creates an empty migrated PostgreSQL volume.
8. Select and verify an exact R2 snapshot into a disposable database.
9. Schedule and execute the separately tagged restore cutover.
10. Verify HTTPS certificate validity, security headers, web rendering, API readiness, closed private ports, firewall state, fresh metrics, the root-filesystem alert, and the five-minute public synthetic check.
11. Trigger a new encrypted backup from the replacement host and verify the snapshot and repository check.
12. Preserve the old VPS until the replacement has passed application, monitoring, and backup acceptance; then retire it through the provider’s secure deletion workflow.

## Recovery evidence and secret hygiene

Keep timestamps, Git SHAs, snapshot IDs, health results, and pass/fail outcomes. Do not capture environment files, Vault content, access-policy tokens, private keys, database URLs, raw database errors, or customer records in terminal transcripts or screenshots. Run `pnpm scan:confidentiality` and inspect `git status` after documentation updates.
