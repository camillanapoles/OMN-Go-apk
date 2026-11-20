# AGENTS.md — audit-build-ci (contrato local da change)

Issue: _pendente_

## Contexto isolado

Pipelines do fork: Dockerfile único multistage (gate no stage `test`, artefatos
em `export`), Fork CI (um build por commit, checks obrigatórios), Fork Sync
(mirror semanal de `master`), Governance/Cleanup. Ler antes: CLAUDE.md §9
(build/CI), doc/gitops/README.md (modelo de branches), docs/RUNBOOK.md
(segredos) e a spec desta change.

## Arquivos permitidos para edição

- `.dockerignore`
- `.github/workflows/test.yml` (só o bloco `on:`)
- `.github/workflows/*.yml` (apenas as linhas `uses:` para SHA pin)
- `.github/dependabot.yml` (novo)
- `docs/RUNBOOK.md`/`docs/CONTRIBUTING.md` (SÓ as linhas de comando afetadas,
  ver issue docs-gov para o escopo completo de docs)

## Proibido editar

- `Dockerfile` (nenhum pin de toolchain muda aqui)
- `.gitlab-ci.yml` (pipeline de tags — espelho upstream)
- `android/app/build.gradle`, `android/build_aar.sh`,
  `metadata/net.basov.omngo.fdroid.yml` (pins de pipeline)
- Segredos e `doc/gitops/rulesets/**` (governança é outra issue)

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles`
2. `npx -y gitnexus@1.6.12 detect-changes`
3. `gh run watch` no Fork CI do PR; para workflows editados, confirmar os
   próximos runs agendados (Fork Sync/Governance) após o merge.
4. Bump de versão nos 3 arquivos (mesmo para CI, a regra §6 vale).

## Proibições do GitNexus

- `pipelines_test.go` lê os arquivos de pipeline: qualquer linha que ele
  compara (API level, Go, NDK, build-tools, Gradle, x/mobile) é intocável
  nesta change.
- Nenhuma action nova; nenhum workflow novo (o fork padronizou os existentes).
- Nunca usar `pull_request_target` nem elevar `permissions` ao pinar.
