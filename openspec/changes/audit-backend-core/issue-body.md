Spec OpenSpec: [`openspec/changes/audit-backend-core/`](../../openspec/changes/audit-backend-core/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[BACKEND-CORE::MELHORIA::SEGURANCA::001]` | MELHORIA | `/login` sem rate limit/backoff |
| `[BACKEND-CORE::MELHORIA::SEGURANCA::002]` | MELHORIA | Sem `nosniff`/CSP (serving.go:116, middleware.go:30) |
| `[BACKEND-CORE::MELHORIA::CICLOMATICIDADE::003]` | MELHORIA | Branch morta `localhost` em `isLocalConnection` |
| `[BACKEND-CORE::GAP::GOVERNANCA::004]` | GAP | Sem meta-teste de headers/HTTP hardening no repocheck |

## Checklist macro (P0 → P2)

- [ ] **P0** Rate limit de `/login` por RemoteAddr com backoff (429 + `Retry-After`)
- [ ] **P0** `X-Content-Type-Options: nosniff` global no `connectionMiddleware`
- [ ] **P0** CSP nas páginas de sistema (não nas notas)
- [ ] **P1** Remover branch morta `localhost`
- [ ] **P1** Meta-teste de headers/throttle
- [ ] **P2** Documentar 429/headers em `doc/API.md` + `TestAPIRouteTable`

Higiene confirmada na auditoria: sem `panic`, sem TODO, `log.Printf` só nas 2 exceções documentadas, sem bare atomics.
Contrato local: `AGENTS.md` dentro do diretório da spec.
