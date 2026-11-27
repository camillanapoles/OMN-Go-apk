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

## Skills ativas

Globais:
- `karpathy-guidelines` — `multica-ai/andrej-karpathy-skills` @ `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`, `gitnexus-refactoring`, `gitnexus-guide`, `gitnexus-cli` — `abhigyanpatwari/GitNexus` @ `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Seleção por spec: nenhuma adicional.

As skills Kotlin/Compose de chrisbanes permanecem inativas por conflito com CLAUDE.md §1 regra 1.
