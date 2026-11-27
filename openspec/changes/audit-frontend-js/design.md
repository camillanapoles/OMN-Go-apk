# Design — audit-frontend-js

## Stack envolvida

- JS vanilla (ES moderno exceto `omn-go-compat.js` ES5), IIFE + `window.*`,
  `data-action`/`OMN.action` (ADR 0020), lazy loading (ADR 0015/0021).
- Testes: `node --test` em `backend/frontend/test/` (DOM stub, `mini-dom.js`,
  `coverage.js`), rodados pelo `repocheck/js_test.go` no Gate.
- **Sem build step** — nenhum bundler/npm no produto; nada de `package.json`.

## Construção (build/test/lint)

- Gate: Fork CI roda `js_test` (que roda os testes JS e mede cobertura ≥ 60%
  por script).
- Toda extração segue as quatro listas da casa para nomes novos
  (`storage.VersionDependentAssets`, `gitsync.GitignorePatterns`,
  `storage.RetiredAssets`, `storage.RenamedAssets` — §4) quando um arquivo
  muda de nome; extrair conteúdo sem renomear não as aciona.
- `TestTemplatesHoldNoStyleBlock` e `TestCompiledPageShellStaysSmall` proíbem
  style/script novo no shell — extrações são de arquivos JS existentes.
- Commits: `refactor(frontend): ...` com bump nos 3 arquivos.

## Governança

- Merge: os 3 checks obrigatórios verdes; PR `cnmfs/*`/`arena/*` →
  `merge/sync-cnmfs`.
- Ordem: 003 (meta-teste primeiro) → 002 (migração pequena) → 001 (extração em
  passos medidos, um PR por passo).
- Restrições: NÃO tocar em `omn-go-compat.js` (ES5, posição no shell é regra);
  NÃO mudar a ordem dos sete scripts do shell; `applySyncLogLine` segue
  exportado em `window`; os stubs do branch `else` (modo `file:`) acompanham
  cada corte; `omn-go-custom.js` (arquivo do usuário) é intocável.
- Risco controlado: cada passo de 001 mantém a cobertura medida e os dois
  testes portados de comportamento (`TestEverySyncLineReachesTheOverlay`,
  `TestLogFilterPortAgreesWithTheRealJavaScript`).
