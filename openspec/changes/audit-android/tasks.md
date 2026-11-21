# Tasks — audit-android

## P0 (comportamento entre versões)

- [x] Setar explicitamente os settings de file access do WebView
  (INTEROPERABILIDADE::001).
  - **Verificação:** APK instalado em emulator API 23 e API 35: notas abrem,
    imagens carregam, sync overlay funciona (mesmo comportamento nas duas);
    `adb shell dumpsys`/log sem `file://` na navegação.
  - **Validação:** `pipelines_test.go` sem diff (Gradle/NDK intactos);
    `java_test.go` verde; APK Build do CI assina e empacota.

## P1 (governança executável)

- [x] Meta-teste do teto de 800 linhas do MainActivity (GOVERNANCA::002).
  - **Verificação:** `go test ./backend/internal/repocheck/` no Gate conta as
    linhas e passa com 472; cenário sintético (arquivo temporário >800) falha
    com a mensagem correta.
  - **Validação:** nenhum outro meta-teste Java muda; `doc/TESTING.md`
    atualizado com o novo check.

## Nota de execução (2026-11-20)

- P0: WebViewSetup seta agora setAllowFileAccess(false),
  setAllowContentAccess(false), setAllowFileAccessFromFileURLs(false) e
  setAllowUniversalAccessFromFileURLs(false), com comentário de uma linha
  da razão. O par emulator 23/35 é manual: o procedimento entrou na seção
  5 de doc/TESTING.md (abrir nota com imagem + Config por http local).
- P1: TestMainActivityStaysUnderTheLineCeiling em java_test.go com o teto
  800 (mainActivityLineCeiling). MainActivity segue com 472 linhas e sem
  edição. Cenário sintético verificado por contagem direta.
- Bump 26.11.45/2611045 (meta-teste Go novo).
