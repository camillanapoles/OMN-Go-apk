# Proposal — audit-docs-gov

> **Issue:** #37
> **Domínio:** `docs-gov` — `docs/`, `doc/`, `backend/internal/repocheck/`
> (meta-testes de governança).
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

A governança do fork é incomum por ser **executável**: `backend/internal/repocheck/`
contém meta-testes que leem o repositório inteiro e falham o build
(`importLayers`, `fileGroups`, regras de comentário/versão/UTC, pipelines,
route table, api doc). A documentação do fork vive em `docs/` (RUNBOOK,
CONTRIBUTING, CODEMAPS com header `<!-- codemap: refreshed 2026-10-04 · base
d9f6c82 · scan: manual -->`) e herda `doc/` do upstream (29 ADRs, API.md,
CODE_MAP.md). A auditoria em si (`docs/AUDIT_PROMPT.md`, commit `02e8cd8`)
foi o primeiro uso real do par GitNexus+OpenSpec no fork. Achados: referência
a arquivo de build que não existe mais e codemaps do fork sem rotina de
refresh.

## Listas identificadas (tags topológicas)

### ERRO
- `[DOCS-GOV::ERRO::GOVERNANCA::001]` — `docs/CONTRIBUTING.md` e
  `docs/RUNBOOK.md` instruem comandos com `Dockerfile.ci` (CONTRIBUTING.md:24,
  :25, :42; RUNBOOK.md:10 e o header `generated:source` de ambos), mas o
  arquivo **não existe** — o próprio header do `fork-ci.yml` documenta que
  "The older Dockerfile.ci of the fork disappeared when the original was
  merged in". Todo contribuidor que seguir a tabela de Commands recebe
  "Dockerfile.ci not found".

### GAP
- `[DOCS-GOV::GAP::GOVERNANCA::002]` — `docs/CODEMAPS/*.md` (5 arquivos,
  architecture/backend/data/dependencies/frontend) estão parados na base
  `d9f6c82` (2026-10-04) com `scan: manual` — o tree avançou para 26.11.35
  (commit base `4ee14c2`) e não há rotina de refresh; o `doc/CODE_MAP.md` do
  upstream (autoridade viva, atualizada por regra §6) e os codemaps do fork
  podem divergir silenciosamente.

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `docs/CONTRIBUTING.md` (tabela Commands) | onboarding de todo contribuidor | BAIXO | nenhum (docs fora do repocheck de comandos — gap herdado) |
| `docs/RUNBOOK.md` (deploy) | operação de release | BAIXO | nenhum |
| `docs/CODEMAPS/*.md` | orientação de código do fork | BAIXO | nenhum |
| `repocheck/` (novos meta-testes) | Gate inteiro | MÉDIO | os próprios meta-testes |

Nota positiva registrada: o `doc/CODE_MAP.md` upstream TEM guarda de versão
(regra §6 força atualizar "State of the tree" a cada bump,
`TestVersionIsTheSameInEachFile`) — o gap é só dos codemaps do fork.
