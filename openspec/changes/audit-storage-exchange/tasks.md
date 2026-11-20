# Tasks — audit-storage-exchange

## P0 (governança de contrato compartilhado)

- [ ] Meta-teste que compara `imageUploadExtensions` (Go) ↔ lista de
  `ShareIn.java` (GOVERNANCA::001).
  - **Verificação:** adicionar uma extensão falsa só no lado Go →
    `go test ./backend/internal/repocheck/` no Gate falha nomeando os dois
    arquivos.
  - **Validação:** nenhum teste de comportamento muda; `java_test.go` verde.

## P1 (segurança — requer decisão do mantenedor)

- [ ] Header de sandbox para `.svg` servido (SEGURANCA::002).
  - **Verificação:** `serving_test.go`: GET de um `.svg` sob `html/images/`
    carrega `Content-Security-Policy: script-src 'none'`; GET de `.png`
    não muda.
  - **Validação:** `TestUploadImageStaysInTheImagesDirectory` e todos os
    Content-Type de `serving_test.go` idênticos; `doc/API.md`/docs de
    imagens atualizados se o comportamento documentado mudar.

## P2 (qualidade interna)

- [ ] Dedup do parse/limite das duas vias de upload (CODIGO::003).
  - **Verificação:** `upload_test.go` (501 linhas) passa SEM edição; os dois
    caminhos continuam com os mesmos códigos 400/500.
  - **Validação:** `TestCommentShare` verde (o helper encurta o arquivo);
    blast radius local confirmado com `npx -y gitnexus@1.6.12 impact
    saveUploadedFile`.
