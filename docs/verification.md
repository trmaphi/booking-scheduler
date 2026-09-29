# Delivery verification evidence

This file separates the original pre-cleanup evidence from the final sanitized delivery run. An unchecked or failed command is not represented as passing.

## Final sanitized delivery run

- UTC date: 2026-09-29T12:32:01Z
- Tested commit: `58b0109c6b9c5a877de7ff423e904541c0e3f57a`
- Node.js: 25.9.0 (the Compose web image uses Node.js 22)
- pnpm: 10.17.1
- Go: 1.27.1 darwin/arm64 (Compose uses Go 1.27)
- Docker Compose: 5.3.1
- OpenSpec: 1.13.1

`pnpm verify:delivery` passed all 18 sequential steps from empty volumes. The generated client was stable; formatting and lint passed; 85 frontend and delivery tests passed with 1 opt-in integration test skipped; explicit Next route type generation, TypeScript checking, and the production build passed. Go formatting, vet, the complete PostgreSQL race suite, the concurrency/idempotency group repeated 20 times, and both production command builds passed. The migrated and fictionally seeded Compose stack passed four smoke assertions. The Playwright booking, reload, API read-back, and post-browser durability journey passed. OpenSpec strict validation passed. The scanner found no issue in tracked files or any reachable commit, blob, path, commit metadata, tag metadata, or tag ref. The final working-tree check passed.

The evidence update and task checkbox are documentation-only changes. The complete 18-step gate was rerun after committing them, and it passed without modifying the working tree.

## Original empty-volume gate run

- UTC date: 2026-09-29T11:40:09Z
- Commit: `9d0b8d3d08c189d22b5f768b57f447d8574d00ba`
- Node.js: 25.9.0 (the Compose web image uses Node.js 22)
- pnpm: 10.17.1
- Go: 1.27.1 darwin/arm64 (Compose uses Go 1.27)
- Docker Compose: 5.3.1
- OpenSpec: 1.13.1

The following table applies only to the `9d0b8d3d08c189d22b5f768b57f447d8574d00ba` run above. It is historical evidence, not the final delivery result.

### Command groups

| Group                                      | Result        | Evidence                                                                                                                                                                          |
| ------------------------------------------ | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Delivery-script contract tests             | PASS          | 9 focused tests existed and passed at the time of this original gate run                                                                                                          |
| Generated client and frontend              | PASS          | Generated output stable; format and lint clean; 36 passed, 1 opt-in test skipped; type check and Next.js production build passed                                                  |
| Go formatting, vet, tests, race, and build | PASS          | Formatting and vet clean; 138 tests discovered across the full PostgreSQL/race suite; repeated concurrency/idempotency group passed 20 times; server and migration commands built |
| Empty-volume migration, seed, and smoke    | PASS          | Volumes removed; migrations and fictional seeds applied; web, liveness, readiness, and schema checks produced 4 PASS results                                                      |
| Playwright booking and persisted reload    | PASS          | 1 browser journey passed, including API read-back after reload                                                                                                                    |
| OpenSpec strict validation                 | PASS          | Change validated with `--strict`                                                                                                                                                  |
| Confidentiality scan                       | EXPECTED FAIL | 8 prohibited-term findings, all in reachable historical blobs; 0 tracked-file findings and no PDF, credential, fixture-policy, or provider-commitment finding                     |
| Clean Git tree                             | NOT RUN       | Sequential gate stopped at the confidentiality finding; final history rewrite and rerun are required                                                                              |

The exact command `pnpm verify:delivery` ran against that original commit. Steps 1–16 passed and step 17 failed on the classified legacy-history findings, so step 18 did not run.

## Later scanner-hardening checks

These were focused scanner and static checks, not a rerun of the empty-volume delivery gate. The hardened scanner emits only category, scope, and an opaque locator hash; it never prints matched content, paths, commit messages, ref names, or object names.

- The disposable-repository scanner suite passed 58 cases after tag-ref coverage was added.
- The latest full frontend/static run after tag-ref coverage passed 85 tests with 1 opt-in test skipped, plus formatting, lint, type checking, generated-client consistency, and strict OpenSpec validation. The preceding hardening run also proved the frozen install.
- The hardened repository scan found 12 legacy-history findings: 8 prohibited-term and 4 source-fingerprint findings. All had `history_blob` scope and opaque locators; there were zero tracked-file, commit-metadata, tag-metadata, tag-ref, PDF, credential, fixture-policy, or provider-commitment findings.

The historical section above records why cleanup was necessary. The final sanitized run supersedes it for delivery readiness.
