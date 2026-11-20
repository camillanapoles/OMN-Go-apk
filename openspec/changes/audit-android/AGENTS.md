# AGENTS.md — audit-android (contrato local da change)

Issue: #35

## Contexto isolado

Camada Android: `MainActivity` (ciclo de vida, teto 800 linhas), classes
simples ao lado (`WebViewSetup`, `ShareIn`, `ShareOut`, `IntentBridge`,
`Shortcuts`, `Fullscreen`, `ExportProvider`), classes puras testadas
(`OmnConfig`, `OmnText`, `LocalServer` em `android/test/`), service em
foreground (`ServerService`, `data_sync`, `START_STICKY`). Ler antes:
CLAUDE.md §1 regras 1, 3, 6, §2 (mapa android/), ADR 0022, 0027 e a spec
desta change.

## Skills ativas

- Globais (manifest `local/skills/manifest.yml`; instalar com
  `bash local/agent-setup/install.sh`): `gitnexus-exploring`,
  `gitnexus-debugging`, `gitnexus-impact-analysis`, `gitnexus-refactoring`,
  `gitnexus-guide`, `gitnexus-cli`, `karpathy-guidelines`.
- Pool (instalar com `bash local/agent-setup/install.sh <id>`):
  - `gradle-run` (chrisbanes/skills) — workflow genérico de build Gradle (sem Kotlin/Compose). Uso: antes do merge Android.
  - `run-project` (chrisbanes/skills) — execução e verificação do projeto (genérico). Uso: antes do merge Android.

## Arquivos permitidos para edição

- `android/app/src/main/java/net/basov/omngo/WebViewSetup.java`
- `backend/internal/repocheck/java_test.go` (meta-teste novo)
- `doc/TESTING.md` (procedimento de verificação manual)
- `fastlane/metadata/android/en-US/changelogs/<versionCode>.txt` (se release
  F-Droid acompanhar)

## Proibido editar

- `android/app/build.gradle` e qualquer Gradle (pin de pipelines)
- `android/build_aar.sh`, `metadata/net.basov.omngo.fdroid.yml`
- `MainActivity.java` (a regra é sobre o tamanho dele; nada a mudar aqui)
- `backend/**` (nenhuma mudança Go nesta change exceto o meta-teste do
  repocheck permitido acima)

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles`
2. `npx -y gitnexus@1.6.12 detect-changes`
3. `gh run watch` no Fork CI — **APK Build** é o check crítico desta change
   (javac/SDK só existem lá).
4. Bump de versão nos 3 arquivos.

## Proibições do GitNexus

- Camada Java não é indexada pelo grafo Go — o blast radius aqui é
  comportamental: qualquer setting novo do WebView exige o par emulator 23/35
  antes do merge.
- Proibido importar AndroidX (§1 regra 1); proibido adicionar dependência
  Gradle.
- Não mover método entre classes do pacote sem reler o ADR 0022.
