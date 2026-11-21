# Tasks — audit-docs-gov

## P0 (instrução quebrada)

- [x] Corrigir `Dockerfile.ci` → `Dockerfile` em CONTRIBUTING e RUNBOOK
  (ERRO::GOVERNANCA::001).
  - **Verificação:** `grep -rn "Dockerfile.ci" docs/ doc/` → 0 hits; tabela de
    Commands executável em host com Docker.
  - **Validação:** nenhum arquivo de código/build no diff; GitOps Guard verde.

## P1 (mapa vivo)

- [x] Refresh dos 5 codemaps contra o tree atual (GAP::GOVERNANCA::002,
  parte 1).
  - **Verificação:** headers `base <sha atual>`; cross-check de 1 seção por
    arquivo contra o real (ex.: `architecture.md` lista as mesmas classes que
    `android/app/src/main/java/net/basov/omngo/`).
  - **Validação:** nenhum conteúdo de código mudou; diff só em `docs/CODEMAPS/`.
- [x] Documentar a rotina de refresh no RUNBOOK (GAP::GOVERNANCA::002,
  parte 2).
  - **Verificação:** RUNBOOK tem seção "Codemaps" com o comando npx exato e o
    momento (pós-tag de release).
  - **Validação:** link bidirecional com a issue no `proposal.md` desta
    change atualizado.

## P2 (cinto opcional)

- [x] Meta-teste anti-`Dockerfile.ci` no repocheck (se aprovado pelo
  mantenedor).
  - **Verificação:** criar arquivo com a literal → Gate vermelho.
  - **Validação:** `TestCommentShare`/meta-testes existentes verdes; se este
    item entrar, commit com bump nos 3 arquivos (toca Go).

## Nota de execução (2026-11-20)

- P0 entregue em 31e20db: CONTRIBUTING e RUNBOOK apontam para o Dockerfile.
- P1: os 5 codemaps ganharam header `base 1eef9f8` e correções contra o
  tree (rotas com throttle, regras do repocheck, tabela de workflows,
  refuse-default-password, ordem do trio vendored). A rotina de refresh
  entrou no RUNBOOK com o flag `--skip-agents-md` (issue #45).
- P2 aprovado: `TestNoDeadDockerfileName` em docs_gov_test.go varre docs/
  pela literal morta. doc/ mantém a história e fica fora do scan.
- Bump 26.11.42/2611042 porque o meta-teste é código Go.
- O grep de validação usa `Dockerfile\.ci` escapado: a cópia funciona como
  regex e a literal morta não aparece nos docs.
