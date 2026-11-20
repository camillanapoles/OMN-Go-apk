Spec OpenSpec: [`openspec/changes/audit-frontend-js/`](../../openspec/changes/audit-frontend-js/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[FRONTEND-JS::MELHORIA::CICLOMATICIDADE::001]` | MELHORIA | `omn-go-api.js` com 918 linhas (2× o segundo maior) |
| `[FRONTEND-JS::MELHORIA::CODIGO::002]` | MELHORIA | `innerHTML` no console UI (strings constantes hoje) |
| `[FRONTEND-JS::GAP::CODIGO::003]` | GAP | Sem lint anti-`innerHTML` para os demais scripts da app |

## Checklist macro (P0 → P2)

- [ ] **P0** Meta-teste lint de `innerHTML` nos scripts da app (exceto vendored)
- [ ] **P1** Migrar `omn-go-console.js` para `createElement`/`textContent`
- [ ] **P2** Passo 1: extrair console mirror de `omn-go-api.js` (exports `window.*` preservados)
- [ ] **P2** Passo 2: mover cliente de DB para `omn-go-db.js` (lazy, stubs nos dois lados)

Restrições duras: shell de `index.html` intocável; `omn-go-compat.js` ES5 intocável; cobertura JS ≥ 60% por script no Gate.
Contrato local: `AGENTS.md` dentro do diretório da spec.
