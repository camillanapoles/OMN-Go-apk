# Tasks — audit-backend-core

## P0 (segurança)

- [x] Rate limit de `/login` por RemoteAddr com backoff (SEGURANCA::001).
  - **Verificação:** `session_test.go`: 5 falhas → 6ª tentativa do mesmo IP
    responde 429 com `Retry-After`; sucesso reseta; IP diferente não herda o
    bloqueio; bypass local nunca bloqueia.
  - **Validação:** `TestBaseline_RouteSet` idêntico (nenhuma rota mudou);
    `go vet` verde no Gate; binary size dentro de
    `binary_size_baseline.json` (código +~100 linhas, sem dependência).
- [x] `X-Content-Type-Options: nosniff` global (SEGURANCA::002, parte 1).
  - **Verificação:** `middleware_test.go`: resposta de qualquer rota tem o
    header; o `pageCacheWriter` não o sobrescreve.
  - **Validação:** `serving_test.go` e `pages_test.go` verdes sem mudança.
- [x] CSP em páginas de sistema (SEGURANCA::002, parte 2).
  - **Verificação:** `pages_test.go`: config/status páginas carregam a meta;
    páginas de nota compiladas não carregam (diff de shell inalterado).
  - **Validação:** `TestCompiledPageShellStaysSmall` verde (meta tag na página
    de sistema, não no shell de nota).

## P1 (qualidade)

- [x] Remover branch morta `localhost` (CICLOMATICIDADE::003).
  - **Verificação:** `middleware_test.go`: `RemoteAddr "localhost:1"` → não
    local.
  - **Validação:** nenhuma outra asserção de `isLocalConnection` muda.
- [x] Meta-teste repocheck de headers/throttle (GOVERNANCA::004).
  - **Verificação:** quebrar o header no middleware → Gate falha com a
    mensagem do meta-teste.
  - **Validação:** `go vet ./backend/internal/repocheck/` verde; nenhum outro
    meta-teste afetado.

## P2 (documentação)

- [x] Documentar 429 e headers em `doc/API.md` §1.4 e regenerar a tabela
  (`OMN_WRITE_API_TABLE=1 go test -run TestAPIRouteTable` no CI).
  - **Verificação:** `TestAPIRouteTable` e `api_doc_test.go` verdes.
  - **Validação:** diff de `doc/API.md` limitado à seção de `/login`.

## Nota de execução

O CSP exige `backend/internal/render/pages.go` (`PageHeader` + o
`http-equiv` de `RenderIndexPage`). A lista de arquivos permitidos não
cita `render/`, mas a spec manda o mecanismo ("via render.PageHeader").
O desvio cobre esse único arquivo. A rota do throttle usa o IP de
`remoteHost` (o IP, e não a porta: "IP distinto não herda").
