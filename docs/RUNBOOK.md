# Runbook (fork OMN-Go-apk)

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Deploy (Release assinada)
```bash
git checkout main && git pull
git tag vYY.MM.S-f.N <sha-em-main> && git push origin vYY.MM.S-f.N
```
Tag dispara `android-gomobile-release.yml` → build Dockerfile.ci (gate interno)
→ Release com 7 assets (universal 69 MB, 4 ABIs, 2 desktop). Verificar:
`gh release view vYY.MM.S-f.N --json assets` (7 itens) e run success ~6 min.

## Segredos (Actions)

<!-- generated:source=.github/workflows/*.yml;Dockerfile.ci -->
| Secret | Required | Purpose | Rotação |
|---|---|---|---|
| `KEYSTORE_BASE64` | Yes | Keystore PKCS12 do fork (b64) | Só se comprometida; queima atualizações |
| `KEYSTORE_PASSWORD` | Yes | Store password (= key password) | idem |
| `KEY_ALIAS` | Yes | `omngo-fork` | fixo |
| `KEY_PASSWORD` | Yes | Key password (igual à store — PKCS12 exige) | idem |
| `FORK_SYNC_TOKEN` | Yes | PAT p/ push/PR que dispara CI (GITHUB_TOKEN não dispara) | PAT do owner |
Cópia local dos valores: `~/keystores/omn-go-fork/omn-go-fork.env` (fora do repo).
<!-- /generated -->

## Sync upstream
- Automático: `fork-sync.yml`, cron `17 4 * * 1` (fast-forward de master,
  PR master→main com auto-merge). Manual: `gh workflow run fork-sync.yml`.
- master divergido do upstream → o workflow falha de propósito; reparar à mão.

## Health checks
- CI: `gh run list --workflow "Fork CI" --branch main --limit 1` → success.
- Sync: run semanal `Fork Sync` success (up-to-date = 9s).
- Release: 7 assets presentes. Observação ao vivo: `acompanha-ci.sh`.

## Problemas conhecidos → fix
| Sintoma | Causa | Fix |
|---|---|---|
| Zero fases de cache + `Post cache: State not set` | runtime env não exposto a `run:` + `actions` sem write | passo `crazy-max/ghaction-github-runtime@v3` + `actions: write` |
| Manifest importa, zero `CACHED`, mais lento que frio | blobs evictados (pool 10 GB, múltiplos exports) | export único já policy em `dev`; se violada, re-exportar 1× do `dev` |
| `Get Key failed: bad key` no gradle | PKCS12 store≠key password | regenerar (sem arquivo antigo) + `jarsigner -verify` local antes do CI |
| Push em `main`/`dev` mostra "Bypassed rule violations" | `enforce_admins=false` (escape auditável do admin) | esperado; evite — use PR |
| PR de bot nunca mergea | `GITHUB_TOKEN` não dispara workflows | usar PAT (`FORK_SYNC_TOKEN`) |

## Rollback
- Código: PR de revert contra `main` (auto-merge no verde). Nunca force-push
  (proteção bloqueia; admin bypass só em emergência auditada).
- APK: instalar asset da Release anterior (assinatura do fork é estável entre
  versões; mesma key = upgrade/downgrade ok).
- Cache envenenado: `gh cache delete --all` + 1 push em `dev` re-exporta.

## Escalação
Repo público → Actions ilimitado, sem janela/watchdog. Falha de infra GitHub:
ver status.github.com; billing blocked não se aplica (público = 0 min).
Dono: camillanapoles (admin; escape hatch com trilha no push output).
