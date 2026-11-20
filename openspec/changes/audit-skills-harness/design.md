# Design — audit-skills-harness

## Stack envolvida

- **Zero impacto no produto**: tudo em `local/` (excluído do Docker context
  por `.dockerignore:7`, nunca embarcado — CLAUDE.md §2).
- bash + git + curl (instalável em Termux — CONTRIBUTING); YAML do manifest
  validado por meta-teste Go (sem dependência: parser mínimo por linha ou
  teste de convenção textual, padrão `repocheck`).
- Formato universal: SKILL.md (frontmatter `name`/`description`) + `AGENTS.md`
  — interoperável com Claude Code, Codex, Cursor, OpenCode, Antigravity e
  skills.sh. **Nenhum lock-in de vendor**: as 4 fontes todas publicam os dois
  formatos ou equivalentes mapeáveis.

## Construção (build/test/lint)

- Quality Gate continua sendo o Fork CI; o novo meta-teste
  `repocheck/agent_skills_test.go` entra no `go test ./backend/...` normal.
- install.sh idempotente: reexecutar não duplica (checagem por hash); nunca
  escreve fora de `.claude/skills/` e `AGENTS.md` (bloco marcado
  `<!-- agent-skills -->`); nunca edita `CLAUDE.md` (contrato vigente é
  intocável por scripts).
- Skills do projeto: versionadas em `local/skills/<id>/SKILL.md`; externas:
  baixadas no pin do manifest (clone raso + checkout do pin) — o repo NÃO
  vendora conteúdo terceiro.
- Commits: `tool(skills): ...` com bump nos 3 arquivos (toca Go no meta-teste).

## Governança

- Merge: 3 checks obrigatórios verdes; PR `cnmfs/*`/`arena/*` →
  `merge/sync-cnmfs`.
- Ordem de execução: **esta issue primeiro** (otimiza os agentes das demais).
- Restrições: nenhuma skill global sem `reason` + pin (meta-teste); skills
  Android do `chrisbanes/skills` entram inativas até triagem contra §1 regra
  1 (Kotlin/Compose proibidos); `CLAUDE.md` e `doc/**` intocáveis; o pool
  nunca é ativado por inteiro (ativar tudo derrota o propósito).
- Princípios de interoperabilidade aplicados: formatos abertos, um tree fonte,
  instalação replicável de qualquer máquina, seleção declarada (auditorável),
  fonte terceira sempre pinada e citada.
