# Spec — skills-harness (detalhamento 5W1H)

## [SKILLS-HARNESS::GAP::GOVERNANCA::001] Skills não versionadas nem replicáveis

- **What (O quê):** as únicas skills do repo são auto-geradas pelo GitNexus em
  `.claude/skills/gitnexus-{exploring,debugging,impact-analysis,refactoring,guide,cli}/SKILL.md`
  e o diretório é ignorado (`.gitignore`); qualquer máquina/worktree novo
  (o fluxo GitOps exige worktree por issue) começa SEM as skills ou com
  versões diferentes.
- **Why (Por quê):** o pipeline Event-Driven do fork puxa cada issue para um
  worktree novo; habilidades instaladas só no head da sessão não acompanham o
  trabalho — agente sem skill = qualidade inconsistente entre issues.
- **Where (Onde):** `.gitignore:11-12` (`.claude/`); `npx gitnexus analyze`
  (gerador); ausentes: `local/skills/` (tree), `local/agent-setup/install.sh`.
- **When (Quando):** setup de cada máquina/worktree/sessão.
- **Who (Quem):** mantenedor e cada agente (Claude Code, Codex, Cursor…) que
  abre um worktree do fork.
- **How (Como):** criar o tree versionado `local/skills/` (fonte da verdade:
  skills do próprio projeto + manifest das externas) e
  `local/agent-setup/install.sh` idempotente que materializa para
  `.claude/skills/` (e para o formato de cada agente suportado). Uma máquina
  nova fica completa com: `git clone && bash local/agent-setup/install.sh`.
  Critério: instalar duas vezes não duplica; `git status` limpo após install;
  remover `.claude/skills/` e re-instalar reproduz o mesmo conjunto (hash dos
  arquivos).

## [SKILLS-HARNESS::GAP::INTEROPERABILIDADE::002] Sem gate de auditoria de habilidade

- **What (O quê):** não existe processo para promolver uma skill a "global":
  sem avaliação de conflito com `CLAUDE.md` §1 (constraints), sem pin de
  origem/versão, sem teste de frontmatter.
- **Why (Por quê):** caso concreto já validado: `chrisbanes/skills` (fonte
  Android recomendada) carrega práticas de Kotlin/Compose — constraint §1
  regra 1 proíbe AndroidX e o projeto é Java puro; uma skill global sem gate
  ensinaria o agente a violar o contrato do repo. Fontes são terceiras e
  mutáveis (chrisbanes commit há horas; sem pin, o conteúdo muda sob os pés).
- **Where (Onde):** novo `local/skills/manifest.yml` (status global/inativo,
  origem, pin de commit, agente-alvo, motivo); novo meta-teste
  `backend/internal/repocheck/agent_skills_test.go` (valida: toda skill do
  manifest existe no tree ou no pin declarado, toda global tem `reason` e
  frontmatter com `name`/`description`).
- **When (Quando):** build (meta-teste no Quality Gate) e setup (install.sh).
- **Who (Quem):** mantenedor (aprova promoção global); agentes (consomem).
- **How (Como):** fluxo de 3 passos fixado: (1) skill candidata entra
  **inativa** no manifest com origem+pin+motivo; (2) auditoria: ler a skill,
  cruzar com `CLAUDE.md` §1/§3-§9 e com as specs abertas, decisão global /
  por-domínio / rejeitada; (3) só `install.sh` ativa o que o manifest marca
  como global — ativação por-spec fica no `AGENTS.md` da spec. Critério:
  meta-teste falha com skill global sem `reason`/pin; manifest com pin
  inválido falha.

## [SKILLS-HARNESS::MELHORIA::INTEROPERABILIDADE::003] Formato universal e fontes validadas

