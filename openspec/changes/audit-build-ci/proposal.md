# Proposal — audit-build-ci

> **Issue:** _pendente_
> **Domínio:** `build-ci` — `Dockerfile`, `.dockerignore`,
> `.github/workflows/`, `.gitlab-ci.yml`.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

O build de referência é um único `Dockerfile` multistage (`sys_env` →
`android_env` → `go_env` → `base_env` → `test` → `project_builder` →
`export`); `project_builder` copia `/gate-passed` do stage `test`, então
nenhum artefato sai de um build com gate falhado. O fork tem o **Fork CI**
(quality gate + APK por PR/push com lógica "one build per commit"), Fork Sync
(semanal, fast-forward do mirror `omn-go/sync` com push direto), Governance
(aplica rulesets), Cleanup de branches `arena/*` pousadas, e o `test.yml`
herdado do upstream. `pipelines_test.go` compara os pins (Go/NDK/build-tools/
Gradle/x-mobile) entre Dockerfile, recipe F-Droid, `build_aar.sh` e Gradle.
Os workflows escopam `permissions` por job e não usam `pull_request_target`.
Na FASE 1 confirmou-se: sem `.github/dependabot.yml`; 9 usos de actions
pinados por tag major; `.dockerignore` exclui `doc/` mas não `docs/` nem
`openspec/`; `test.yml` dispara em push para `master`.

## Listas identificadas (tags topológicas)

### MELHORIA
- `[BUILD-CI::MELHORIA::GOVERNANCA::001]` — `.dockerignore` não exclui
  `docs/` (só `doc/`!) nem `openspec/`: os codemaps do fork, RUNBOOK,
  CONTRIBUTING, AUDIT_PROMPT e toda a árvore OpenSpec entram no build context
  e são copiados nos stages (`COPY . .` em :136 e :164).
- `[BUILD-CI::MELHORIA::SEGURANCA::002]` — As 9 referências de actions estão
  pinadas por tag major (`actions/checkout@v4`, `docker/setup-buildx-action@v3`,
  `crazy-max/ghaction-github-runtime@v3`, `actions/upload-artifact@v4`) e não
  por SHA; não existe `dependabot.yml` para ao menos automatizar o bump.

### CONFLITO
- `[BUILD-CI::CONFLITO::INTEROPERABILIDADE::003]` — O `test.yml` herdado do
  upstream dispara em `push: branches: [master]` — e o Fork Sync faz push
  direto no `master` (mirror) a cada sync semanal: cada sync paga um build
  Docker completo (`test` stage) que não faz parte do fluxo GitOps do fork
  (o gate do fork é o Fork CI, em PRs de `arena/*`/`cnmfs/*`).

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `.dockerignore` | build context de todos os stages | BAIXO | `repocheck/pipelines_test.go` lê Dockerfile (não o dockerignore) — sem guarda; validar na task |
| `.github/workflows/*.yml` (pins) | todos os pipelines | MÉDIO | nenhum; validação manual por run |
| `.github/workflows/test.yml` (gatilho) | sync semanal | BAIXO | Fork CI não depende dele; `cleanup/fork-sync` independentes |

Regra da casa aplicável: "The three builds must agree" (§9) — nenhuma mudança
de pins de toolchain aqui; os itens são de contexto de build e gatilhos.
