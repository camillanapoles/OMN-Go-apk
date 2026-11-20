Spec OpenSpec: [`openspec/changes/audit-build-ci/`](../../openspec/changes/audit-build-ci/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[BUILD-CI::MELHORIA::GOVERNANCA::001]` | MELHORIA | `.dockerignore` sem `docs/` e `openspec/` (contexto de build) |
| `[BUILD-CI::MELHORIA::SEGURANCA::002]` | MELHORIA | 9 actions por tag major, sem SHA; sem dependabot.yml |
| `[BUILD-CI::CONFLITO::INTEROPERABILIDADE::003]` | CONFLITO | `test.yml` do upstream roda a cada sync semanal (push no mirror `master`) |

## Checklist macro (P0 → P1)

- [ ] **P0** Excluir `docs/` e `openspec/` no `.dockerignore`
- [ ] **P1** Restringir gatilho de `test.yml` a `workflow_dispatch`
- [ ] **P1** Pin por SHA das 9 actions + `.github/dependabot.yml` (github-actions)

Restrições duras: nenhum pin de toolchain muda (`pipelines_test.go` governa Go/NDK/Gradle/x-mobile); segredos intocáveis; mudança de workflow sempre por PR.
Contrato local: `AGENTS.md` dentro do diretório da spec.
