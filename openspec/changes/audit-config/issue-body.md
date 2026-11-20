Spec OpenSpec: [`openspec/changes/audit-config/`](../../openspec/changes/audit-config/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[CONFIG::CONFLITO::SEGURANCA::001]` | CONFLITO | Senha admin default hard-coded no fresh install (`config.go:310` + `session.go:183`) |
| `[CONFIG::MELHORIA::USABILIDADE::002]` | MELHORIA | Nenhum aviso de first-run sobre a senha default |
| `[CONFIG::MELHORIA::GOVERNANCA::003]` | MELHORIA | Autor de sync default `"Anonymous"` (commit.go via config) |

## Checklist macro (P0 → P2)

- [ ] **P0** Recusar login de rede com a senha default (403) — bypass local intacto
- [ ] **P1** Aviso de senha default na Config page/Status
- [ ] **P1** Onboarding do `author` de sync
- [ ] **P2** Meta-teste repocheck anti-literal da senha default

Blast radius chave: `config.Load` = **HIGH** (4 nós) — meta-testes `config_test.go`/`session_test.go` guardam.
Contrato local: `AGENTS.md` dentro do diretório da spec.
