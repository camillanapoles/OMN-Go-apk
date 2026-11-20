# Índice da Auditoria Arquitetural (2026-11-19)

Auditoria executada conforme `docs/AUDIT_PROMPT.md` — branch de sessão
`arena/d9c9b8dd-omn-go-apk`, commit base `02e8cd8`, grafo GitNexus 6.217 nós /
20.613 arestas / 527 flows. **20 itens** em **7 specs**; 2 falsos positivos
descartados no loop de validação (traversal de upload: stdlib aplica
`filepath.Base` + meta-teste da casa; WAL do SQLite: decisão documentada para
FUSE/Android). Domínios `render-search`, `sync-git` e `db` foram descartados
por não terem achados defensáveis.

## Mapa Topológico Geral

```
audit-config (dom:config)
├── CONFLITO :: SEGURANCA         :: 1   [CONFIG::CONFLITO::SEGURANCA::001]
├── MELHORIA :: USABILIDADE       :: 1   [CONFIG::MELHORIA::USABILIDADE::002]
└── MELHORIA :: GOVERNANCA        :: 1   [CONFIG::MELHORIA::GOVERNANCA::003]
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

Totais: **CONFLITO 2 · ERRO 1 · GAP 5 · MELHORIA 12 = 20 itens.**

## Tabela de Rastreabilidade

| Tag da Spec | Diretório OpenSpec | AGENTS.md | GitHub Issue # |
| --- | --- | --- | --- |
| `audit-config` | `openspec/changes/audit-config/` | `audit-config/AGENTS.md` | _pendente_¹ |
| `audit-backend-core` | `openspec/changes/audit-backend-core/` | `audit-backend-core/AGENTS.md` | _pendente_¹ |
| `audit-storage-exchange` | `openspec/changes/audit-storage-exchange/` | `audit-storage-exchange/AGENTS.md` | _pendente_¹ |
| `audit-frontend-js` | `openspec/changes/audit-frontend-js/` | `audit-frontend-js/AGENTS.md` | _pendente_¹ |
| `audit-android` | `openspec/changes/audit-android/` | `audit-android/AGENTS.md` | _pendente_¹ |
| `audit-build-ci` | `openspec/changes/audit-build-ci/` | `audit-build-ci/AGENTS.md` | _pendente_¹ |
| `audit-docs-gov` | `openspec/changes/audit-docs-gov/` | `audit-docs-gov/AGENTS.md` | _pendente_¹ |

¹ **Issues desabilitadas no repositório** (o token da sessão de auditoria não
tem escopo de admin para habilitar: HTTP 403). Após habilitar em
*Settings → General → Features → Issues*, rodar **uma vez**:

```bash
bash openspec/changes/create-issues.sh
```

O script cria as 7 issues (`spec:draft`, `audit`, `dom:*`), grava o número em
cada `proposal.md`/`AGENTS.md` e imprime esta tabela preenchida. Aí cada issue
está pronta para ser puxada individualmente em worktrees/branches futuros
(`cnmfs/<slug>`) pelo pipeline Event-Driven, conforme `docs/AUDIT_PROMPT.md`.

## Conformidade da execução (FASE 4)

- Diff contém apenas `openspec/**`, `docs/AUDIT_PROMPT.md` (commit anterior) e
  a entry de higiene `.gitignore` (`.gitnexus/`, `.claude/`).
- Nenhum arquivo de produção alterado; sem `.claude/`, `AGENTS.md` de raiz ou
  `.gitnexus/` no tree.
- Quality Gate do Fork CI **verde** nos commits da auditoria (run
  44648264656).
- Commits 100% docs/openspec: sem bump de `APP_VERSION` (exceção da FASE 0.6).
