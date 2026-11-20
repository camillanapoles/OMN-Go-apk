# Spec — frontend-js (detalhamento 5W1H)

## [FRONTEND-JS::MELHORIA::CICLOMATICIDADE::001] `omn-go-api.js` concentrador de 918 linhas

- **What (O quê):** o arquivo central da app acumula chamadas ao backend de
  todas as features + os três mecanismos de lazy loading + o mirror de logs
  (`logLinePrints`, `applySyncLogLine`) + stubs do modo `file:`.
- **Why (Por quê):** cada feature nova adiciona fetches no mesmo arquivo,
  aumentando o custo de revisão e o risco de regressão transversal; a casa já
  resolveu isso para scripts de clique (lazy modules) — o próprio arquivo é o
  host dos mecanismos que permitiriam a divisão.
- **Where (Onde):** `backend/frontend/html/js/OMN-Go/omn-go-api.js` (918
  linhas); consumidores: todos os scripts de página; grafo: File node com
  maior fan-in do frontend.
- **When (Quando):** build/manutenção; manifesta a cada revisão de PR de
  frontend.
- **Who (Quem):** mantenedor; agentes que adicionam features.
- **How (Como):** em passos separados e medidos: (1) extrair o bloco do
  console mirror (`logLinePrints`/`applySyncLogLine`) mantendo os exports
  `window.*` e os nomes que `TestLogFilterPortAgreesWithTheRealJavaScript` e
  `TestEverySyncLineReachesTheOverlay` exigem; (2) extrair o cliente de DB
  (`omn-go-db.js` já existe como lazy — mover para lá o que é do db); (3) só
  depois avaliar mais cortes. Cada passo: cobertura ≥ 60% mantida,
  `lazy.test.js` verde, shell de `index.html` INALTERADO (nada de `<script>`
  novo).

## [FRONTEND-JS::MELHORIA::CODIGO::002] UI do console construída com `innerHTML`

- **What (O quê):** `omn-go-console.js:41,57,58,66` usa `innerHTML` para
  montar header/botões/contador do console (strings constantes hoje;
  `omn-go-bookmark.js:127,152` usa `innerHTML = ''` só para limpar).
- **Why (Por quê):** consistência com a regra da casa (`textContent` para
  valores — `TestBookmarkerWritesNoValueAsHTML`); padrão que não escala para
  dados dinâmicos e não tem rede de segurança fora do Bookmarker.
- **Where (Onde):** `backend/frontend/html/js/OMN-Go/omn-go-console.js`;
  referência de padrão: `Bookmarker.js`.
- **When (Quando):** interação — abrir/limpar o console.
- **Who (Quem):** mantenedor; usuários (sem mudança visível).
- **How (Como):** reescrever com `createElement`/`textContent`/`append`
  (alvo de Cobertura continua ≥ 60%); os ids/classes (`omn-console-*`) ficam
  idênticos. Critério: `console.test.js`/`dom-page.js` pressiona os controles
  e o DOM resultante é o mesmo (mesmos ids/classes).

## [FRONTEND-JS::GAP::CODIGO::003] Sem meta-teste anti-`innerHTML` para os demais scripts

- **What (O quê):** existe meta-teste anti-HTML para Bookmarker.js apenas;
  não há checagem de `innerHTML =` com valor dinâmico nos outros 14 scripts da
  app.
- **Why (Por quê):** `innerHTML` é o vetor XSS clássico do frontend sem build
  step e sem sanitizer; a casa prefere lint de fonte (§8: "Some tests scan the
  source and act as lint rules").
- **Where (Onde):** `backend/internal/repocheck/js_test.go` (lugar do lint JS)
  ou `source_rules_test.go`; alvo: `backend/frontend/html/js/OMN-Go/*.js`
  (exceto vendored: `highlight.min.js`, `katex.min.js`, `auto-render.min.js`).
- **When (Quando):** build — Quality Gate.
- **Who (Quem):** mantenedor; agentes que escrevem frontend.
- **How (Como):** meta-teste que permite só as formas seguras
  (`el.innerHTML = ''`) e bloqueia interpolação/atribuição de variável,
  com lista de exceções explícita (console migrado no 002 sai da lista).
  Critério: `x.innerHTML = valor` sintético num script da app → Gate vermelho.
