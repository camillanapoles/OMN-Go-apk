# AGENTS.md — audit-docs-gov (contrato local da change)

Issue: #37

## Contexto isolado

Documentação do fork (`docs/`: RUNBOOK, CONTRIBUTING, CODEMAPS) e seu contrato
com a governança executável (`backend/internal/repocheck/`). Autoridades:
`doc/CODE_MAP.md` é o mapa vivo do upstream (guarda §6); `docs/CODEMAPS` é a
camada do fork; `docs/RUNBOOK` é operação; `docs/CONTRIBUTING` é onboarding.
Ler antes: `docs/CONTRIBUTING.md`, `docs/RUNBOOK.md`, `doc/gitops/README.md`,
`doc/decisions/README.md` e a spec desta change.

## Arquivos permitidos para edição

- `docs/CONTRIBUTING.md`, `docs/RUNBOOK.md`, `docs/CODEMAPS/*.md`
- `backend/internal/repocheck/` (APENAS o meta-teste opcional do P2, se
  aprovado)
- `openspec/changes/audit-docs-gov/**` (esta própria change)

## Proibido editar

- `doc/**` (upstream em STE — inclusive ADRs e CODE_MAP)
- `CLAUDE.md` (contrato vigente não muda numa issue de docs)
- Qualquer arquivo de código, build ou workflow

## Comandos obrigatórios antes do commit

1. `grep -rn "Dockerfile.ci" docs/ doc/ .` → 0 hits antes de commitar.
2. `npx -y gitnexus@1.6.12 check --cycles` e `detect-changes`.
3. PR → GitOps Guard verde (`gh run watch`).
4. Sem bump se o diff for 100% `docs/`+`openspec/` (exceção da auditoria);
   com bump se o meta-teste opcional (Go) entrar.

## Proibições do GitNexus

- O grafo não indexa Markdown como código de produto — a validação de docs é
  humana/grep; nunca "validar" doc por presença no índice.
- Não usar o codemap velho como fonte de verdade de linhas: conferir no tree
  real antes de qualquer referência arquivo:linha nova.
- Nenhum número de versão em comentários de código novo (se o meta-teste
  entrar, `TestNoVersionNumberInComments` aplica).

## Skills ativas

Globais:
- `karpathy-guidelines` — `multica-ai/andrej-karpathy-skills` @ `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`, `gitnexus-refactoring`, `gitnexus-guide`, `gitnexus-cli` — `abhigyanpatwari/GitNexus` @ `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Seleção por spec: nenhuma adicional.

`docs-governance` depende de arquivos auxiliares e não deve ser instalada como um SKILL.md isolado.
