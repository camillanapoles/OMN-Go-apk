# Tasks — debt-gitnexus-injection

## P0 — Skills mandam o analyze com o flag anti-injeção

- [x] Cada instrução de `analyze` em `local/skills/gitnexus-*/SKILL.md` vira
      `analyze --skip-agents-md`, com nota curta do porquê.

## P1 — meta-teste quebra o gate se os artefatos voltarem

- [x] `backend/internal/repocheck/` ganha
      `TestGitNexusStaysOutOfTheContract`: `CLAUDE.md` sem
      `<!-- gitnexus:start -->`; `AGENTS.md` de raiz sem o bloco. Mensagem
      clara: rodar o analyze com `--skip-agents-md`.

## P2 — nota do padrão de grep das mensagens de teste

- [x] Registrar num skill de diagnóstico: mensagens de `t.Errorf` saem como
      `file:line:` sem coluna. O formato `file:line:col:` do vet não as
      captura num grep de annotations de CI.

## Fechamento

- [x] CI verde → M==N na issue + `--remove-label spec:draft`. Issue fica OPEN.

## Nota de execução (2026-11-20)

- Os cinco skills gitnexus-* e o gitnexus-cli receberam o flag
  `--skip-agents-md` em cada instrução de analyze, com a razão na mesma
  linha.
- `TestGitNexusStaysOutOfTheContract` cobre `CLAUDE.md` e `AGENTS.md` de
  raiz pela literal `<!-- gitnexus:start -->`. O arquivo de raiz ausente
  conta como estado limpo.
- A nota do P2 entrou no fim de `gitnexus-debugging/SKILL.md`.
- Bump 26.11.41/2611041 porque o meta-teste é código Go.
