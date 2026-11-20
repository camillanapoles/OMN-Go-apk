# Spec — config (detalhamento 5W1H)

## [CONFIG::CONFLITO::SEGURANCA::001] Senha de admin default hard-coded no fresh install

- **What (O quê):** o fresh install grava `admin_password: "admin_secret_changeme"`
  em `config.json` (`config.go:310`) e o login de rede aceita essa constante
  pública (`session.go:183` — `passwordMatches` só recusa senha vazia).
- **Why (Por quê):** com `share_lan: true`, qualquer dispositivo da rede loga
  como admin com uma credencial que está publicada num fork público — lê/edita
  notas, roda `/api/sql`, força pull/push. A tensão: "instalar e funcionar"
  (UX) × "nenhuma credencial conhecida" (segurança). O ADR 0002 já reconhece
  que o perigo cresce com LAN ligado.
- **Where (Onde):** `backend/internal/config/config.go:310` (constante);
  `backend/internal/app/session.go:183` (`passwordMatches`), `:186-190`
  (log de recusa só quando vazia). Blast radius: `impact
  Function:backend/internal/config/config.go:Load` → 4 nós, risco HIGH.
- **When (Quando):** runtime — primeiro boot sem `config.json`; manifesta a
  cada `POST /login` de um host não-local enquanto a senha não for trocada.
- **Who (Quem):** `package config` (fonte do default) e `package app`
  (autoridade de login `hasRole`/`handleLogin`); persona: usuário Android com
  LAN sharing ligado.
- **How (Como):** no `handleLogin` (autoridade única de login), recusar login
  de rede enquanto `cfg.AdminPassword == "admin_secret_changeme"` (resposta
  403 com texto explícito) + log `Errf`; manter o acesso local (bypass
  `isLocalConnection`) funcionando para a pessoa trocar a senha na Config page.
  Adicionar meta-teste em `backend/internal/repocheck/` que proíba a literal
  fora de `config.go` e o teste de comportamento em `session_test.go`.
  Alternativa (com mantenedor): gerar senha aleatória no primeiro boot e exibir
  uma vez. Critério verificável (M==N): `POST /login` com a senha default a
  partir de RemoteAddr não-local responde 403; com senha trocada, 200.

## [CONFIG::MELHORIA::USABILIDADE::002] Nenhum sinal de first-run sobre a senha default

- **What (O quê):** a Config page oculta o valor da senha (`Secret: true`,
  `fields.go:96`) — correto — mas nada na UI ou no Status indica que a
  credencial vigente é a constante pública.
- **Why (Por quê):** a pessoa que liga `share_lan` não tem como saber que
  precisa trocar a senha antes; o risco de 001 é invisível.
- **Where (Onde):** `backend/internal/config/page_values.go` (linhas de Status);
  `backend/frontend/templates/config_page.html` (seção Server);
  `config_fields_test.go` / `page_values_test.go`.
- **When (Quando):** interação — primeiro acesso à Config page / Status page.
- **Who (Quem):** usuário do device (admin local); mantenedor do fork.
- **How (Como):** acrescentar às `PageValues`/`StatusValue` um boolean derivado
  (ex.: `ADMIN_PASSWORD_DEFAULT_CHECKED`) calculado comparando com a constante;
  quando true, a Config page mostra aviso fixo (bloco `warning` do
  `omnDialog`/CSS de alerta) e o Status marca o valor. Critério: com a senha
  default, o placeholder aparece com `true` e some ao salvar outra senha.

## [CONFIG::MELHORIA::GOVERNANCA::003] Autor de sync default "Anonymous"

- **What (O quê):** `config.go:311` define `Author: "Anonymous"` no fresh
  install; `gitsync/commit.go:167` usa `svc.configAuthor()` como autor dos
  commits de sincronização.
- **Why (Por quê):** o histórico do repositório de notas fica sem identidade
  real, dificultando blame/resolução de conflito entre devices; viola o
  espírito de rastreabilidade da casa (build carrega commit, ADR 0028).
- **Where (Onde):** `backend/internal/config/config.go:311`;
  `backend/internal/gitsync/commit.go:167` e a função `configAuthor`.
- **When (Quando):** runtime — cada commit de sync.
- **Who (Quem):** `package gitsync` (consumidor) e `package config` (default);
  persona: usuário com Git Sync habilitado.
- **How (Como):** aproveitar a onboarding de 002: quando `author` ainda é o
  default, o aviso de first-run pede também o nome do device/autor (um campo
  só). Opcional: `NormalizeXxx` que derive de `config.Hostname` quando
  `"Anonymous"`. Critério: commit criado após definir `author` carrega o nome
  no `Author` (teste em `repo_test.go`).
