# Tasks — audit-docs-gov

## P0 (instrução quebrada)

- [ ] Corrigir `Dockerfile.ci` → `Dockerfile` em CONTRIBUTING e RUNBOOK
  (ERRO::GOVERNANCA::001).
  - **Verificação:** `grep -rn "Dockerfile.ci" docs/ doc/` → 0 hits; tabela de
    Commands executável em host com Docker.
  - **Validação:** nenhum arquivo de código/build no diff; GitOps Guard verde.

## P1 (mapa vivo)

- [ ] Refresh dos 5 codemaps contra o tree atual (GAP::GOVERNANCA::002,
  parte 1).
  - **Verificação:** headers `base <sha atual>`; cross-check de 1 seção por
    arquivo contra o real (ex.: `architecture.md` lista as mesmas classes que
    `android/app/src/main/java/net/basov/omngo/`).
  - **Validação:** nenhum conteúdo de código mudou; diff só em `docs/CODEMAPS/`.
- [ ] Documentar a rotina de refresh no RUNBOOK (GAP::GOVERNANCA::002,
  parte 2).
  - **Verificação:** RUNBOOK tem seção "Codemaps" com o comando npx exato e o
    momento (pós-tag de release).
  - **Validação:** link bidirecional com a issue no `proposal.md` desta
    change atualizado.

## P2 (cinto opcional)

- [ ] Meta-teste anti-`Dockerfile.ci` no repocheck (se aprovado pelo
  mantenedor).
  - **Verificação:** criar arquivo com a literal → Gate vermelho.
  - **Validação:** `TestCommentShare`/meta-testes existentes verdes; se este
    item entrar, commit com bump nos 3 arquivos (toca Go).
