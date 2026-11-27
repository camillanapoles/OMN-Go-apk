# AGENTS.md — audit-skills-harness (contrato local da change)

Issue: #30

## Contexto isolado

Infra de habilidades dos agentes: fontes externas validadas (karpathy,
mattpocock, chrisbanes, ECC — evidência e pins na spec), formato universal
SKILL.md + AGENTS.md, tree fonte `local/skills/`, instalador
`local/agent-setup/install.sh`, gate no `repocheck`. Ler antes: `CLAUDE.md`
§1 (constraints — o gate existe para elas), §2 (papel de `local/`),
`docs/CONTRIBUTING.md` (Termux) e a spec desta change.

## Arquivos permitidos para edição

- `local/skills/manifest.yml` (manifest; no conteúdo terceiro)
- `local/agent-setup/install.sh`
- `backend/internal/repocheck/agent_skills_test.go` (novo meta-teste)
- `backend/internal/repocheck/testdata/agent-skills-manifest.yml` (fixture do gate)
- `AGENTS.md` (instruções do workspace e apêndice geral)
- `openspec/changes/*/AGENTS.md` (seção "Skills ativas")
- `openspec/changes/audit-skills-harness/design.md` e `tasks.md` (decisões e evidências)
- `docs/RUNBOOK.md` (rotina)
- `.gitignore` (SÓ se um caminho novo precisar de regra — `.claude/` já entra)

`local/skills/manifest.yml` points to the fixture in `repocheck`. This keeps
`local/` out of the Docker context while the Go gate can read the same file.
Do not add an exception for `local/` to `.dockerignore`.

## Proibido editar

- `CLAUDE.md`, `doc/**` (contrato e docs upstream)
- `backend/**` exceto o meta-teste novo; `android/**`; `Dockerfile`;
  `.github/workflows/**`
- `local/skills/` NÃO pode receber conteúdo vendorado de fonte terceira
  (só o manifest as referencia)

## Comandos obrigatórios antes do commit

1. `bash local/agent-setup/install.sh && bash local/agent-setup/install.sh`
   (idempotência: mesmos hashes)
2. `npx -y gitnexus@1.6.12 check --cycles && npx -y gitnexus@1.6.12 detect-changes`
3. PR → Quality Gate do Fork CI (`gh run watch`) — valida o meta-teste.
4. Bump de versão nos 3 arquivos (toca Go).

## Contrato do instalador

- `--spec` deve ler os IDs na linha `Seleção por spec:` do `AGENTS.md` da
  change e falhar fechado se eles não coincidirem com o conjunto `applies_to`
  do manifest.

## Proibições do GitNexus

- Nenhuma skill global sem `reason` + `pin` (meta-teste prende).
- Skills de Android (chrisbanes) permanecem `inactive` enquanto §1 regra 1
  proibir AndroidX/Kotlin — o meta-teste não resolve isso: é decisão de
  mantenedor registrada no manifest.
- Nunca ativar o pool inteiro de uma vez (contexto de agente inflado).

## Skills ativas

Globais:
- `karpathy-guidelines` — `multica-ai/andrej-karpathy-skills` @ `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`, `gitnexus-refactoring`, `gitnexus-guide`, `gitnexus-cli` — `abhigyanpatwari/GitNexus` @ `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Seleção por spec: nenhuma adicional.

As globais cobrem descoberta e impacto; as demais fontes aguardam revisão.
