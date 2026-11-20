#!/usr/bin/env bash
# Cria as 7 issues da auditoria e fecha a rastreabilidade bidirecional
# (número da issue entra em proposal.md e AGENTS.md de cada spec).
#
# Pré-requisito: issues habilitadas no repo —
#   GitHub → Settings → General → Features → Issues (o token desta sessão
#   não tem escopo de admin para isso: HTTP 403).
# Uso: bash openspec/changes/create-issues.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
if ! gh repo view --json hasIssuesEnabled -q .hasIssuesEnabled | grep -q true; then
  echo "ERRO: issues desabilitadas em $REPO. Habilite em Settings -> General -> Features e rode de novo." >&2
  exit 1
fi

DIRS=(audit-skills-harness audit-config audit-backend-core audit-storage-exchange audit-frontend-js audit-android audit-build-ci audit-docs-gov)
TITLES=(
  "[SPEC][SKILLS-HARNESS][PRIORIDADE MÁXIMA] Governança de skills: fontes validadas, install replicável e seleção por-spec"
  "[SPEC][CONFIG] Auditoria e Refatoração: senha admin default e onboarding de identidade"
  "[SPEC][BACKEND-CORE] Auditoria e Refatoração: hardening de login, headers e meta-testes HTTP"
  "[SPEC][STORAGE-EXCHANGE] Auditoria e Refatoração: contrato Go-Java de uploads e sandbox de SVG"
  "[SPEC][FRONTEND-JS] Auditoria e Refatoração: lint anti-innerHTML e divisão de omn-go-api.js"
  "[SPEC][ANDROID] Auditoria e Refatoração: file access do WebView e meta-teste do MainActivity"
  "[SPEC][BUILD-CI] Auditoria e Refatoração: contexto Docker, pins de actions e ruído de CI"
  "[SPEC][DOCS-GOV] Auditoria e Refatoração: docs com Dockerfile.ci morto e codemaps parados"
)
# A primeira issue carrega também o label de prioridade máxima.
DOMS=(dom:skills-harness,priority:high dom:config dom:backend-core dom:storage dom:frontend-js dom:android dom:build-ci dom:docs-gov)

# Labels idempotentes
gh label create "spec:draft" --color 1D76DB --description "Spec OpenSpec aguardando refinamento" 2>/dev/null || true
gh label create "audit" --color FBCA04 --description "Item vindo da auditoria arquitetural" 2>/dev/null || true
gh label create "priority:high" --color B60205 --description "Executar antes das demais (skills-harness)" 2>/dev/null || true
for d in "${DOMS[@]}"; do gh label create "${d%%,*}" --color 0E8A16 --description "Domínio da auditoria arquitetural" 2>/dev/null || true; done

echo "| Tag da Spec | Diretório OpenSpec | AGENTS.md | GitHub Issue # |"
echo "| --- | --- | --- | --- |"
for i in "${!DIRS[@]}"; do
  d="${DIRS[$i]}"
  url=$(gh issue create --title "${TITLES[$i]}" --label "spec:draft,audit,${DOMS[$i]}" --body-file "openspec/changes/$d/issue-body.md")
  num="${url##*/}"
  sed -i "s/> \*\*Issue:\*\* _pendente_/> **Issue:** #${num}/" "openspec/changes/$d/proposal.md"
  sed -i "s/^Issue: _pendente_$/Issue: #${num}/" "openspec/changes/$d/AGENTS.md"
  echo "| \`audit-${d#audit-}\` | openspec/changes/$d/ | openspec/changes/$d/AGENTS.md | #$num |"
done
echo
echo "Rastreabilidade fechada. Commit e push das edições de proposal.md/AGENTS.md:"
echo "  git add openspec && git commit -m 'docs(audit): Link audit specs to issues.' && git push"
