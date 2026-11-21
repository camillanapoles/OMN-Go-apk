# Tasks — audit-skills-harness (PRIORIDADE MÁXIMA — execute primeiro)

## P0 (replicabilidade — fundação dos agentes)

- [ ] Criar `local/skills/manifest.yml` com as 5 fontes validadas + 6 skills
  gitnexus como globais de ferramenta (GAP::GOVERNANCA::001).
  - **Verificação:** toda entry tem `source` + `pin` (commit) + `status` +
    `reason`; skills do projeto existem em `local/skills/<id>/SKILL.md`.
  - **Validação:** `docker`/build inalterados (`local/` fora do contexto);
    `git status` limpo pós-install.
- [ ] Criar `local/agent-setup/install.sh` idempotente (GAP::GOVERNANCA::001).
  - **Verificação:** em diretório limpo, instala exatamente o conjunto global
    do manifest em `.claude/skills/`; segunda execução não duplica (mesmos
    hashes); re-instalar após `rm -rf .claude/skills/` reproduz os mesmos
    hashes.
  - **Validação:** roda em bash 4+/Termux sem dependências além de git+curl;
    nunca escreve em `backend/`, `android/`, `CLAUDE.md`.

## P1 (gate de auditoria — governança executável)

- [ ] Meta-teste `repocheck/agent_skills_test.go` do manifest
  (GAP::INTEROPERABILIDADE::002).
  - **Verificação:** entry global sem `reason`/`pin` → Gate vermelho; pin
    ausente/curto → vermelho; skill listada e ausente do tree → vermelho com
    o caminho.
  - **Validação:** `go vet` verde; nenhum outro meta-teste afetado.
- [ ] Triagem das skills externas contra `CLAUDE.md` §1
  (GAP::INTEROPERABILIDADE::002 — casos chrisbanes e jev).
  - **Verificação:** skills Kotlin/Compose marcadas `inactive` com
    `reason: "conflito §1 regra 1"`; `jev-agent-skill` marcada `inactive` com
    `reason: "serviço externo + API key — opt-in por estágio"`; candidatas
    genéricas (build/review) documentadas.
  - **Validação:** manifest 100% com decisão explícita (global/inativa/
    rejeitada) — zero `status` vazio.

## P2 (seleção por-spec)

- [ ] Adicionar seção "Skills ativas" aos 8 `AGENTS.md` das specs
  (MELHORIA::USABILIDADE::004).
  - **Verificação:** cada AGENTS.md declara globais + seleção do pool
    justificada (ou "nenhuma adicional"); seleção coerente com o domínio.
  - **Validação:** diff limitado a `openspec/**` + `local/**`.
- [ ] Rotina no RUNBOOK (abrir spec → ler manifest → ativar → registrar)
  (MELHORIA::USABILIDADE::004).
  - **Verificação:** RUNBOOK tem seção "Agent skills" com comandos exatos.
  - **Validação:** coordena com docs-gov::002 (mesma rotina de docs).
