# Proposal — storage-templates-ux

> **Issue:** #56
> **Domínio:** `ux` + `storage` + `interop` — editor, settings e a pasta de notas.
> **Branch da sessão:** `arena/3e348850-omn-go-apk` · **Commit base:** `bfba41a`
> **Nota:** commit toca Go, JS, CSS e templates — com bump de versão.

## Cenário atual

A toolbar do editor tem cinco ferramentas de navegação e registro (abreviação,
ciclo de seleção, wrap, linhas, busca) e nenhum botão de design de texto. A
criação de uma nota começa vazia, sem ponto de partida. No desktop o diretório
de storage é fixo (`./data`), e a pessoa não escolhe onde as notas ficam. Os
templates, quando existirem, precisam nascer abertos: Markdown puro numa pasta
drop-in, sem formato próprio que trave a pessoa.

## Listas identificadas (tags topológicas)

### GAP
- `[UX::GAP::USABILIDADE::001]` — A registry `TOOLS` do editor não tem botões
  de design de texto (evidência: `omn-go-editor.js`, 5 entradas de navegação).
- `[STORAGE::GAP::GOVERNANCA::002]` — `initStorage` aceita `overrideDir` só
  por chamada de código; não há escolha persistida da pessoa (evidência:
  desktop passa `""` em `main_desktop.go:15`).
- `[TEMPLATES::GAP::PRODUTIVIDADE::003]` — Sem templates: nota nova começa do
  zero (evidência: não existe endpoint nem pasta de templates).
- `[INTEROP::GAP::INTEROP::004]` — Integrabilidade dos templates: sem diretório
  aberto, cada formato próprio seria um lock-in novo.

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `TOOLS` + helpers do editor | todo o editor | BAIXO | `TestEditorToolbarHoldsTheTextDesignTools` (novo) |
| `/api/templates` (rota nova) | tabela de rotas | BAIXO | `TestEditorOffersTheNoteTemplates` + linha em `doc/API.md` |
| `initStorage` (cadeia de ponteiros) | arranque do app | MÉDIO | `TestStorageChainWalksThePointers` + `TestPeekStorageDir` (novos) |
| `Config.StorageDir` + tabela fields | config page + POST | MÉDIO | `TestEveryConfigFieldIsInTheTable` (existente) + meta-teste novo |
| `config_page.html` (tela Storage) | settings | BAIXO | meta-teste novo (marcadores) |

## Ciclo de gate (registro)

O primeiro ciclo do gate achou dois defeitos de integração, ambos de testes
da casa que guardam estruturas inteiras: o mapa `fileGroups` de
`group_links_test.go` exige um grupo para cada arquivo novo do pacote app, e
`TestBaseline_RouteSet` exige a rota nova na lista exata de rotas. Os dois
entraram no tree com esta change.
