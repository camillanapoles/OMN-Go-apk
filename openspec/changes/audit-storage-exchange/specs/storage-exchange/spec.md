# Spec — storage-exchange (detalhamento 5W1H)

## [STORAGE-EXCHANGE::GAP::GOVERNANCA::001] Cópia Java da lista de extensões sem meta-teste

- **What (O quê):** `imageUploadExtensions` existe em Go
  (upload_handlers.go:23) e em Java (`android/app/src/main/java/net/basov/omngo/ShareIn.java`,
  apontado pelo próprio comentário do Go) sem nenhum teste que compare as duas
  cópias.
- **Why (Por quê):** a casa duplica por necessidade (o Java não lê o grafo Go)
  mas SEMPRE com um meta-teste que compara (fold table, log filter, local-
  pattern). Aqui a única proteção é um comentário "Keep the copies the same" —
  um `.svg`/`.webp` novo num lado só produz uma falha silenciosa de UX (share
  recusado ou upload aceito que o outro lado não aceita).
- **Where (Onde):** `backend/internal/app/upload_handlers.go:20-23`;
  `android/app/src/main/java/net/basov/omngo/ShareIn.java` (lista de exts do
  share de imagem); lugar do teste: `backend/internal/repocheck/` (lê Go e
  Java, como `java_test.go`).
- **When (Quando):** build — a cada Quality Gate.
- **Who (Quem):** `package app` e camada Android; mantenedor.
- **How (Como):** meta-teste `repocheck` que extrai a lista literal dos dois
  arquivos (regex sobre o source, padrão de `source_rules_test.go`) e falha se
  divergirem. Critério: adicionar `.bmp` a um lado só → Gate vermelho nomeando
  os dois arquivos.

## [STORAGE-EXCHANGE::MELHORIA::SEGURANCA::002] SVG executável sem sandbox

- **What (O quê):** upload aceita `.svg`; serving entrega `image/svg+xml`
  inline, sem `nosniff`/CSP/`Content-Disposition` — SVG é XML executável no
  mesmo origin.
- **Why (Por quê):** o browser do próprio admin abre `html/images/x.svg`
  direto (link numa nota ou history); scripts do SVG rodam no origin
  `127.0.0.1:<port>` e podem chamar as APIs admin do device (o bypass local
  considera a conexão admin). O upload é admin-only e o CSRF é barrado por
  `foreignRequest`, então o vetor exige um SVG malicioso enviado por... o
  próprio admin (ex.: imagem baixada de fonte externa) — risco de segundo
  salto, não de atacante remoto direto.
- **Where (Onde):** `backend/internal/app/upload_handlers.go:23` (aceita);
  `backend/internal/config/content_types.go:33` e `config.go:173`
  (`image/svg+xml`); `backend/internal/app/serving.go:116` (servido inline);
  `serving_test.go:27`.
- **When (Quando):** runtime/interação — abrir o SVG no browser do device.
- **Who (Quem):** usuário admin; `package app` (serving).
- **How (Como):** escolha mínima que não quebra a casa: para `.svg` sob
  `html/images/`, servir com `Content-Disposition: inline` + header
  `Content-Security-Policy: script-src 'none'` (o serving.go já tem o ponto
  único de Content-Type em :116; adicionar o gancho por extensão). Alternativa
  mais dura (com mantenedor): remover `.svg` da lista de upload. Critério:
  abrir SVG com `<script>` no browser não executa (teste de header em
  `serving_test.go`).

## [STORAGE-EXCHANGE::MELHORIA::CODIGO::003] Duplicação das duas vias de upload

- **What (O quê):** `saveUploadedFile` e `readUserFile` repetem o parse
  multipart (threshold 10 MB, comentário idêntico) e a checagem de limite
  (uma via `header.Size`, outra `len(data)`).
- **Why (Por quê):** qualquer mudança de política (novo limite, novo header)
  precisa de dois patches simétricos; já há assimetria sem motivo documentado.
- **Where (Onde):** `backend/internal/app/upload_handlers.go:34-87` e
  `:98-146`; testes dos dois lados em `upload_test.go`.
- **When (Quando):** build/manutenção.
- **Who (Quem):** mantenedor do `package app`.
- **How (Como):** extrair um helper interno de leitura limitada de parte
  (parse + limite) mantendo `userFileName` como o ÚNICO resolvedor de nome
  (regra 7) e `saveUploadedFile` para o caminho de imagem. Critério:
  `upload_test.go` passa sem edição (comportamento idêntico), incluindo os
  casos de traversal e limite.
