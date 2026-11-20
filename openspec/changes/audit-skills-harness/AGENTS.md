# AGENTS.md — audit-skills-harness (contrato local da change)

Issue: #30

## Contexto isolado

Infra de habilidades dos agentes: fontes externas validadas (karpathy,
mattpocock, chrisbanes, ECC — evidência e pins na spec), formato universal
SKILL.md + AGENTS.md, tree fonte `local/skills/`, instalador
`local/agent-setup/install.sh`, gate no `repocheck`. Ler antes: `CLAUDE.md`
§1 (constraints — o gate existe para elas), §2 (papel de `local/`),
`docs/CONTRIBUTING.md` (Termux) e a spec desta change.

## Skills ativas

- Globais (manifest `local/skills/manifest.yml`; instalar com
  `bash local/agent-setup/install.sh`): `gitnexus-exploring`,
  `gitnexus-debugging`, `gitnexus-impact-analysis`, `gitnexus-refactoring`,
  `gitnexus-guide`, `gitnexus-cli`, `karpathy-guidelines`.
- Pool (instalar com `bash local/agent-setup/install.sh <id>`):
  - `ecc-conventions` (affaan-m/ECC) — convenções de desenvolvimento na revisão do próprio harness. Uso: durante a triagem de skills contra §1.
  - `triage` (mattpocock/skills) — triagem de skill nova (fluxo de 3 passos do manifest). Uso: no passo de auditoria do fluxo.

## Arquivos permitidos para edição

- `local/skills/**` (tree + manifest)
- `local/agent-setup/install.sh`
- `backend/internal/repocheck/agent_skills_test.go` (novo)
- `openspec/changes/*/AGENTS.md` (seção "Skills ativas")
- `docs/RUNBOOK.md` (rotina)
- `.gitignore` (SÓ se um caminho novo precisar de regra — `local/` já entra)

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

## Proibições do GitNexus

- Nenhuma skill global sem `reason` + `pin` (meta-teste prende).
- Skills de Android (chrisbanes) permanecem `inactive` enquanto §1 regra 1
  proibir AndroidX/Kotlin — o meta-teste não resolve isso: é decisão de
  mantenedor registrada no manifest.
- Nunca ativar o pool inteiro de uma vez (contexto de agente inflado).
