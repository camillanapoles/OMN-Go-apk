# Design — audit-config

## Stack envolvida

- Go 1.25/1.26 (`net.basov.omngo`), sem CGO; pacotes `backend/internal/config`
  (layer própria segundo `importLayers`) e `backend/internal/app` (handlers).
- Nada de dependência nova: a correção usa stdlib (`crypto/subtle` já em uso).

## Construção (build/test/lint)

- Gate: `go vet ./backend/... && go test ./backend/...` — **roda no Quality
  Gate do Fork CI** (host de sessão não tem Go).
- Testes de comportamento em `session_test.go` (package app, `newTestApp`)
  e `config_test.go` (package config). Meta-teste de fonte (literal proibida)
  em `backend/internal/repocheck/` — lugar certo por CLAUDE.md §8.
- Contratos de interface que não podem quebrar:
  - `GET /api/config` / `POST /api/config` — formas de resposta fixadas em
    `doc/API.md` §1.4 e `TestBaseline_RouteSet`.
  - `configFieldSent` (config_handlers.go) — pedido que omite campo não muda
    campo; a flag de "senha default" não pode ser escrita por POST.
  - `PageValues` placeholders → nomes em UPPER_SNAKE no `config_page.html`
    (`TestEachTableValueHasAPlaceOnTheConfigPage`).
- Formato de commit: `fix(config): Sentence. vYY.MM.S` com bump nos 3 arquivos
  (este domínio toca `backend/` — **com bump**).

## Governança

- Critérios de aceite para merge: Quality Gate + APK Build + GitOps Guard
  verdes; PR de branch `cnmfs/config-default-password` (ou `arena/`) contra
  `merge/sync-cnmfs`; conversas resolvidas.
- Isolamento: uma issue = uma branch = um worktree futuro; nada de editar
  arquivos fora do escopo do `AGENTS.md` desta change.
- Restrições: não criar segunda autoridade de login (`hasRole` é única,
  CLAUDE.md §1 regra 7); não adicionar campo exportado em `package app`; não
  mudar o formato da linha `APP_VERSION` (CI lê com grep/awk).
- ADR: se o mantenedor escolher "senha gerada no primeiro boot", registrar
  decisão em `doc/decisions/00XX-...md` antes do código (formato do
  `doc/decisions/README.md`; `TestDecisions*` do repocheck valida).
