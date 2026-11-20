# Proposal — audit-android

> **Issue:** _pendente_
> **Domínio:** `android` — `android/app/src/main/java/net/basov/omngo/`,
> `android/test/`, Gradle.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

A camada Android é Java puro sem AndroidX (constraint fixa): `MainActivity`
com 472 linhas (dentro do teto de 800 da §2), componentes em classes simples
(`WebViewSetup`, `Fullscreen`, `ShareIn`, `ShareOut`, `IntentBridge`,
`Shortcuts`), três classes sem import Android (`OmnConfig`, `OmnText`,
`LocalServer`) cada uma com teste em `android/test/`. O AAR é gerado por
`android/build_aar.sh` (gomobile, pin `XMOBILE_VERSION`). O `ServerService`
roda em foreground com tipo `data_sync` e `START_STICKY` para sobreviver ao
`/api/restart`. O `WebViewSetup` configura apenas `setJavaScriptEnabled(true)`
e `setDomStorageEnabled(true)` (WebViewSetup.java:57-58). A camada é
espelehada pelos meta-testes `java_test.go` e `pipelines_test.go` (que compara
Go/NDK/build-tools/Gradle/x-mobile entre Dockerfile, recipe F-Droid e
`build_aar.sh`).

## Listas identificadas (tags topológicas)

### MELHORIA
- `[ANDROID::MELHORIA::INTEROPERABILIDADE::001]` — `setAllowFileAccess` (e
  `setAllowFileAccessFromFileURLs`-related) não é setado em `WebViewSetup`:
  o comportamento cai no **default da plataforma, que mudou no Android 10/11
  (API 29/30)** — em APIs ≤ 28 file access no WebView é `true` por default e
  nas novas é `false`. Com `minSdk 23`, o mesmo APK se comporta de forma
  diferente entre versões de Android.

### GAP
- `[ANDROID::GAP::GOVERNANCA::002]` — O teto de **800 linhas do MainActivity**
  (CLAUDE.md §2: "stays below 800 lines") não tem meta-teste: `java_test.go`
  compila e roda os testes de unidade mas não varre o tamanho do arquivo; a
  regra vive só em prosa.

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `WebViewSetup.java` (setup do WebView) | toda a UI do app Android | MÉDIO | `android/test/` não cobre WebViewSetup (classes sem Android são as testadas) — validação será manual + instrumentation docs |
| `MainActivity.java` (472 linhas) | ciclo de vida todo | MÉDIO | meta-teste novo (a criar) |
| `pipelines_test.go` | lê `build.gradle`/`build_aar.sh`/Dockerfile | ALTO se Gradle mudar | `TestAndroidBuildIsTheSameOnEachHost`, `pipelines_test.go` |

Nota: nenhum achado exige mudar Gradle — os dois itens tocam apenas Java de
classe e meta-teste Go.
