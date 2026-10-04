# HISTORY — fork GitOps session log

Each entry records the increment, the evidence, and the verdict.

## 2026-10-04 — Infrastructure increment (PR #1, merged f2c3c6a)

- Branch model live: `master` = upstream mirror, `dev` = staging, `main` = release. Default branch = `main`.
- Gate: `Quality Gate` + `APK Build` required on `main` and `dev`; auto-merge ON; strict ON; `enforce_admins=false`.
- Signing: dedicated fork keystore at `~/keystores/omn-go-fork/` (outside the repo). PKCS12 needs store password = key password; first attempt diverged and failed with `Get Key failed: bad key` — regenerated with one password, proved by `jarsigner -verify` (`jar verified`).
- Secrets live: `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD`, `FORK_SYNC_TOKEN`.
- Deploy proof: tag `v26.10.25-f.1` pushed; `android-gomobile-release.yml` builds the signed Release.

### Performance ledger (measure first)

| Item | Baseline | Evidence |
|---|---|---|
| Quality Gate job | 1m54s | run 37216757254, attempt 1/2 |
| APK Build job (cold layers) | 5m35s | run 37216757254, attempt 2 (16:38:25→16:44:00) |
| Serial critical path (needs: gate) | 7m29s | 1m54s + 5m35s |
| Layer cache | MISS on attempt 2 (zero `CACHED` steps; first success had not yet exported) | build log, no `CACHED` markers; test stage ran 59.3s inside APK job |

- Change 1: drop `needs: gate` — jobs run in parallel. The APK build already runs the gate in its `test` stage (`gate-passed` copy blocks artifacts). Expected critical path: max(1m54s, 5m35s) ≈ 5m35s. Verdict: measured on PR #2.
- Change 2: cache stays as configured (`mode=max`, scopes `omngo`/`omngo-test`). Attempt 2 was the first successful export; the next build is the true warm test. Verdict: pending next build.
- Reverted ideas: none yet.
