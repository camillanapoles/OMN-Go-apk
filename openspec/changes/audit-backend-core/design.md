# Design — audit-backend-core

## Stack envolvida

- Go 1.25/1.26, `net/http` stdlib (`ServeMux`, `httptest` nos testes); sem
  framework, sem middleware library, sem assertion lib (CLAUDE.md §8).
- Rate limit: implementação própria mínima (map + mutex + `atomic.Int64` —
  CLAUDE.md §1 regra 4 proíbe bare 64-bit atomics em 32-bit ABIs). **Sem
  dependência nova** (a casa mantém 3 requires).

## Construção (build/test/lint)

- Gate no Fork CI: `go vet ./backend/... && go test ./backend/...`
  (`docker buildx build --target test` lá; host de sessão não tem Go/Docker).
- Testes: `middleware_test.go`, `session_test.go`, `serving_test.go`,
  `server_test.go` (comportamento — prova o DO) e 1 meta-teste novo em
  `backend/internal/repocheck/` (cinto).
- Contratos: `doc/API.md` §1.4 fixa as respostas — 429 novo precisa de linha
  lá; `TestBaseline_RouteSet` e `TestAPIRouteTable` regenerados só se a rota
  mudar (não muda: throttle é interno ao handler/middleware).
- Comment share ≤ 20% (`TestCommentShare`) — comentários curtos, justificativa
  longa vai para ADR.
- Commits: `fix(core): ...` / `feat(core): ...` com bump nos 3 arquivos
  (CLAUDE.md §6 — este domínio toca `backend/`).

## Governança

- Merge: PR de `cnmfs/*` ou `arena/*` → `merge/sync-cnmfs` com os 3 checks
  obrigatórios (Quality Gate, APK Build, GitOps Guard) verdes.
- Ordem recomendada dos itens: 003 (trivial) → 002 (headers) → 004 (meta-teste
  junto) → 001 (throttle, maior).
- Restrições: `hasRole` continua a única checagem de papel (§1 regra 7); o
  bypass local não pode ser bloqueado pelo throttle; `/api/logs` segue
  admin-only; nenhuma rota nova fora de `registerRoutes`.
- Worktree futuro isolado por issue; nada de editar `android/`, `frontend/`,
  `config/` (domínios próprios, issues separadas).
