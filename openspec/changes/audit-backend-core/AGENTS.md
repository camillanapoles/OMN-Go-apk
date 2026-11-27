# AGENTS.md — audit-backend-core (contrato local da change)

Issue: #32

## Contexto isolado

`package app` (backend/internal/app/): o servidor local do OMN-Go. O fluxo de
toda resposta é `connectionMiddleware` (foreignRequest → cache headers) →
rota (`a.route` em `registerRoutes`, server.go) → handler. O papel é decidido
por `hasRole` (autoridade única; local = admin; rede = cookie HMAC). Ler
antes: `CLAUDE.md` §1 regras 10-11, §3 (Routes/JSON/Logging), ADR 0001, 0002,
0004, 0016, 0017, 0018 e a spec desta change.

## Arquivos permitidos para edição

- `backend/internal/app/middleware.go`, `session.go`, `serving.go`,
  `server.go` (somente se um header exigir gancho no `route`)
- `backend/internal/app/middleware_test.go`, `session_test.go`,
  `serving_test.go`, `pages_test.go`
- `backend/internal/repocheck/` (1 meta-teste novo)
- `doc/API.md` (§1.4), `doc/decisions/` (ADR do throttle)

## Proibido editar

- `backend/backend.go` (facade), `backend/internal/app/routes.go`,
  `page_access.go` (tabela de papéis não muda)
- `backend/internal/config/**`, `backend/internal/gitsync/**`, `android/**`,
  `backend/frontend/**` (domínios das outras issues)
- `Dockerfile`, workflows

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles`
2. `npx -y gitnexus@1.6.12 detect-changes` (só arquivos do escopo)
3. PR → esperar Quality Gate do Fork CI (`gh run watch`) — o `go test` não
   roda no host.
4. Bump de versão nos 3 arquivos (toca código Go).

## Proibições do GitNexus

- `connectionMiddleware` está no caminho de todos os fluxos (527 flows no
  grafo): mudar sua assinatura exige revisar `pageCacheWriter` e o meta-teste
  novo em conjunto.
- Não criar segunda checagem de papel: `hasRole` e `systemPages` são únicos
  (§1 regra 7).
- Não adicionar dependência de rate limit (stdlib apenas).
- Nenhum `log.Printf` direto; usar `a.log(logx.Session)` / `logx.Server`.

## Skills ativas

Globais:
- `karpathy-guidelines` — `multica-ai/andrej-karpathy-skills` @ `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`, `gitnexus-refactoring`, `gitnexus-guide`, `gitnexus-cli` — `abhigyanpatwari/GitNexus` @ `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Seleção por spec: `security-review`.

Ative durante a revisão de login e headers; as regras do repositório têm precedência.
