# AGENTS.md — audit-frontend-js (contrato local da change)

Issue: #34

## Contexto isolado

Frontend sem build step: scripts IIFE com `window.*`, páginas compiladas pelo
servidor, shell de `templates/index.html` copiado para cada nota (7 scripts em
ordem fixa + vendored + custom por último). O arquivo central é
`omn-go-api.js` (chamadas ao backend + lazy loader). Ler antes: CLAUDE.md §4
(frontend rules), ADR 0015, 0020, 0021 e a spec desta change.

## Arquivos permitidos para edição

- `backend/frontend/html/js/OMN-Go/omn-go-api.js`,
  `omn-go-console.js`, `omn-go-db.js`
- `backend/frontend/test/**` (testes JS + stubs)
- `backend/internal/repocheck/js_test.go` (lint novo)
- `backend/frontend/html/css/OMN-Go/omn-go-console.css` (se a migração exigir)

## Proibido editar

- `backend/frontend/templates/index.html` e qualquer template (shell é regra)
- `omn-go-compat.js` (ES5 — intocável)
- `omn-go-custom.js` / `omn-go-custom.css` (arquivos do usuário)
- `backend/frontend/html/js/OMN-Go/*.min.js` (vendored)
- `backend/internal/app/**` (o Go não muda nesta change)

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles`
2. `npx -y gitnexus@1.6.12 detect-changes`
3. Node local disponível: `node --test backend/frontend/test/` como pré-checagem
   rápida (o Gate oficial é o Fork CI — `gh run watch`).
4. Bump de versão nos 3 arquivos.

## Proibições do GitNexus

- `omn-go-api.js` tem o maior fan-in do frontend: cada corte precisa manter os
  exports `window.*` e os stubs dos DOIS lugares (`omnLazyActions` e o branch
  `else` do guard `file:`) — `lazy.test.js` falha com nome faltando.
- Nenhum nome novo de script/estilo sem as quatro listas da casa
  (`VersionDependentAssets`, `GitignorePatterns`, `RetiredAssets`,
  `RenamedAssets`).
- Nada de `alert/confirm/prompt` (`TestNoBrowserDialog`) nem handler inline em
  template (`TestTemplatesHoldNoInlineHandler`).
