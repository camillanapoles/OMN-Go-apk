# Spec — backend-core (detalhamento 5W1H)

## [BACKEND-CORE::MELHORIA::SEGURANCA::001] `/api/login` sem rate limit

- **What (O quê):** `handleLogin` (session.go:157-197) aceita tentativas de
  senha ilimitadas por origem; não há throttle no middleware nem contador de
  falhas.
- **Why (Por quê):** com LAN sharing ligado, um atacante na rede testa senhas
  sem custo contra a credencial admin; a comparação é constant-time, mas o
  número de tentativas não é limitado. A porta de login é a única entrada de
  credencial do produto.
- **Where (Onde):** `backend/internal/app/session.go:157-197`;
  `backend/internal/app/middleware.go` (camada de conexão); rota
  `"/login"` em server.go:304. Grafo: folha (closure) — blast radius real = a
  rota `/login` + a página de login.
- **When (Quando):** runtime — a cada POST `/login` de um RemoteAddr
  não-local.
- **Who (Quem):** `package app` (middleware/sessão); persona: usuário com
  `share_lan` ligado.
- **How (Como):** contador de falhas por RemoteAddr com janela e backoff
  (ex.: `map[string]*failState` com mutex + `atomic.Int64` para o total;
  resposta 429 com `Retry-After` após N falhas; reset no sucesso). Registrar
  a exceção do bypass local (o device dono nunca é bloqueado). Critério
  (M==N): no teste, 6 falhas seguidas do mesmo RemoteAddr → a 6ª responde 429;
  conexão local nunca bloqueia; sucesso zera o contador.

## [BACKEND-CORE::MELHORIA::SEGURANCA::002] Headers de hardening ausentes

- **What (O quê):** nenhuma resposta carrega `X-Content-Type-Options: nosniff`;
  não existe Content-Security-Policy para páginas nem para assets servidos.
- **Why (Por quê):** o servidor entrega HTML gerado de notas (com
  `html.WithUnsafe()` deliberado) e arquivos de usuário; `nosniff` custa zero
  e impede sniffing de tipo; um CSP restritivo para as páginas de sistema
  (não para notas) reduz o efeito de qualquer escape faltante futuro.
- **Where (Onde):** `backend/internal/app/middleware.go:30-32`
  (`connectionMiddleware` — lugar único dos headers globais, por ADR 0004) e
  `backend/internal/app/serving.go:116,138-140` (Content-Type de arquivos).
- **When (Quando):** runtime — cada resposta.
- **Who (Quem):** `package app`; persona: usuário do WebView/desktop.
- **How (Como):** `nosniff` no `connectionMiddleware` (uma linha, cobre todas
  as respostas); CSP apenas nas páginas de sistema via `render.PageHeader`
  (meta tag) para não interferir com notas que rodam script. Critério: teste
  em `middleware_test.go` asser `nosniff` em qualquer rota; páginas de sistema
  carregam a meta CSP; páginas de nota não mudam.

## [BACKEND-CORE::MELHORIA::CICLOMATICIDADE::003] Branch morta em `isLocalConnection`

- **What (O quê):** middleware.go:14 compara `host == "localhost"`, mas
  `RemoteAddr` de conexão TCP é sempre `IP:porta` (`net.SplitHostPort` já
  separou), nunca a string `localhost`.
- **Why (Por quê):** código morto sugere que um RemoteAddr textual existe,
  o que pode induzir um mantenedor a aceitar `Host` (header controlado pelo
  cliente) como sinal de localidade no futuro — confusão de segurança.
- **Where (Onde):** `backend/internal/app/middleware.go:10-15`; usada por
  `hasRole` (middleware.go, autoridade única de papel) e testada em
  `middleware_test.go`.
- **When (Quando):** build/review — leitura do código.
- **Who (Quem):** mantenedores e agentes que estendem a checagem local.
- **How (Como):** remover a comparação `localhost` (deixar `127.0.0.1`/`::1`)
  e fixar com comentário de uma linha a razão (RemoteAddr é IP). Critério:
  `middleware_test.go` ganha caso com `RemoteAddr: "localhost:8080"` → não
  local (cai no cookie).

## [BACKEND-CORE::GAP::GOVERNANCA::004] Sem meta-teste de headers/HTTP hardening

- **What (O quê):** o `repocheck` (lint de fonte da casa) não tem teste que
  prenda os headers globais da resposta nem o throttle; hoje essas invariantes
  vivem só em testes de comportamento dispersos.
- **Why (Por quê):** a casa lint-a regras transversais (`TestNoDirectLogPrintf`
  etc.); headers de segurança e rate limit são transversais por definição —
  sem meta-teste, um refactor silenciosamente apaga o header.
- **Where (Onde):** `backend/internal/repocheck/` (sem arquivo de headers);
  alvo do lint: `backend/internal/app/middleware.go`, `serving.go`.
- **When (Quando):** build — a cada Quality Gate.
- **Who (Quem):** mantenedor do fork; agentes de trabalho futuros.
- **How (Como):** `repocheck` novo (ex.: `http_headers_test.go`) que exige a
  chamada `w.Header().Set("X-Content-Type-Options", ...)` no middleware e que
  o throtller exista (presença do tipo/constante). Preferir teste de
  comportamento no server (prova o DO, CLAUDE.md §8) + meta-teste como cinto.
  Critério: remover a linha do header quebra o Gate com mensagem clara.
