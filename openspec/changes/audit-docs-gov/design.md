# Design — audit-docs-gov

## Stack envolvida

- Markdown puro (docs do fork em pt-BR, docs do upstream em STE/inglês);
  GitNexus CLI como ferramenta de refresh; `repocheck` para novos meta-testes
  de docs (opcional).
- Regra de idioma do fork: `docs/**` é pt-BR (RUNBOOK/CONTRIBUTING/CODEMAPS);
  `doc/**` herdado é STE — manter cada lado no seu idioma; STE tem regras
  próprias (CLAUDE.md §10) para qualquer linha nova em `doc/`.

## Construção (build/test/lint)

- Nada de build afetado (docs não entram no contexto Docker após o item da
  issue build-ci — coordernar a ordem: dockerignore primeiro evita pagar
  contexto com docs erradas).
- Opcional (se aceito): meta-teste que proíbe a literal `Dockerfile.ci` no
  tree (padrão repocheck: ler fontes, falhar com lista de hits) — garante que
  o nome morto não volte.
- Commits: `doc(gitops): ...` sem bump? — docs de `docs/**` não afetam o
  produto, mas a regra §6 diz "bump em todo commit" com exceção negociada;
  seguir a exceção registrada nesta auditoria para commits 100% docs/openspec
  (sem bump), e com bump se tocar `backend/` (o meta-teste opcional toca!).

## Governança

- Merge: 3 checks verdes (GitOps Guard é o relevante; Quality Gate/APK Build
  rodam de qualquer forma).
- Restrições: não reescrever docs do upstream (`doc/**` em STE) nesta issue —
  o escopo é `docs/**` do fork; ADRs são intocáveis (só leitura); não criar
  segundo `CODE_MAP` concorrente — o item 002 explicitamente decide entre
  refresh ou apontar para a autoridade (uma das duas, não as duas mal-feitas).
- Rastreabilidade com a auditoria: esta change fecha os itens topológicos
  `[DOCS-GOV::ERRO::GOVERNANCA::001]` e `[DOCS-GOV::GAP::GOVERNANCA::002]`.
