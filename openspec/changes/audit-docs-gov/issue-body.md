Spec OpenSpec: [`openspec/changes/audit-docs-gov/`](../../openspec/changes/audit-docs-gov/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[DOCS-GOV::ERRO::GOVERNANCA::001]` | ERRO | CONTRIBUTING/RUNBOOK instruem `Dockerfile.ci` que não existe (fork-ci.yml:7-8 documenta a morte) |
| `[DOCS-GOV::GAP::GOVERNANCA::002]` | GAP | `docs/CODEMAPS/*` parados em `base d9f6c82` (2026-10-04) sem rotina de refresh |

## Checklist macro (P0 → P2)

- [ ] **P0** `Dockerfile.ci` → `Dockerfile` em CONTRIBUTING.md (:24,:25,:42) e RUNBOOK.md (:10,:16)
- [ ] **P1** Refresh dos 5 codemaps contra o tree atual (headers `base <sha>`)
- [ ] **P1** Rotina de refresh documentada no RUNBOOK (GitNexus `analyze` pós-tag)
- [ ] **P2** (opcional, se mantenedor aprovar) meta-teste anti-literal `Dockerfile.ci` no repocheck

Escopo fechado: `docs/**` é pt-BR, `doc/**` (upstream) é STE e intocável nesta issue.
Contrato local: `AGENTS.md` dentro do diretório da spec.
