Spec OpenSpec: [`openspec/changes/audit-storage-exchange/`](../../openspec/changes/audit-storage-exchange/) (branch `arena/d9c9b8dd-omn-go-apk`)

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[STORAGE-EXCHANGE::GAP::GOVERNANCA::001]` | GAP | Lista de exts Go ↔ cópia em `ShareIn.java` sem meta-teste |
| `[STORAGE-EXCHANGE::MELHORIA::SEGURANCA::002]` | MELHORIA | `.svg` executável sem sandbox/nosniff (decision-required) |
| `[STORAGE-EXCHANGE::MELHORIA::CODIGO::003]` | MELHORIA | Duplicação `saveUploadedFile` × `readUserFile` |

## Checklist macro (P0 → P2)

- [ ] **P0** Meta-teste que compara `imageUploadExtensions` (Go) ↔ `ShareIn.java`
- [ ] **P1** Header de sandbox para `.svg` servido (CSP `script-src 'none'`) — **decisão do mantenedor**: CSP vs remover `.svg` da lista
- [ ] **P2** Dedup do parse multipart/limite das duas vias

Nota da auditoria: traversal em upload foi investigado e **descartado** — o stdlib aplica `filepath.Base` (RFC 7578 §4.2) e `TestUploadImageStaysInTheImagesDirectory` prende o comportamento.
Contrato local: `AGENTS.md` dentro do diretório da spec.
