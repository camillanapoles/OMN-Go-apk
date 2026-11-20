# Spec — build-ci (detalhamento 5W1H)

## [BUILD-CI::MELHORIA::GOVERNANCA::001] Contexto de build carrega `docs/` e `openspec/`

- **What (O quê):** `.dockerignore` lista `doc/` (com re-include de
  `doc/decisions/`) mas não `docs/` nem `openspec/`; ambos entram no contexto
  e são copiados nos stages de build (`COPY . .`).
- **Why (Por quê):** contexto maior = mais upload/resolve no buildx a cada
  Fork CI e release; documentação interna do fork (RUNBOOK com apontadores
  para segredos locais — sem valores, mas com nomes de arquivos fora do repo)
  não pertence à imagem; `openspec/` crescerá a cada auditoria.
- **Where (Onde):** `.dockerignore:1-11`; `Dockerfile:136,164` (`COPY . .`);
  afetado: `docs/**` (5 arquivos + AUDIT_PROMPT), `openspec/**` (esta
  auditoria).
- **When (Quando):** build — cada `docker build` (local, Fork CI, release).
- **Who (Quem):** mantenedor do fork; runners do GitHub Actions.
- **How (Como):** acrescentar `docs/` e `openspec/` ao `.dockerignore`
  (mantendo `!doc/decisions/` como está). Validar que `pipelines_test.go` e
  os stages não referenciam nada de `docs/`/`openspec/` (grep no Dockerfile —
  não referenciam). Critério: `docker build` (CI) verde com os dois diretórios
  fora do contexto e diff de camadas sem mudança de conteúdo relevante.

## [BUILD-CI::MELHORIA::SEGURANCA::002] Actions pinadas por tag major, não por SHA

- **What (O quê):** 9 usos de third-party/first-party actions com pin por tag
  major; sem `dependabot.yml`.
- **Why (Por quê):** tag major é mutável — um compromise do repositório da
  action (ou do registry) injeta código no contexto com `contents: write` do
  release/sync; pin por SHA é a mitigação padrão; dependabot mantém o SHA
  atualizado com PR auditável (combina com o fluxo GitOps do fork).
- **Where (Onde):** `.github/workflows/{fork-ci,test,release,android-gomobile-release,fork-sync,sync-gitlab,governance,cleanup-branches}.yml`
  (9 linhas `uses:`); ausente: `.github/dependabot.yml`.
- **When (Quando):** CI/CD — cada run.
- **Who (Quem):** mantenedor (dono dos segredos `FORK_SYNC_TOKEN`,
  `KEYSTORE_*`).
- **How (Como):** pinar cada action por SHA full com comentário da versão
  (`actions/checkout@<sha> # v4.2.2`); adicionar `.github/dependabot.yml`
  com `package-ecosystem: github-actions` + `github-actions` label. Critério:
  todos os `uses:` com 40 hex; dependabot abre PR (auditable) nas próximas
  versões; nenhum run muda de comportamento (mesmas versões resolvidas).

## [BUILD-CI::CONFLITO::INTEROPERABILIDADE::003] `test.yml` do upstream dispara no push de mirror

- **What (O quê):** `test.yml` (herdado) roda em `push`/`pull_request` para
  `master`; o `Fork Sync` atualiza `master` com push direto semanal — cada
  sync dispara um build Docker do stage `test` no fork.
- **Why (Por quê):** o gate do fork é o Fork CI (PRs de `arena/*`/`cnmfs/*` e
  staging); o build do `test.yml` no mirror é trabalho de CI que não protege
  nada que o Fork CI não cubra (o mirror é idêntico ao upstream por
  definição) — custo de minutos de runner e ruído na aba Actions.
- **Where (Onde):** `.github/workflows/test.yml:1-8` (gatilho);
  `.github/workflows/fork-sync.yml` (push direto em `master`);
  interação documentada no header do `fork-ci.yml`.
- **When (Quando):** CI — cada sync semanal (cron `17 4 * * 1`) e qualquer
  push manual ao mirror.
- **Who (Quem):** mantenedor; quota de Actions do fork.
- **How (Como):** escolher UMA: (a) desativar o gatilho push de `test.yml`
  (`workflow_dispatch` apenas — preserva o workflow para uso manual), ou (b)
  adicionar guard de repositório/branch que pule o run quando o push vier do
  Fork Sync (convention: commit message do sync). Recomendado (a) — mais
  simples e auditável. Critério: sync semanal não enfileira run do
  `test.yml`; dispatch manual continua funcionando.
