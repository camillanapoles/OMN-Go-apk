# AGENTS.md — audit-config (contrato local da change)

Issue: #31

## Contexto isolado

Domínio `config`: `backend/internal/config/` (Config, Store, fields, page
values, hostname, user files) e seus pontos de consumo em
`backend/internal/app/` (`config_app.go`, `config_handlers.go`,
`config_page.go`). Autoridades da casa: `config.Save` é o único escritor de
`config.json`; `configFields` é a única tabela de campos; `Secret: true`
oculta segredos da página (ADR 0005). Ler antes: `CLAUDE.md` §1 (regra 7),
§3 ("Configuration"), ADR 0001/0005/0014, e a spec desta change
(`specs/config/spec.md`).

## Arquivos permitidos para edição

- `backend/internal/config/config.go` (somente o bloco do fresh install e
  normalize relacionado)
- `backend/internal/app/session.go` (handleLogin/passwordMatches)
- `backend/internal/config/fields.go`, `page_values.go` (linha/valor derivado)
- `backend/frontend/templates/config_page.html` (aviso)
- Testes: `session_test.go`, `config_test.go`, `config_fields_test.go`,
  `page_values_test.go`, `backend/internal/repocheck/` (1 meta-teste novo)
- Docs: `doc/API.md` (seção do campo novo), `doc/decisions/` (1 ADR se
  necessário)

## Proibido editar

- `backend/backend.go`, `backend/internal/app/server.go` (facade/rotas —
  nenhuma rota nova)
- `backend/internal/gitsync/` (o item 003 só toca config; commit.go é leitura)
- `Dockerfile`, `.github/workflows/**`, `android/**`

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles` (na raiz)
2. `npx -y gitnexus@1.6.12 detect-changes` (confere que só os arquivos do
   escopo mudaram)
3. Quality Gate do Fork CI no PR: `gh run watch <run-id>` — `go vet`+`go test`
   não rodam no host de sessão.
4. Bump de versão nos 3 arquivos (CLAUDE.md §6) — este domínio toca código.

## Proibições do GitNexus

- `config.Load` tem blast radius HIGH (4 nós): qualquer assinatura nova de
  `Load` exige atualizar os call sites e o meta-teste em conjunto.
- Não duplicar `passwordMatches` (autoridade única de comparação de senha);
  a verificação de "senha default" vive lá.
- Nenhum `log.Printf` direto (usar `a.log(logx.Session)`); nenhum bare 64-bit
  atomic; nenhum `log.Printf` além das 2 exceções documentadas.

## Skills ativas

Globais:
- `karpathy-guidelines` — `multica-ai/andrej-karpathy-skills` @ `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`, `gitnexus-refactoring`, `gitnexus-guide`, `gitnexus-cli` — `abhigyanpatwari/GitNexus` @ `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Seleção por spec: `security-review`.

Ative durante a revisão de autenticação e segredos; as regras do repositório têm precedência.