- **What (O quê):** adotar SKILL.md (frontmatter `name`/`description`) +
  `AGENTS.md` como formatos, e as 5 fontes externas auditadas nesta sessão:
  - `multica-ai/andrej-karpathy-skills` — guidelines gerais (Think Before
    Coding, Simplicity First, Surgical Changes, Goal-Driven Execution);
    estrutura `skills/karpathy-guidelines/` + `.claude-plugin/` +
    `.cursor/rules/` (commit `2c60614`, estável).
  - `mattpocock/skills` — eng. software geral, ativo (commit `abeeaf9`,
    2026-11-18), `.agents/` + marketplace próprio, CI valida frontmatter,
    instala em 9 agentes (Claude/Codex/Copilot/Gemini/Cursor/OpenCode/Devin/
    Windsurf/Amp) e via skills.sh.
  - `chrisbanes/skills` — Android/Kotlin/Compose (commit `edd18ff`,
    2026-11-20), 19 skills, plugins Claude/Codex/OpenCode, CalVer — **triagem
    obrigatória contra §1 regra 1** (Kotlin/Compose proibidos; usar só
    workflow/build/review genéricos).
  - `affaan-m/ECC` — harness de agentes (skills, instincts, memory, security,
    research-first; commit `4adeee6`, 2026-11-20) — instalar seletivas
    (ecc-conventions, ecc-guardrails, ecc-research-playbook), não o repo.
  - `jev-ai/jev-agent-skill` — decisões tipadas do harness (pin `8702f43`,
    2026-09-21; SKILL.md universal instalável com `npx skills add` ou cópia
    para `.agents/skills/jev-agent/SKILL.md`). Validação: repo alcançável,
    formato universal, last change recente. **Condições do gate**: serviço
    externo + `JEV_API_KEY` — a chave vive no ambiente local e NUNCA entra no
    tree (padrão `doc/gitops/SECRETS.md`); status default `inactive` no
    manifest, selecionada por estágio só quando um julgamento do estágio
    precisar (envia estado da sessão a serviço de terceiros e consome
    créditos); endpoint `POST https://thejevai.com/v1/systemone`. Usos no
    fork: guard de tool call destrutivo (`noul`), triagem de itens da
    auditoria (`choice`), verificação de evidência antes de publicar spec
    (`noul`).
- **Why (Por quê):** formato aberto = zero lock-in; seleção por skill evita
  importar conflito; pin = replicabilidade byte-a-byte entre worktrees.
- **Where (Onde):** `local/skills/manifest.yml`; `local/agent-setup/install.sh`
  (bash + git apenas — Termux-compatível, cf. CONTRIBUTING).
- **When (Quando):** setup; e a cada auditoria de skill nova.
- **Who (Quem):** mantenedor; agentes nas máquinas/worktrees.
- **How (Como):** manifest YAML com `id/source/pin/path/status/reason/
  applies_to`; install.sh lê o manifest, baixa (clone raso no pin ou copia do
  tree local), valida frontmatter e instala nos alvos; skills do próprio
  projeto vivem versionadas em `local/skills/<id>/SKILL.md`. Critério:
  install.sh em máquina limpa instala exatamente o que o manifest declara;
  pin divergente do instalado falha.

## [SKILLS-HARNESS::MELHORIA::USABILIDADE::004] Pool inativo e seleção por-spec

- **What (O quê):** cada spec passa a declarar no seu `AGENTS.md` a seção
  "Skills ativas": as globais do projeto + as skills INATIVAS selecionadas do
  pool (do manifest) que servem ao domínio da spec, com o momento de uso
  (ex.: ECC harness-audit durante a revisão; chrisbanes review antes do merge
  Android).
- **Why (Por quê):** ativar tudo globalmente infla o contexto de cada agente e
  aumenta conflito; o pool inativo selecionado por spec dá foco (o princípio
  Karpathy "Simplicity First" aplicado ao próprio harness).
- **Where (Onde):** `AGENTS.md` das 8 specs (`openspec/changes/*/AGENTS.md`);
  `local/skills/manifest.yml` (campo `applies_to` sugere domínios).
- **When (Quando):** início da execução de cada issue/spec.
- **Who (Quem):** o agente que executa a spec (recebe o conjunto exato);
  mantenedor (audita a seleção no PR).
- **How (Como):** a execução da issue skills-harness adiciona a seção aos 8
  AGENTS.md e o RUNBOOK registra a rotina: abrir spec → ler manifest → ativar
  globais + selecionar do pool → registrar a seleção no AGENTS.md da spec.
  Critério: cada AGENTS.md lista ≥ 1 global e justifica cada inativa
  selecionada (ou declara "nenhuma adicional").
