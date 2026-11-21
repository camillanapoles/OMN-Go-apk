# AGENTS.md — debt-gitnexus-injection (contrato local da change)

Issue: #45

## Contexto isolado

Ferramenta de code intelligence (gitnexus 1.6.12) e os artefatos que ela
escreve nos arquivos de contrato do repo. Autoridades: `CLAUDE.md` é
intocável; o repocheck é a governança executável; `local/skills/` é o
harness versionado do fork.

## Skills ativas

- Globais (manifest `local/skills/manifest.yml`; instalar com
  `bash local/agent-setup/install.sh`): `gitnexus-exploring`,
  `gitnexus-debugging`, `gitnexus-impact-analysis`, `gitnexus-refactoring`,
  `gitnexus-guide`, `gitnexus-cli`, `karpathy-guidelines`, `tdd`,
  `security-review`.

## Arquivos permitidos para edição

- `local/skills/gitnexus-*/SKILL.md` (cada instrução de analyze)
- `local/skills/tdd/SKILL.md` ou `gitnexus-debugging/SKILL.md` (P2, uma nota)
- `backend/internal/repocheck/` (APENAS o meta-teste do P1)
- `openspec/changes/debt-gitnexus-injection/**` (esta própria change)

## Proibido editar

- `CLAUDE.md` (é o objeto da proteção)
- `doc/**`, qualquer código de produto, build ou workflow
- `AGENTS.md` de raiz (não deve nem existir fora da injeção)

## Comandos obrigatórios antes do commit

1. `grep -rn "gitnexus:start" CLAUDE.md AGENTS.md 2>/dev/null` → 0 hits.
2. `git status` limpo de `.gitnexus/`, `.claude/`, `CLAUDE.md`, `AGENTS.md`.
3. parse-check + style-check + gofmt no meta-teste novo; share ≤ 200‰.
4. `node --test backend/frontend/test/*.test.js` verde (299 testes).
5. Fork CI verde (`gh run watch`).

## Proibições do GitNexus

- Rodar SEMPRE `gitnexus analyze --skip-agents-md`. O `analyze` puro é
  proibido a partir desta issue: ele injeta `<!-- gitnexus:start -->` em
  `CLAUDE.md` e cria `AGENTS.md` na raiz.
- `detect-changes` e `check --cycles` não escrevem arquivos.
