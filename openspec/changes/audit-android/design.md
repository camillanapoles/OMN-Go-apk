# Design — audit-android

## Stack envolvida

- Java plain (sem AndroidX/AppCompat — constraint fixa §1 regra 1), WebView
  (`android.webkit`), minSdk 23, target/compile no AGP 8.2.2, build-tools
  34.0.0 (pin — pipelines_test compara).
- Testes Java: `android/test/` FORA do Gradle de propósito, compilados por
  `repocheck/java_test.go` com `javac`+`java` no CI (classes sem Android).
- AAR via `android/build_aar.sh` (gomobile) — **não tocar**.

## Construção (build/test/lint)

- Nenhuma das mudanças toca Gradle, recipe F-Droid ou `build_aar.sh` —
  `pipelines_test.go` e `TestAndroidBuildIsTheSameOnEachHost` não devem ter
  diff (validação explícita na task).
- O item 002 estende `repocheck/java_test.go` (Go lendo Java — padrão já
  existente).
- O item 001 é mudança de comportamento Android sem teste unitário possível
  (classe com import Android): documentar o procedimento de verificação
  manual (emulator 23/35) no PR e no `doc/TESTING.md`.
- Commits: `fix(android): ...` com bump nos 3 arquivos.

## Governança

- Merge: 3 checks obrigatórios (Quality Gate, **APK Build** — relevante aqui,
  GitOps Guard) verdes.
- Restrições: proibido introduzir AndroidX/AppCompat; proibido mover classes
  de arquivo (a divisão de classes é decisão ADR 0022); `MainActivity` segue
  dono do ciclo de vida; nenhuma permissão nova no manifest.
- O item 001 deve listar explicitamente no PR os três settings escolhidos
  (`allowFileAccess`/`allowContentAccess`/file-URLs) com a razão de cada um —
  é decisão de segurança reversível só com evidência de que nenhum fluxo usa
  `file://`.
