# Tasks — audit-storage-exchange

## P0 (governança de contrato compartilhado)

- [x] Meta-teste que compara `imageUploadExtensions` (Go) ↔ lista de
  `ShareIn.java` (GOVERNANCA::001).
  - **Verificação:** adicionar uma extensão falsa só no lado Go →
    `go test ./backend/internal/repocheck/` no Gate falha nomeando os dois
    arquivos.
  - **Validação:** nenhum teste de comportamento muda; `java_test.go` verde.

## P1 (segurança — requer decisão do mantenedor)

- [x] Header de sandbox para `.svg` servido (SEGURANCA::002).
  - **Verificação:** `serving_test.go`: GET de um `.svg` sob `html/images/`
    carrega `Content-Security-Policy: script-src 'none'`; GET de `.png`
    não muda.
  - **Validação:** `TestUploadImageStaysInTheImagesDirectory` e todos os
    Content-Type de `serving_test.go` idênticos; `doc/API.md`/docs de
    imagens atualizados se o comportamento documentado mudar.

## P2 (qualidade interna)

- [x] Dedup do parse/limite das duas vias de upload (CODIGO::003).
  - **Verificação:** `upload_test.go` (501 linhas) passa SEM edição; os dois
    caminhos continuam com os mesmos códigos 400/500.
  - **Validação:** `TestCommentShare` verde (o helper encurta o arquivo);
    blast radius local confirmado com `npx -y gitnexus@1.6.12 impact
    saveUploadedFile`.

## Nota de execução (2026-11-20)

- Decisão do mantenedor no P1: CSP `script-src 'none'` (via escolhida na
  issue; não removeram .svg da lista).
- 001: `TestUploadExtensionListsStayInSync` lê as duas listas por regex e
  compara como conjunto. Verificação negativa rodou: ext falsa só no lado Go
  quebra o teste com os dois paths na mensagem.
- 003: `parseUploadForm` + `uploadTooLarge` unificam o parse e a mensagem de
  limite das duas vias. upload_test.go passou sem edição; as mensagens ficam
  byte a byte.
- 002: `setSandboxHeader` em serving.go liga o CSP por extensão nos dois
  pontos de Content-Type (serveEmbeddableAsset e serveStorageSubdir). O
  nosniff já é global desde o #32. Fontes e vendored não mudam de resposta.
- Share exato = 199‰ (3341/16723). Bump 26.11.43/2611043.
