Spec OpenSpec: [`openspec/changes/debt-gitnexus-injection/`](../../openspec/changes/debt-gitnexus-injection/) (branch `arena/1d9904ca-omn-go-apk`)

Débito aberto pela sessão de execução do #32 (achado de 2026-11-20).

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[SKILLS-HARNESS::DEBITO::GOVERNANCA::001]` | DEBITO | `gitnexus analyze` escreve `<!-- gitnexus:start -->` em `CLAUDE.md` (intocável) e cria `AGENTS.md` na raiz |
| `[SKILLS-HARNESS::GAP::GOVERNANCA::002]` | GAP | Nenhum gate quebra o build se os artefatos injetados entram num commit |

## Achado

`npx -y gitnexus@1.6.12 analyze` injeta um bloco `<!-- gitnexus:start -->` com instruções próprias no fim de `CLAUDE.md` e grava `AGENTS.md` na raiz. `CLAUDE.md` é intocável pelas regras do repo. Cada analyze exigiu um revert manual (`git checkout -- CLAUDE.md && rm -f AGENTS.md`) — débito operacional, e nada impede o commit acidental dos artefatos.

## Correção verificada (teste, não crença)

`gitnexus analyze --skip-agents-md` não escreve nem em `CLAUDE.md` nem em `AGENTS.md` (validado nesta branch em 2026-11-20). O help do flag: "Skip updating the gitnexus section in AGENTS.md and CLAUDE.md. Does not skip standard skills in .claude/skills or .agents/skills; use --skip-skills for those." O modo `--watch` já pula a injeção por padrão.

## Checklist macro (P0 → P2)

- [ ] **P0** Skills `local/skills/gitnexus-*/SKILL.md`: cada instrução de `analyze` vira `analyze --skip-agents-md`, com nota do porquê
- [ ] **P1** meta-teste `TestGitNexusStaysOutOfTheContract` no repocheck: `CLAUDE.md` sem `<!-- gitnexus:start -->`; `AGENTS.md` de raiz sem o bloco — o gate quebra com mensagem clara
- [ ] **P2** (opcional) nota num skill de diagnóstico: mensagens de `t.Errorf` saem como `file:line:` sem coluna; o formato `file:line:col:` do vet não as captura

## Padrão de fechamento

M==N nesta issue + `gh issue edit <n> --remove-label spec:draft` quando o gate verde aprovar; a issue fica OPEN.

Escopo fechado: `local/skills/gitnexus-*/SKILL.md`, `backend/internal/repocheck/` (1 meta-teste), `openspec/changes/debt-gitnexus-injection/**`. `CLAUDE.md`, `doc/**`, código de produto e workflows intocáveis.
Contrato local: `AGENTS.md` dentro do diretório da spec.
