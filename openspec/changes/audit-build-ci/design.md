# Design — audit-build-ci

## Stack envolvida

- Docker multistage (buildx, `golang:1.26.0-trixie` base), GitHub Actions,
  GitLab CI (espelho de tags). **Nenhuma mudança de toolchain** — os pins de
  Go/NDK/Gradle/x-mobile são governados por `pipelines_test.go` e ficam
  intactos.

## Construção (build/test/lint)

- Validação local impossível (host sem Docker) — tudo via Fork CI e release.
- O repocheck lê workflows (`pipelines_test.go`): rodar o Gate após qualquer
  edição de `.github/workflows/`.
- `test.yml` não é check obrigatório de nenhum ruleset — mudar seu gatilho
  não afeta os checks exigidos (Quality Gate/APK Build/GitOps Guard vivem no
  Fork CI e no guard).
- Commits: `build(github): ...`/`chore(github): ...` com bump nos 3 arquivos.

## Governança

- Merge: 3 checks obrigatórios verdes; PR `cnmfs/*`/`arena/*` →
  `merge/sync-cnmfs`.
- Ordem: 001 (dockerignore — trivial) → 003 (gatilho) → 002 (SHA pin +
  dependabot, o maior diff).
- Restrições: nunca editar os pins de toolchain do Dockerfile/Gradle/recipe
  nesta issue (domínio de outra decisão, ADR/pipelines_test); segredos não
  aparecem em nenhum diff; o `Fork Sync` segue com PAT (`FORK_SYNC_TOKEN`) —
  nenhuma mudança de permissão de workflow.
- Break-glass: mudanças de workflow só por PR (nunca push direto) —
  `main`/`merge/sync-cnmfs` exigem PR por ruleset.
