# Design — audit-storage-exchange

## Stack envolvida

- Go 1.25/1.26 (`net/http` multipart, `os`/`path/filepath`), Java 21 da camada
  Android para o lado da cópia `ShareIn.java` (compilado só no CI/`java_test`).
- Sem dependência nova; headers HTTP via stdlib.

## Construção (build/test/lint)

- Gate: Fork CI (`go vet` + `go test`; `java_test.go` compila `android/test`
  quando o JDK existe na imagem).
- O meta-teste de 001 segue o padrão dos meta-testes de fonte do
  `repocheck` (ler arquivo, regex, falhar com mensagem que aponte os dois
  lados).
- Header CSP de 002 entra no ponto único de Content-Type (serving.go:116);
  `TestColorValuesLiveOnlyInThePalette` etc. não se aplicam; `serving_test.go`
  recebe o caso novo.
- Commits: `fix(app): ...`/`feat(app): ...` com bump nos 3 arquivos.

## Governança

- Merge: PR com Quality Gate + APK Build + GitOps Guard verdes; origem
  `cnmfs/*`/`arena/*` → `merge/sync-cnmfs`.
- Ordem: 001 (meta-teste primeiro — prende o terreno) → 003 (dedup) → 002
  (header).
- Restrições: `storage.RelInside` continua o único teste de contenção;
  `userFileName` o único resolvedor de nome; `storage.ReplaceFile` o único
  escritor atômico de nota; não mexer na tabela `Config.MimeTypes` de usuário
  (o 002 é header de resposta, não mudança de MIME).
- O item 002 tem decisão de produto (CSP vs remover .svg): marcar como
  `decision-required` na issue e resolver com o mantenedor antes do código.
