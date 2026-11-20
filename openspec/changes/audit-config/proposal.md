# Proposal — audit-config

> **Issue:** #31
> **Domínio:** `config` (`backend/internal/config/` + os pontos de consumo em `backend/internal/app/`)
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base da auditoria:** `02e8cd8`
> **Nota de versão:** commit 100% documentação/openspec — sem bump de `APP_VERSION` (exceção registrada na FASE 0 de `docs/AUDIT_PROMPT.md`).

## Cenário atual

`config.Store` (`backend/internal/config/store.go`) é a única autoridade sobre
`config.Config`: leitura com `Get()` sob `RLock`, escrita com
`Update(func(c *Config))` sob `Lock`, reparo de valores por `NormalizeXxx`
(fields.go) e ocultação de segredos na página via `Secret: true`
(page_values.go:23). O GitNexus indexa o domínio com blast radius **HIGH**:
`Function:backend/internal/config/config.go:Load` tem 4 nós a montante —
todo o boot (`initStorage` → `loadConfig` → handlers) depende dele. O pacote
tem testes próprios (`config_test.go`, `fields_test.go`, `page_values_test.go`,
`user_files_test.go`).

## Listas identificadas (tags topológicas)

### CONFLITO
- `[CONFIG::CONFLITO::SEGURANCA::001]` — Senha de admin default hard-coded no
  fresh install. Evidência: `backend/internal/config/config.go:310`
  (`AdminPassword: "admin_secret_changeme"` dentro de `Load`); o login a aceita
  (`backend/internal/app/session.go:183`, `passwordMatches` só recusa quando a
  senha configurada é vazia) e `handleLogin` não tem nenhuma verificação de
  "senha ainda é a default".

### MELHORIA
- `[CONFIG::MELHORIA::USABILIDADE::002]` — Nenhum sinal de first-run sobre a
  senha default. A página Config oculta o valor (`fields.go:96` `Secret: true`,
  correto por ADR 0005), mas nada avisa a pessoa que a credencial instalada é
  pública (constante do código).
- `[CONFIG::MELHORIA::GOVERNANCA::003]` — Autor de git default `"Anonymous"`
  (`config.go:311`) flui para o autor dos commits de sincronização
  (`backend/internal/gitsync/commit.go:167`, `svc.configAuthor()`), gerando
  histórico de sync sem identidade real em todos os devices que não ajustarem
  `author`.

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius (GitNexus) | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `config.Load` (config.go:299) | 4 nós a montante, **HIGH** | ALTO | `config_test.go` (`TestLoad*`), boot coberto por `storage_init_test.go` |
| `passwordMatches` / `handleLogin` (session.go:157-197) | folha (chamada via closure `post(a.handleLogin)`, não linkada no grafo) | MÉDIO | `session_test.go`, `TestBaseline_RouteSet` |
| `configFields` linha `admin_password` (fields.go:93-98) | tabela com meta-testes (`TestEveryConfigFieldIsInTheTable`) | MÉDIO | `fields_test.go`, `config_fields_test.go` |
| `gitsync commit.go configAuthor` | pacote layer 4, hooks em `connectGroups` | MÉDIO | `repo_test.go`, `sync_test.go` |

Regra da casa aplicável: **uma autoridade por decisão** (CLAUDE.md §1 regra 7) —
a correção de 001 deve viver em `handleLogin`/`passwordMatches` ou na
normalização de `config`, nunca em ambos sem meta-teste que compare.
