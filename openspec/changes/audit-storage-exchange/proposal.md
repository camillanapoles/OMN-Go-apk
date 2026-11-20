# Proposal — audit-storage-exchange

> **Issue:** _pendente_
> **Domínio:** `storage-exchange` — `backend/internal/storage/`,
> `backend/internal/exchange/` e os handlers de upload em
> `backend/internal/app/upload_handlers.go`.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

A camada de armazenamento é uma das mais bem guardadas do tree:
`storage.RelInside` é o único teste de contenção de path (CLAUDE.md §1 regra
7), `writeNoteFile` escreve via `storage.ReplaceFile` (temp + rename), o upload
de imagem tem meta-teste de traversal próprio
(`TestUploadImageStaysInTheImagesDirectory`, upload_test.go:131-150 —
`../../md/Welcome.png`, `/etc/evil.png`, `sub/dir/deep.png`) porque o stdlib
`multipart.Part.FileName()` aplica `filepath.Base` (RFC 7578 §4.2). O caminho
de arquivos de usuário (`readUserFile`/`userFileName`) sanitiza de novo e é
testado (upload_test.go:438). Sobraram três achados de governança/consistência
entre as DUAS vias de upload e a lista de extensões compartilhada com o Java.

## Listas identificadas (tags topológicas)

### GAP
- `[STORAGE-EXCHANGE::GAP::GOVERNANCA::001]` — A lista
  `imageUploadExtensions` (upload_handlers.go:23) declara no próprio comentário
  que `ShareIn.java` tem uma cópia ("Keep the copies the same") e **não existe
  meta-teste** que compare as duas (grep em `backend/internal/repocheck/` e
  `android/test/` = zero hits por `imageUploadExtensions`/lista equivalente).
  Padrão da casa é o oposto: `TestFoldTableHasAFrontendCopy` compara as duas
  tabelas de fold; `TestLogFilterPortAgreesWithTheRealJavaScript` compara as
  duas implementações de filtro de log.

### MELHORIA
- `[STORAGE-EXCHANGE::MELHORIA::SEGURANCA::002]` — `.svg` é aceito no upload
  (upload_handlers.go:23) e servido como `image/svg+xml`
  (config/content_types.go:33; asserção em serving_test.go:27) sem `nosniff`,
  sem CSP e sem `Content-Disposition` — um SVG navegado direto executa
  `<script>` no mesmo origin do servidor local (as APIs admin do device ficam
  ao alcance do script do SVG aberto no browser do admin).
- `[STORAGE-EXCHANGE::MELHORIA::CODIGO::003]` — As duas vias de upload
  duplicam o parse multipart e as checagens de limite
  (`saveUploadedFile` upload_handlers.go:34-87 vs `readUserFile` :98-146),
  incluindo o comentário do threshold 10 MB duplicado — manutenção dupla já
  manifestada (limite checado de formas diferentes: `header.Size` vs
  `len(data)`).

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `imageUploadExtensions` (upload_handlers.go:23) | usado por `handleUpload`; cópia em ShareIn.java | MÉDIO | `upload_test.go` (casos de tipo), `TestUploadImageStaysInTheImagesDirectory` |
| `serving.go` Content-Type de `html/images/*.svg` | caminho de cada asset servido | MÉDIO | `serving_test.go:27`, `TestDarkMapsAreTheSame` (assets) |
| `saveUploadedFile`/`readUserFile` | folhas; chamadas de `handleUpload`/`handleUploadUserFile` (closures) | MÉDIO | `upload_test.go` (501 linhas, os dois caminhos) |
| `android/ShareIn.java` | camada Java, sem grafo Go | MÉDIO | `android/test/` (3 suites; nenhuma cobre a lista) |

Nota: o traversal em `saveUploadedFile` foi investigado e **descartado** como
achado — o stdlib sanitiza o filename e o meta-teste da casa prende o
comportamento (FASE 2, loop de validação).
