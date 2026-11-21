Spec OpenSpec: [`openspec/changes/audit-skills-harness/`](../../openspec/changes/audit-skills-harness/) (branch `arena/d9c9b8dd-omn-go-apk`)

> **PRIORIDADE MÁXIMA — executar ANTES das demais issues** (otimiza os agentes que executarão as outras 7).

## Tags topológicas contempladas

| Tag | Categoria | Resumo |
| --- | --- | --- |
| `[SKILLS-HARNESS::GAP::GOVERNANCA::001]` | GAP | Skills não versionadas/replicáveis (`.claude/skills/` gerado ad-hoc, ignorado pelo git) |
| `[SKILLS-HARNESS::GAP::INTEROPERABILIDADE::002]` | GAP | Sem gate de auditoria de skill nova (caso concreto: chrisbanes Kotlin/Compose × §1 Java puro) |
| `[SKILLS-HARNESS::MELHORIA::INTEROPERABILIDADE::003]` | MELHORIA | Formato universal SKILL.md/AGENTS.md + fontes pinadas |
| `[SKILLS-HARNESS::MELHORIA::USABILIDADE::004]` | MELHORIA | Pool inativo + seleção por-spec nos AGENTS.md |

## Fontes validadas nesta auditoria (evidência na spec)

- ✅ `multica-ai/andrej-karpathy-skills` — guidelines gerais (pin `2c60614`)
- ✅ `mattpocock/skills` — eng. software (commit `abeeaf9`, 2026-11-18; CI valida frontmatter; 9 agentes)
- ✅ `chrisbanes/skills` — Android (commit `edd18ff`, 2026-11-20; **triagem obrigatória**: Kotlin/Compose proibidos §1)
- ✅ `affaan-m/ECC` — harness (commit `4adeee6`, 2026-11-20; instalar seletivas, não o repo)
- ✅ `jev-ai/jev-agent-skill` — decisões tipadas do harness (pin `8702f43`; **gate**: serviço externo + `JEV_API_KEY` fora do tree, status `inactive`, opt-in por estágio; docs: thejevai.com/docs)

## Checklist macro (P0 → P2)

- [ ] **P0** `local/skills/manifest.yml` (fonte+pin+status+reason) + 6 skills gitnexus como globais
- [ ] **P0** `local/agent-setup/install.sh` idempotente (bash+git, Termux-ok; uma máquina nova = clone + install)
- [ ] **P1** Meta-teste repocheck do manifest (gate de auditoria; global sem reason/pin falha)
- [ ] **P1** Triagem das fontes contra CLAUDE.md §1 (chrisbanes inativo até decisão)
- [ ] **P2** Seção "Skills ativas" nos 8 AGENTS.md + rotina no RUNBOOK

Interoperabilidade: formatos abertos (SKILL.md frontmatter + AGENTS.md), um tree fonte, seleção declarada, fonte terceira sempre pinada. Zero lock-in de vendor. `local/` nunca entra no build.
Contrato local: `AGENTS.md` dentro do diretório da spec.
