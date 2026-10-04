# Contributing (fork OMN-Go-apk)

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

Contribuições entram pelo loop GitOps (skill `github-ops-cicd`, regras R1–R9).
Nada mergeia sem os checks; ninguém aprova no circuito.

## Setup
1. Fork clone + remotes:
   ```bash
   git remote add origin https://github.com/camillanapoles/OMN-Go-apk.git
   git remote add upstream https://github.com/mvbasov/OMN-Go.git
   ```
2. Sem toolchain no host: o build é Docker (`local/build.sh`) ou CI. Em
   Termux: `git` + `gh` bastam; o gate roda no Actions.
3. Segredos já vivem no repo (ver RUNBOOK §Segredos). Não commitar keystore.

## Commands

<!-- generated:source=local/cicd/*.sh;Dockerfile.ci;.github/workflows -->
| Command | Description |
|---------|-------------|
| `bash local/build.sh` | Docker build completo (base+ci) → `output-binaries/` |
| `docker buildx build -f Dockerfile.ci --target test .` | Só o quality gate (go vet+test Go/Java/JS) |
| `docker buildx build -f Dockerfile.ci --target export --output type=local,dest=artifacts .` | Artefatos (exige os 4 segredos de assinatura via `--secret`) |
| `WORKFLOW="Fork CI" bash local/cicd/observa-ci.sh [pr]` | Watcher assíncrono; notifica no Termux |
| `WORKFLOW="Fork CI" bash local/cicd/acompanha-ci.sh [run\|branch]` | Live view 10s (job/step/tempo) |
| `bash local/cicd/iniciar-sessao.sh` | Bootstrap de sessão (git+PRs+WAL) |
| `gh workflow run fork-sync.yml` | Sync upstream agora (ou cron semanal 17:04 * * 1) |
<!-- /generated -->

## Loop de contribuição
1. Spec com critérios verificáveis (M==N) → sem critérios, sem branch.
2. `git checkout -b feat/<slug>` a partir de `dev`.
3. Push → CI gateia → vermelho? corrige NO branch e repete.
4. PR → `dev` (staging) → depois `dev` → `main` (release) — sempre `--auto`.
5. Tag `vYY.MM.S-f.N` só em `main` (sufixo `-f.N` do fork; tags de release
   do upstream pertencem ao upstream).

## Testes
- Gate único: `go vet ./backend/... && go test ./backend/...` dentro do stage
  `test` do Dockerfile.ci (roda também JS `node --test` e o Java de
  `android/test/`). O build de artefato copia `/gate-passed`: sem gate, sem APK.
- O build export ROUDA o gate internamente; falha de teste mata o APK cedo.

## Estilo
- ASD-STE100 em docs, comentários e commits (doc/TERMINOLOGY.md).
- `TestNoCommentStyleFault` (frases ≤25 palavras, voz ativa), share de
  comentários ≤20%, `TestNoVersionNumberInComments`, `gofmt` limpo.
- Commit: `type(scope): Sentence. vYY.MM.S` (≤80ch) + bullets `-` (versão é
  regra do upstream; branches do fork não alteram `backend/version.go`).

## Checklist de PR
- [ ] Critérios da spec todos atendidos com evidência no corpo do PR
- [ ] `Quality Gate` + `APK Build` verdes no merge-ref
- [ ] Nenhum push direto em `main`/`dev` (bypass admin aparece auditado no output)
- [ ] Docs tocadas? codemaps (`docs/CODEMAPS/`) atualizados no mesmo PR
- [ ] Nenhum segredo/keystore no diff
