# Tasks — audit-build-ci

## P0 (contexto e custo)

- [ ] Excluir `docs/` e `openspec/` do contexto Docker (GOVERNANCA::001).
  - **Verificação:** Fork CI verde após o diff de 2 linhas no
    `.dockerignore`; grep confirma que Dockerfile/stages não leem nada dos
    dois diretórios.
  - **Validação:** `pipelines_test.go` verde (não lê dockerignore, mas o Gate
    completo precisa passar); APK Build produz os mesmos 7 assets
    (`gh release view`/artifacts do run).

## P1 (ruído de CI)

- [ ] Restringir o gatilho de `test.yml` (CONFLITO::INTEROPERABILIDADE::003).
  - **Verificação:** `gh workflow run test.yml` manual funciona
    (workflow_dispatch); próximo Fork Sync semanal não cria run do test.yml.
  - **Validação:** Fork CI e GitOps Guard inalterados; nenhum ruleset perde
    check obrigatório.

## P1 (cadeia de suprimentos)

- [ ] Pin por SHA das 9 actions + dependabot de github-actions
  (SEGURANCA::002).
  - **Verificação:** `grep -c "uses:.*@v" .github/workflows/*.yml` → 0; todos
    os runs seguintes verdes (Fork CI, Governance, Fork Sync).
  - **Validação:** nenhum segredo novo; `permissions` de cada workflow
    idênticos ao anterior ao pin; dependabot.yml com `directories: "/"`.
