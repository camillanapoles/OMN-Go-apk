# Spec — tools-gov (detalhamento 5W1H)

## [SKILLS-HARNESS::DEBITO::GOVERNANCA::001] `analyze` injeta texto nos contratos

- **What (O quê):** `gitnexus analyze` (1.6.12) escreve um bloco
  `<!-- gitnexus:start -->` no fim de `CLAUDE.md` e cria `AGENTS.md` na raiz
  com instruções próprias da ferramenta.
- **Why (Por quê):** `CLAUDE.md` é intocável pelas regras do repo. Cada
  analyze exigiu um revert manual (`git checkout -- CLAUDE.md && rm -f
  AGENTS.md`). Sem isso, um commit acidental leva o texto da ferramenta para
  o contrato vigente de todas as sessões.
- **Where (Onde):** `CLAUDE.md` (fim do arquivo); `AGENTS.md` (raiz, novo);
  instruções do analyze em `local/skills/gitnexus-*/SKILL.md`.
- **When (Quando):** sempre que um agente roda o analyze para atualizar o
  índice do GitNexus (pré-commit, diagnóstico, impact).
- **Who (Quem):** agentes de sessão; mantenedor ao revisar diffs.
- **How (Como):** o flag `--skip-agents-md` pula a injeção (verificado em
  2026-11-20: o analyze com o flag não toca nem `CLAUDE.md` nem `AGENTS.md`;
  o help do flag confirma o alcance; o modo `--watch` já pula por padrão).
  Critério: os skills mandam sempre o flag; o meta-teste do P1 quebra o
  gate se o marcador voltar.

## [SKILLS-HARNESS::GAP::GOVERNANCA::002] Sem gate contra os artefatos injetados

- **What (O quê):** nenhum teste do repocheck impede o commit de
  `CLAUDE.md` com o bloco `gitnexus:start` ou de um `AGENTS.md` de raiz
  injetado.
- **Why (Por quê):** a governança do repo vive em `CLAUDE.md`. Um gate
  executável transforma a regra "intocável" em fato verificado no CI.
- **Where (Onde):** `backend/internal/repocheck/` (meta-teste novo);
  `CLAUDE.md`; `AGENTS.md` de raiz.
- **When (Quando):** cada Quality Gate do Fork CI; cada `go test` local.
- **Who (Quem):** CI; agentes antes do commit.
- **How (Como):** `TestGitNexusStaysOutOfTheContract` procura a literal
  `<!-- gitnexus:start -->` nos dois arquivos (o de raiz só quando existe)
  e falha com mensagem que manda usar `--skip-agents-md`. Critério: o
  needle some → o gate quebra com a mensagem clara.
