# Tasks — audit-build-ci

## P0 (contexto e custo)

- [x] Excluir `docs/` e `openspec/` do contexto Docker (GOVERNANCA::001).
  - **Verificação:** Fork CI verde após o diff de 2 linhas no
    `.dockerignore`; grep confirma que Dockerfile/stages não leem nada dos
    dois diretórios.
  - **Validação:** `pipelines_test.go` verde (não lê dockerignore, mas o Gate
    completo precisa passar); APK Build produz os mesmos 7 assets
    (`gh release view`/artifacts do run).

## P1 (ruído de CI)

- [x] Restringir o gatilho de `test.yml` (CONFLITO::INTEROPERABILIDADE::003).
  - **Verificação:** `gh workflow run test.yml` manual funciona
    (workflow_dispatch); próximo Fork Sync semanal não cria run do test.yml.
  - **Validação:** Fork CI e GitOps Guard inalterados; nenhum ruleset perde
    check obrigatório.

## P1 (cadeia de suprimentos)

- [x] Pin por SHA das 9 actions + dependabot de github-actions
  (SEGURANCA::002).
  - **Verificação:** `grep -c "uses:.*@v" .github/workflows/*.yml` → 0; todos
    os runs seguintes verdes (Fork CI, Governance, Fork Sync).
  - **Validação:** nenhum segredo novo; `permissions` de cada workflow
    idênticos ao anterior ao pin; dependabot.yml com `directories: "/"`.

## Nota de execução (2026-11-20)

- P0: docs/ e openspec/ entram no .dockerignore. Desvio minimo e
  necessario: docs_gov_test.go ganhou t.Skip quando docs/ nao existe no
  tree (padrao da casa do api_doc_test), senao o gate Docker quebrava com
  o contexto novo.
- P1-1: o on: do test.yml virou workflow_dispatch somente. O comentario de
  cabecalho do arquivo descreve os gatilhos antigos; o contrato so permite
  o bloco on:, entao o comentario fica para a revisao do mantenedor.
- P1-2: 17 linhas uses: pinadas por SHA (5 actions: checkout v4, buildx
  v3, github-runtime v3, action-gh-release v2, upload-artifact v4), cada
  uma com o comentario da tag. dependabot.yml novo, github-actions semanal.
- Bump 26.11.46/2611046 (neste commit de conclusao; o commit 50f0241 ja
  nomeava a versao na mensagem).
