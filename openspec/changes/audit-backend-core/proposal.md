# Proposal — audit-backend-core

> **Issue:** #32
> **Domínio:** `backend-core` — `package app` (`backend/internal/app/`): server,
> rotas, middleware, sessão de rede, serving, request guard.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

O `package app` é o servidor HTTP local: `http.ServeMux` com tabela de rotas em
`registerRoutes` (server.go:280-341), middleware de conexão
(`connectionMiddleware` → `foreignRequest` → cache headers), autoridade única
de papel (`hasRole`, middleware.go) com bypass local, sessão assinada HMAC-SHA256
(session.go) e serving de arquivos com Content-Type resolvido por
`config.ResolveContentType` (serving.go:116). A tabela de rotas é presa por
golden set (`TestBaseline_RouteSet`) e por `TestAPIRouteTable`; o guard de
origem por `request_guard_test.go`. A higiene geral é alta: sem `panic(` em
produção, sem TODO/FIXME, `log.Printf` só nas 2 exceções documentadas, sem
bare 64-bit atomics (verificado por grep na FASE 1). Os achados abaixo são de
endurecimento e governança, não de quebra.

## Listas identificadas (tags topológicas)

### MELHORIA
- `[BACKEND-CORE::MELHORIA::SEGURANCA::001]` — `POST /login` sem rate limit nem
  backoff. Evidência: `session.go:157-197` (`handleLogin` não tem contador),
  `middleware.go` (não há throttle em nenhuma camada). Com `share_lan: true`,
  um host da rede pode tentar senhas sem custo.
- `[BACKEND-CORE::MELHORIA::SEGURANCA::002]` — Headers de hardening ausentes:
  nenhum `X-Content-Type-Options: nosniff` no tree (grep sem hits), nenhum CSP.
  O `connectionMiddleware` (middleware.go:30-32) já é "o único lugar que
  controla o cache do cliente" — lugar natural para o header global.
- `[BACKEND-CORE::MELHORIA::CICLOMATICIDADE::003]` — Branch morta em
  `isLocalConnection` (middleware.go:14): `host == "localhost"` nunca é
  verdadeiro — `net.SplitHostPort(r.RemoteAddr)` devolve um IP, nunca a
  string `localhost`.

### GAP
- `[BACKEND-CORE::GAP::GOVERNANCA::004]` — O `repocheck` não tem meta-teste que
  prenda os headers de resposta HTTP da casa (nosniff, e o futuro CSP) nem o
  comportamento de throttle: invariantes de rede não são lint-ados como as
  demais regras transversais (compare `TestNoDirectLogPrintf`,
  `TestNoBare64BitAtomics`).

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius (GitNexus) | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `connectionMiddleware` (middleware.go:22) | wrapper de todas as respostas (fluxos 527 do grafo passam por ele) | ALTO | `middleware_test.go`, `server_test.go` |
| `handleLogin` (session.go:157) | folha no grafo (chamada via closure `post(a.handleLogin)`) — impacto local | MÉDIO | `session_test.go`, `TestBaseline_RouteSet` |
| `isLocalConnection` (middleware.go:10) | folha no grafo; usada por `hasRole` (autoridade única) | MÉDIO | `middleware_test.go`, `session_test.go` |
| `serving.go` (headers de arquivo) | caminho de cada asset/página | MÉDIO | `serving_test.go` (27+ asserções de Content-Type) |

Limitação registrada: `impact` devolve `impactedCount: null` para os handlers
porque o grafo não liga closures de `registerRoutes`; o blast radius real é o
conjunto de rotas da tabela (server.go:280-341), todo preso por baseline.
