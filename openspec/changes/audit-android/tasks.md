# Tasks — audit-android

## P0 (comportamento entre versões)

- [ ] Setar explicitamente os settings de file access do WebView
  (INTEROPERABILIDADE::001).
  - **Verificação:** APK instalado em emulator API 23 e API 35: notas abrem,
    imagens carregam, sync overlay funciona (mesmo comportamento nas duas);
    `adb shell dumpsys`/log sem `file://` na navegação.
  - **Validação:** `pipelines_test.go` sem diff (Gradle/NDK intactos);
    `java_test.go` verde; APK Build do CI assina e empacota.

## P1 (governança executável)

- [ ] Meta-teste do teto de 800 linhas do MainActivity (GOVERNANCA::002).
  - **Verificação:** `go test ./backend/internal/repocheck/` no Gate conta as
    linhas e passa com 472; cenário sintético (arquivo temporário >800) falha
    com a mensagem correta.
  - **Validação:** nenhum outro meta-teste Java muda; `doc/TESTING.md`
    atualizado com o novo check.
