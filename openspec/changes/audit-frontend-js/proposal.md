# Proposal — audit-frontend-js

> **Issue:** _pendente_
> **Domínio:** `frontend-js` — `backend/frontend/html/js/OMN-Go/` (scripts da
> app), `backend/frontend/html/css/OMN-Go/` e `backend/frontend/templates/`.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

O frontend não tem build step (sem `package.json`/bundler — CLAUDE.md §4) e
tem infraestrutura de teste real: `backend/frontend/test/` roda via `node
--test` dentro do `repocheck/js_test.go`, com meta de **60% de linhas
cobertas por script** (`jsLineCoverageFloor`). Os padrões são enforçados por
meta-testes: `TestNoBrowserDialog`, `TestBookmarkerWritesNoValueAsHTML`,
`TestTemplatesHoldNoInlineHandler`, `TestCompatScriptIsFirstAndES5`,
`TestEachNameInAStylesheetHasThePrefix`, entre outros. Verificou-se na FASE 1:
nenhum `alert()/confirm()/prompt()` nos scripts da app; os `console.log`
restantes são o mirror documentado de logs (`omn-go-api.js:671`) e código
vendored; os `innerHTML` da app estão concentrados no console UI e na limpeza
de listas (`= ''`, seguro).

## Listas identificadas (tags topológicas)

### MELHORIA
- `[FRONTEND-JS::MELHORIA::CICLOMATICIDADE::001]` — `omn-go-api.js` tem **918
  linhas** (o dobro do segundo maior script da app, `omn-go-core.js`, 523) e
  acumula duas responsabilidades: tudo que chama o backend (fetch/API) e o
  mecanismo de lazy loading (`omnLoadModule`/`omnLazy`/`omnLazyActions`).
- `[FRONTEND-JS::MELHORIA::CODIGO::002]` — `omn-go-console.js` constrói a UI
  com `innerHTML` para strings constantes (console.js:41, 57, 58, 66) — hoje
  seguro, mas contradiz a prática da casa de escrever valores com
  `textContent` (regra enforçada só no Bookmarker) e um futuro valor dinâmico
  nesse padrão vira XSS sem teste que pegue.

### GAP
- `[FRONTEND-JS::GAP::CODIGO::003]` — O meta-teste
  `TestBookmarkerWritesNoValueAsHTML` cobre só `Bookmarker.js`; nenhum
  meta-teste impede `innerHTML =` com valor dinâmico nos demais scripts da app
  (`omn-go-console.js` hoje, qualquer script amanhã).

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `omn-go-api.js` | carregado em toda página de nota + exportado `window.applySyncLogLine` usado por `omn-go-sync.js` | ALTO | `TestEverySyncLineReachesTheOverlay`, `lazy.test.js` (roda cada ação/exports), cobertura 60% |
| `omn-go-console.js` | hook de console em toda página; carrega antes dos scripts modernos | MÉDIO | cobertura js_test, `console.test.js` (se existir caso), `dom-page.js` |
| Ordem de carga (index.html shell) | shell copiado para cada nota compilada | ALTO | `TestCompatScriptIsFirstAndES5`, `TestCompiledPageShellStaysSmall` |

Regra da casa: extração de módulo DEVE usar o padrão lazy existente
(`omnLazyActions`/`omnLazy` + stubs no branch `else` do guard `file:`),
nunca um script a mais no shell (§4, ADR 0015/0021).
