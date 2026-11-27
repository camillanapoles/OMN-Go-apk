Spec OpenSpec: [`openspec/changes/audit-android/`](../../openspec/changes/audit-android/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[ANDROID::MELHORIA::INTEROPERABILIDADE::001]` | MELHORIA | `setAllowFileAccess` no default da plataforma (muda na API 29/30; minSdk 23) |
| `[ANDROID::GAP::GOVERNANCA::002]` | GAP | Teto de 800 linhas do `MainActivity` (472 hoje) sem meta-teste |

## Checklist macro (P0 → P1)

- [ ] **P0** Setar explicitamente file access/content access do WebView (false) — validar em emulator API 23 e 35
- [ ] **P1** Meta-teste no `repocheck/java_test.go` para o teto de 800 linhas

Restrições duras: sem AndroidX (constraint §1), sem tocar Gradle/`build_aar.sh`/recipe (pins de `pipelines_test.go`), `MainActivity` não muda.
Contrato local: `AGENTS.md` dentro do diretório da spec.
