# Spec — android (detalhamento 5W1H)

## [ANDROID::MELHORIA::INTEROPERABILIDADE::001] File access do WebView depende do default da plataforma

- **What (O quê):** `WebViewSetup` seta apenas `setJavaScriptEnabled(true)` e
  `setDomStorageEnabled(true)`; `setAllowFileAccess` fica no default da
  plataforma — `true` até API 28, `false` a partir da API 29.
- **Why (Por quê):** o app serve tudo por HTTP local; file access no WebView
  não é necessário para o produto, e deixá-lo variável entre versões cria
  dois comportamentos do mesmo APK (um deles mais permissivo: um `file://`
  aberto por conteúdo de nota em API ≤ 28 lê arquivos do app que em API ≥ 29
  não lê). O fork publica em F-Droid para um parque de Androids amplo.
- **Where (Onde):** `android/app/src/main/java/net/basov/omngo/WebViewSetup.java:57-58`
  (settings existentes); `android/app/build.gradle` (`minSdk 23`);
  navegação de links é interceptada em `omn-go-nav.js`/`shouldOverrideUrlLoading`.
- **When (Quando):** runtime — criação do WebView, a cada start de activity.
- **Who (Quem):** usuários Android (todas as versões suportadas);
  `WebViewSetup` como classe dona do setup.
- **How (Como):** setar explicitamente `webSettings.setAllowFileAccess(false)`
  (e avaliar `setAllowFileAccessFromFileURLs(false)`/
  `setAllowContentAccess(false)` no mesmo bloco, com comentário de uma linha
  da razão). Verificação manual em emulator API 23 e API 35 (notas abrem,
  imagens carregam via http local). Critério: nenhum fluxo de nota/asset
  depende de `file://` (o guard `file:` de `omn-go-api.js` é só para páginas
  exportadas no desktop).

## [ANDROID::GAP::GOVERNANCA::002] Teto de 800 linhas do `MainActivity` sem meta-teste

- **What (O quê):** a regra "MainActivity stays below 800 lines" (CLAUDE.md
  §2) não é enforçada por teste; hoje 472 linhas, sem alerta de crescimento.
- **Why (Por quê):** o padrão da casa é regra com teste (`TestVersionIsTheSameInEachFile`,
  `TestNoBare64BitAtomics`...); sem o meta-teste, a classe volta a inchar
  silenciosamente até alguém reler a prosa — exatamente o que a regra existe
  para evitar (ADR 0022 divide MainActivity em classes simples).
- **Where (Onde):** `android/app/src/main/java/net/basov/omngo/MainActivity.java`
  (472 linhas); lugar do teste: `backend/internal/repocheck/java_test.go`
  (que já lê os fontes Java).
- **When (Quando):** build — Quality Gate.
- **Who (Quem):** mantenedor; qualquer PR Android futuro.
- **How (Como):** dentro de `java_test.go`, varrer o arquivo e falhar acima de
  800 linhas com mensagem que aponte para a regra/ADR 0022 (extração para
  classe simples). Critério: sinteticamente, o teste conta as linhas corretas;
  PR que adiciona 400 linhas ao MainActivity → Gate vermelho.
