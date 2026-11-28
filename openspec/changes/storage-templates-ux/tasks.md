# Tasks — storage-templates-ux

## P1 (design de texto)

- [x] Box de botões de design de texto na toolbar do editor
  (UX::GAP::USABILIDADE::001).
  - **Verificação:** negrito, itálico, H1, H2, lista, lista numerada, caixa
    de tarefa, citação, código inline e link envolvem a seleção no editor
    aberto; pressionar duas vezes o mesmo botão desfaz o efeito.
  - **Validação:** `TestEditorToolbarHoldsTheTextDesignTools` verde no Fork
    CI; cada entrada nova usa o registry `TOOLS` sem mudar o construtor.

## P1 (templates)

- [x] Templates: pasta aberta, endpoint e box no editor
  (TEMPLATES::GAP::PRODUTIVIDADE::003 + INTEROP::GAP::INTEROP::004).
  - **Verificação:** `GET /api/templates` responde os modelos da pasta
    `templates/` do storage e os internos; um `.md` solto na pasta aparece no
    box e sombreia um interno de mesmo nome; inserir preenche a nota vazia.
  - **Validação:** teste do handler verde; `TestEditorOffersTheNoteTemplates`
    verde; ADR 0032 registrado no índice de decisões.

## P1 (diretório de storage)

- [x] `storage_dir` na config com tela própria e cadeia de ponteiros no
  arranque (STORAGE::GAP::GOVERNANCA::002).
  - **Verificação:** no desktop, config.json com `storage_dir` move o storage
    na próxima abertura; um ponteiro que aponta de volta para a origem não
    trava o arranque; o Android mantém o diretório do wrapper.
  - **Validação:** `TestStorageChainWalksThePointers` e `TestPeekStorageDir`
    verdes; `TestEveryConfigFieldIsInTheTable` continua verde; ADR 0033 no
    índice.
