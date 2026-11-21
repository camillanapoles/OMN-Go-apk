# Tasks — audit-frontend-js

## P0 (rede de segurança antes do refactor)

- [x] Meta-teste lint de `innerHTML` nos scripts da app (GAP::CODIGO::003).
  - **Verificação:** caso sintético `el.innerHTML = valor` num script da app →
    Gate vermelho; `el.innerHTML = ''` permitido; vendored fora do escopo.
  - **Validação:** `js_test.go` e cobertura dos scripts inalterados.

## P1 (consistência)

- [x] Migrar `omn-go-console.js` para `createElement`/`textContent`
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

## Nota de execução (2026-11-20)

- P0: `TestNoDynamicInnerHTML` em js_test.go. Formas seguras são o clear
  `= ''`. Onze atribuições antigas ficam pinadas em `innerHTMLExemptions`,
  com a linha exata. O regex cobre o valor na linha seguinte
  (`el.innerHTML =`). Critério sintético verificado: valor dinâmico novo
  quebra o gate.
- P1: omn-go-console.js migrou para createElement/textContent com os
  helpers `iconButton` e `renderConsoleCount`. O DOM é o mesmo: mesmos ids
  e classes. Os 5 innerHTML saíram; o arquivo está fora da lista de
  exceções. Testes JS 299/0.
- P2 fica para PRs separados (a tasks.md manda um PR por passo). O Passo 1
  tem um conflito de contrato a resolver: TestSyncLogShapeHasAFrontendCopy
  (ports_test.go) e TestLogFilterPortAgreesWithTheRealJavaScript
  (js_port_test.go) leem e executam o mirror pelo nome omn-go-api.js, e os
  dois arquivos de teste estão na lista proibida desta change. Extrair o
  bloc deles exige emendar a porta ou a lista.
- Bump 26.11.44/2611044 (lint Go novo).
