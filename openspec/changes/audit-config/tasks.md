# Tasks — audit-config

## P0 (segurança/credencial)

- [x] Recusar login de rede com a senha default (CONFLITO::SEGURANCA::001).
  - **Verificação:** `session_test.go`: `POST /login` com RemoteAddr
    `192.0.2.10:5555` e senha `"admin_secret_changeme"` → 403; com senha
    trocada → 200 + cookies; conexão local → 200 (bypass intacto).
  - **Validação:** `TestBaseline_RouteSet` inalterado; blast radius
    (`config.Load`, HIGH) coberto por `config_test.go` verde no Gate; nenhum
    golden de `baseline_test.go` muda (nenhuma rota nova).

## P1 (sinalização e histórico)

- [x] Aviso de senha default na Config page e Status (USABILIDADE::002).
  - **Verificação:** `config_fields_test.go`/`page_values_test.go`: o valor
    derivado é `true` com a senha default e `false` após troca; placeholder
    existe no template (`TestEachTableValueHasAPlaceOnTheConfigPage`).
  - **Validação:** `TestEveryConfigFieldIsInTheTable` verde; nenhuma mudança em
    `/api/config` JSON além do campo novo documentado em `doc/API.md`
    (`TestAPIRouteTable` com `OMN_WRITE_API_TABLE=1` no CI).
- [x] Onboarding do `author` de sync (GOVERNANCA::003).
  - **Verificação:** `repo_test.go`: commit de sync após definir `author`
    carrega o nome; com `"Anonymous"` o aviso aparece.
  - **Validação:** `TestImportLayers`/`TestGroupsUseOnlyLowerLayers` verdes
    (nenhum import novo entre grupos).

## P2 (regressão estrutural)

- [x] Meta-teste repocheck que proíbe a literal da senha default fora de
  `config.go`.
  - **Verificação:** `go test ./backend/internal/repocheck/` no Gate falha se
    a literal aparecer em outro arquivo.
  - **Validação:** `TestCommentShare` e `TestNoVersionNumberInComments`
    inalterados (comentário novo curto, justificativa em ADR).
