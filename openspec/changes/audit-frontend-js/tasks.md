# Tasks — audit-frontend-js

## P0 (rede de segurança antes do refactor)

- [ ] Meta-teste lint de `innerHTML` nos scripts da app (GAP::CODIGO::003).
  - **Verificação:** caso sintético `el.innerHTML = valor` num script da app →
    Gate vermelho; `el.innerHTML = ''` permitido; vendored fora do escopo.
  - **Validação:** `js_test.go` e cobertura dos scripts inalterados.

## P1 (consistência)

- [ ] Migrar `omn-go-console.js` para `createElement`/`textContent`
  (MELHORIA::CODIGO::002).
  - **Verificação:** teste do console pressiona clear/abrir/fechar e os ids
    `omn-console-*` permanecem; cobertura do arquivo ≥ 60%.
  - **Validação:** `TestEachNameInATemplateHasThePrefix` e `js_test` verdes;
    nenhum id/classe renomeado.

## P2 (estrutura — um PR por passo)

- [ ] Passo 1 de 001: extrair o console mirror de `omn-go-api.js`.
  - **Verificação:** `TestEverySyncLineReachesTheOverlay` e
    `TestLogFilterPortAgreesWithTheRealJavaScript` verdes sem edição;
    `window.applySyncLogLine` segue exportado.
  - **Validação:** cobertura total ≥ 60%; shell de `index.html` sem diff
    (`TestCompiledPageShellStaysSmall`).
- [ ] Passo 2 de 001: mover o cliente de DB para `omn-go-db.js` (lazy).
  - **Verificação:** `lazy.test.js` roda as ações extraídas (stubs
    `omnLazyActions` atualizados nos dois lados: lazy + branch `file:`).
  - **Validação:** `TestEachAppScriptAndStyleIsVersionDependent` e
    `TestVersionDependentAssetsAreGitignored` verdes (nenhum nome novo de
    arquivo — conteúdo movido, não renomeado).
