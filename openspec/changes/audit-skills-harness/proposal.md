# Proposal — audit-skills-harness (PRIORIDADE MÁXIMA)

> **Issue:** _pendente_
> **Domínio:** `skills-harness` — `local/skills/` (novo), `local/agent-setup/`
> (novo), `.gitignore`, `docs/RUNBOOK.md` (rotina). Executar esta issue
> **ANTES** das demais: ela otimiza os agentes que executarão as outras 7.
> **Branch da sessão:** `arena/d9c9b8dd-omn-go-apk` · **Commit base:** `02e8cd8`
> **Nota:** commit de specs — sem bump de `APP_VERSION`.

## Cenário atual

O repositório hoje tem habilidades de agente **não governadas**: o GitNexus
auto-gera 6 skills em `.claude/skills/gitnexus-*/SKILL.md` a cada `analyze`
(--skip-agents-md não evita as skills), mas o diretório está no `.gitignore`
— cada máquina/worktree regenera ou fica sem; não há install replicável, não
há catálogo, não há gate de auditoria, e o `AGENTS.md` de cada spec não
declara quais skills a spec usa. As 4 fontes externas desejadas foram
**validadas nesta auditoria** (ver `specs/skills-harness/spec.md` para
evidência): todas reais, ativas e no formato aberto SKILL.md/frontmatter com
plugins multi-agente — mas o `chrisbanes/skills` recomenda Kotlin/Compose,
que colide com a constraint §1 regra 1 do `CLAUDE.md` (sem AndroidX, Java
puro): prova de que o gate de auditoria é indispensável.

## Listas identificadas (tags topológicas)

### GAP
- `[SKILLS-HARNESS::GAP::GOVERNANCA::001]` — Skills de agente não são ativos
  versionados nem replicáveis: `.claude/skills/` é gerado ad-hoc pelo GitNexus
  e ignorado pelo git; nenhum `install.sh`/tree garante o mesmo conjunto em
  cada máquina, worktree e sessão (`arena/*`); o conjunto vigente não é
  declarado em lugar nenhum.
- `[SKILLS-HARNESS::GAP::INTEROPERABILIDADE::002]` — Sem gate de auditoria
  para habilidade nova: nada impede uma skill global que contradiga as
  constraints do projeto (o caso `chrisbanes/skills` × Kotlin/Compose é
  concreto), que traga instruções de vendor lock-in, ou que entre sem origem
  e versão (pin) registradas.

### MELHORIA
- `[SKILLS-HARNESS::MELHORIA::INTEROPERABILIDADE::003]` — Adoção do formato
  universal (SKILL.md com frontmatter YAML + contrato `AGENTS.md`): fontes
  externas instaláveis por script idempotente, seleção por skill (não repo
  inteiro), pin de commit por fonte, um único tree `local/skills/` que serve
  Claude Code, Codex, Cursor, OpenCode e afins — sem lock-in de vendor.
- `[SKILLS-HARNESS::MELHORIA::USABILIDADE::004]` — Seleção por-spec do pool:
  cada spec declara no seu `AGENTS.md` as skills ativas (globais do projeto +
  selecionadas do pool inativo para o domínio da spec); o pool inativo fica
  catalogado no manifest com o momento sugerido de ativação.

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `.gitignore` (linhas `.claude/`) | todas as ferramentas de agente | BAIXO | nenhum (validar manualmente que skills versionadas ficam em `local/skills/`, não em `.claude/`) |
| `local/` (novo conteúdo) | Docker context já exclui `local/` — zero impacto no build | BAIXO | `.dockerignore:7` |
| `repocheck/` (1 meta-teste novo do manifest) | Quality Gate | MÉDIO | o próprio meta-teste |
| `AGENTS.md` das 7 specs (seção "Skills ativas") | execução das issues | BAIXO | revisão humana no PR |

Regra da casa aplicável: `local/` é mantenedor-only e nunca vai para o build
(CLAUDE.md §2) — o lugar correto para a infra de agentes; nada disso entra
no APK/imagem.
