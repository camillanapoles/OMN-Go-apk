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

- Change 1: drop `needs: gate` — jobs run in parallel. The APK build already runs the gate in its `test` stage (`gate-passed` copy blocks artifacts). Expected critical path: max(1m54s, 5m35s) ≈ 5m35s. **Verdict: KEEP — PR #2 measured 5m18s wait, merge-ref run 5m49s, dev-push run 4m34s; two jobs ran at the same time. −26% against the 7m29s serial path.**
- Change 2: make the gha layer cache work. First attempt (`actions: write` permission) was **insufficient alone** — the log still showed no import/export phases and `Post cache: State not set`. True root cause: the `type=gha` backend reads `ACTIONS_CACHE_URL` + `ACTIONS_RUNTIME_TOKEN`, which the runner hands to actions only; a plain `run:` step needs `crazy-max/ghaction-github-runtime@v3` to expose them. Both changes are in. Verdict: the first build with the runtime step exports the cache; the next build (post-merge `main` push) is the warm measurement.
- The upstream `android-gomobile-release.yml` has the same silent gap (its builds always run cold). Left untouched on purpose; the fork's workflow files stay free of upstream merge conflicts.
- Change 3: disabled the `sync-gitlab.yml` workflow on the fork (no GitLab remote; it failed on each push and added permanent red noise). Not a performance change; removes false alarms from the observer.
- Deploy proof: Release `OMN-Go 26.10.25` (tag `v26.10.25-f.1`) published with 7 assets: universal APK 69 MB, four ABI APKs, two desktop binaries. Build time 5m50s (16:46:48→16:52:38).
- Reverted ideas: none yet.
