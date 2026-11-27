# Índice da Auditoria Arquitetural (2026-11-19/20)

Auditoria executada conforme `docs/AUDIT_PROMPT.md` — branch de sessão
`arena/d9c9b8dd-omn-go-apk`, commit base `02e8cd8`, grafo GitNexus 6.217 nós /
20.613 arestas / 527 flows. **24 itens** em **8 specs**; 2 falsos positivos
descartados no loop de validação (traversal de upload: stdlib aplica
`filepath.Base` + meta-teste da casa; WAL do SQLite: decisão documentada para
FUSE/Android). Domínios `render-search`, `sync-git` e `db` foram descartados
por não terem achados defensáveis.

**Prioridade máxima: `audit-skills-harness`** — governança de skills dos
agentes (fontes validadas: multica-ai/andrej-karpathy-skills, mattpocock/skills,
chrisbanes/skills, affaan-m/ECC, jev-ai/jev-agent-skill — esta última com
gate de serviço externo: `JEV_API_KEY` fora do tree e ativação opt-in por
estágio). Execute esta issue ANTES das demais: ela otimiza os agentes que
executarão as outras 7 (ver `docs/AUDIT_PROMPT.md` FASE S e CLAUDE.md §12).

## Mapa Topológico Geral

```
audit-skills-harness (dom:skills-harness) ★ PRIORIDADE MÁXIMA
├── GAP      :: GOVERNANCA        :: 1   [::001] skills não versionadas/replicáveis
├── GAP      :: INTEROPERABILIDADE :: 1  [::002] sem gate de auditoria de skill
├── MELHORIA :: INTEROPERABILIDADE :: 1  [::003] formato universal + fontes pinadas
└── MELHORIA :: USABILIDADE       :: 1   [::004] pool inativo + seleção por-spec
audit-config (dom:config)
├── CONFLITO :: SEGURANCA         :: 1   [CONFIG::CONFLITO::SEGURANCA::001]
├── MELHORIA :: USABILIDADE       :: 1   [::002]
└── MELHORIA :: GOVERNANCA        :: 1   [::003]
audit-backend-core (dom:backend-core)
├── MELHORIA :: SEGURANCA         :: 2   [::001] login sem rate limit, [::002] headers
├── MELHORIA :: CICLOMATICIDADE   :: 1   [::003] branch morta localhost
└── GAP      :: GOVERNANCA        :: 1   [::004] meta-teste de headers ausente
audit-storage-exchange (dom:storage)
├── GAP      :: GOVERNANCA        :: 1   [::001] lista de exts Go↔Java sem meta-teste
├── MELHORIA :: SEGURANCA         :: 1   [::002] SVG executável sem sandbox
└── MELHORIA :: CODIGO            :: 1   [::003] duplicação das vias de upload
audit-frontend-js (dom:frontend-js)
├── MELHORIA :: CICLOMATICIDADE   :: 1   [::001] omn-go-api.js 918 linhas
├── MELHORIA :: CODIGO            :: 1   [::002] innerHTML no console UI
└── GAP      :: CODIGO            :: 1   [::003] lint anti-innerHTML ausente
audit-android (dom:android)
├── MELHORIA :: INTEROPERABILIDADE :: 1  [::001] file access do WebView por default de API
└── GAP      :: GOVERNANCA        :: 1   [::002] teto 800 linhas sem meta-teste
audit-build-ci (dom:build-ci)
├── MELHORIA :: GOVERNANCA        :: 1   [::001] dockerignore sem docs/ e openspec/
├── MELHORIA :: SEGURANCA         :: 1   [::002] actions por tag major, sem dependabot
└── CONFLITO :: INTEROPERABILIDADE :: 1  [::003] test.yml roda no push do mirror
audit-docs-gov (dom:docs-gov)
├── ERRO     :: GOVERNANCA        :: 1   [::001] docs instruem Dockerfile.ci morto
└── GAP      :: GOVERNANCA        :: 1   [::002] codemaps do fork sem rotina de refresh
```

Totais: **CONFLITO 2 · ERRO 1 · GAP 7 · MELHORIA 14 = 24 itens.**

## Tabela de Rastreabilidade

| Tag da Spec | Diretório OpenSpec | AGENTS.md | GitHub Issue # |
| --- | --- | --- | --- |
| `audit-skills-harness` ★ | `openspec/changes/audit-skills-harness/` | `audit-skills-harness/AGENTS.md` | #30 |
| `audit-config` | `openspec/changes/audit-config/` | `audit-config/AGENTS.md` | #31 |
| `audit-backend-core` | `openspec/changes/audit-backend-core/` | `audit-backend-core/AGENTS.md` | #32 |
| `audit-storage-exchange` | `openspec/changes/audit-storage-exchange/` | `audit-storage-exchange/AGENTS.md` | #33 |
| `audit-frontend-js` | `openspec/changes/audit-frontend-js/` | `audit-frontend-js/AGENTS.md` | #34 |
| `audit-android` | `openspec/changes/audit-android/` | `audit-android/AGENTS.md` | #35 |
| `audit-build-ci` | `openspec/changes/audit-build-ci/` | `audit-build-ci/AGENTS.md` | #36 |
| `audit-docs-gov` | `openspec/changes/audit-docs-gov/` | `audit-docs-gov/AGENTS.md` | #37 |

Issues criadas em 2026-11-20 por `openspec/changes/create-issues.sh`
(rastreabilidade bidirecional fechada: cada `proposal.md` e `AGENTS.md`
carrega o número da issue). Ordem de execução: **#30 primeiro** → #31 → #32 →
#37 → demais. Remover o label `spec:draft` de cada issue quando o refinamento
M==N for aprovado pelo mantenedor.



## Conformidade da execução (FASE 4)

- Diff contém apenas `openspec/**`, `docs/AUDIT_PROMPT.md` e a entry de
  higiene `.gitignore` (`.gitnexus/`, `.claude/`).
- Nenhum arquivo de produção alterado; sem `.claude/`, `AGENTS.md` de raiz ou
  `.gitnexus/` no tree.
- Quality Gate do Fork CI **verde** nos commits da auditoria (runs
  44590409395, 44648264656, 44649059643).
- Commits 100% docs/openspec: sem bump de `APP_VERSION` (exceção da FASE 0.6).
