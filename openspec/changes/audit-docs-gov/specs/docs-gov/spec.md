# Spec — docs-gov (detalhamento 5W1H)

## [DOCS-GOV::ERRO::GOVERNANCA::001] Docs instruem `Dockerfile.ci` inexistente

- **What (O quê):** a tabela de Commands de `docs/CONTRIBUTING.md` (3 linhas)
  e o fluxo de deploy de `docs/RUNBOOK.md` citam `-f Dockerfile.ci` /
  "build Dockerfile.ci", arquivo que não existe no tree (o build único é o
  `Dockerfile`).
- **Why (Por quê):** o primeiro comando que um contribuidor copia da doc falha
  ("path Dockerfile.ci not found"); o RUNBOOK descreve o gate de release com
  um nome morto — em emergência, a pessoa opera com instrução errada. A fonte
  morta também alimenta os comentários `generated:source=...;Dockerfile.ci`,
  que mentem sobre a origem.
- **Where (Onde):** `docs/CONTRIBUTING.md:20,24,25,42`; `docs/RUNBOOK.md:10,16`;
  evidência de morte: `.github/workflows/fork-ci.yml:7-8`; árvore: só
  `Dockerfile` existe.
- **When (Quando):** build/ops — sempre que alguém seguir as docs (contribuir,
  operar release).
- **Who (Quem):** contribuidores novos; mantenedor em operação de release;
  agentes que leem RUNBOOK/CONTRIBUTING como fonte.
- **How (Como):** substituir `Dockerfile.ci` → `Dockerfile` nas 3 linhas de
  CONTRIBUTING e na linha do RUNBOOK; atualizar os comentários
  `generated:source` para `Dockerfile`. Critério: nenhum `Dockerfile.ci` no
  tree (`grep -r Dockerfile.ci .` = 0 hits); comandos da tabela executam no
  host com Docker (validar no CI/ambiente do mantenedor).

## [DOCS-GOV::GAP::GOVERNANCA::002] Codemaps do fork sem rotina de refresh

- **What (O quê):** `docs/CODEMAPS/*.md` carregam `<!-- codemap: refreshed
  2026-10-04 · base d9f6c82 · scan: manual -->` e nenhuma automação ou rotina
  os mantém; já estão 2+ versions atrás do tree.
- **Why (Por quê):** o propósito do codemap é orientar ONDE mudar; um codemap
  velho desorienta (aponta arquivo/linha que mudou) — pior do que não ter.
  O fork já tem a ferramenta certa no fluxo desta auditoria (GitNexus
  `detect-changes`/`analyze`).
- **Where (Onde):** `docs/CODEMAPS/{architecture,backend,data,dependencies,frontend}.md`
  (header de cada um); contraste: `doc/CODE_MAP.md` (upstream, com guarda §6).
- **When (Quando):** build/documentação — a cada versão publicada.
- **Who (Quem):** mantenedor do fork; agentes de sessão que começam pelos
  codemaps.
- **How (Como):** (1) refresh agora (5 arquivos contra o tree atual);
  (2) definir a rotina no `docs/RUNBOOK.md`: após cada tag de release, rodar
  `npx -y gitnexus@1.6.12 analyze . --skip-fts --skip-agents-md` + revisão
  manual dos 5 codemaps, ou migrar o conteúdo para apontar ao
  `doc/CODE_MAP.md` como autoridade e manter só as diferenças do fork.
  Critério: header `base <commit atual>` nos 5 arquivos após o refresh;
  rotina documentada com comando exato.
