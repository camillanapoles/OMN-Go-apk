#!/usr/bin/env bash
# Gate de release: a release só sai quando UMA spec está completamente
# concluída. O workflow Release roda este script antes de criar a tag,
# e o responsável pode rodá-lo à mão antes do dispatch.
#
# Uso: bash local/release/spec-release-gate.sh <número-da-issue>
#
# Falhou o gate, nada é publicado e a mensagem diz exatamente o que
# falta. Ver doc/decisions/0031-release-after-a-completed-spec.md.

set -euo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)"

ISSUE="${1:-}"
fail() { echo "GATE-RELEASE: FALHOU — $1" >&2; exit 1; }

[ -n "$ISSUE" ] || fail "uso: spec-release-gate.sh <número-da-issue>"
command -v gh >/dev/null 2>&1 || fail "gh não está disponível (o gate lê a issue da spec)"

# 1. A issue da spec está fechada e marcada com o label spec:done.
gh issue view "$ISSUE" --json state >/dev/null 2>&1 \
  || fail "a issue #$ISSUE não existe (confira o número passado no dispatch)"
STATE="$(gh issue view "$ISSUE" --json state -q .state)"
[ "$STATE" = "CLOSED" ] || fail "a issue #$ISSUE está $STATE — conclua a spec e feche a issue antes da release"
gh issue view "$ISSUE" --json labels -q '.labels[].name' | grep -qx "spec:done" \
  || fail "a issue #$ISSUE não tem o label spec:done (adicione ao fechar, com a evidência dos critérios)"

# 2. A spec da issue existe no tree e não tem tarefa pendente.
SPEC_PROPOSAL="$(grep -rl "Issue: #$ISSUE" openspec/changes/*/proposal.md 2>/dev/null | head -1 || true)"
[ -n "$SPEC_PROPOSAL" ] || fail "nenhuma spec em openspec/changes cita a issue #$ISSUE"
SPEC_DIR="$(dirname "$SPEC_PROPOSAL")"
TASKS="$SPEC_DIR/tasks.md"
[ -f "$TASKS" ] || fail "a spec $SPEC_DIR não tem tasks.md"
OPEN="$(grep -c '^[[:space:]]*- \[ \]' "$TASKS" || true)"
[ "$OPEN" = "0" ] || fail "$TASKS tem $OPEN tarefa(s) pendente(s) — conclua antes da release"
DONE="$(grep -c '^[[:space:]]*- \[x\]' "$TASKS" || true)"
[ "$DONE" -ge 1 ] || fail "$TASKS não marca nenhuma tarefa como concluída ([x])"

# 3. Cada tarefa tem os blocos de verificação e validação (M==N).
MISSING="$(awk '
  /^[[:space:]]*- \[/ { if (task != "" && (!ver || !val)) print task; task = $0; ver = 0; val = 0; next }
  /\*\*Verificação:\*\*/ { ver = 1 }
  /\*\*Validação:\*\*/ { val = 1 }
  /^## / { if (task != "" && (!ver || !val)) print task; task = "" }
  END { if (task != "" && (!ver || !val)) print task }
' "$TASKS" || true)"
[ -z "$MISSING" ] || fail "$TASKS tem tarefa sem os critérios Verificação/Validação: $MISSING"

# 4. A versão é a mesma nos três arquivos (regra da seção 6 do CLAUDE.md).
V_GO="$(grep -oP 'APP_VERSION\s*=\s*"\K[^"]+' backend/version.go)" \
  || fail "backend/version.go sem a linha APP_VERSION"
V_GRADLE="$(grep -oP 'versionName "\K[^"]+' android/app/build.gradle | head -1)"
V_MAP="$(grep -oP 'State of the tree: version \K[0-9.]+' doc/CODE_MAP.md)"
[ "$V_GO" = "$V_GRADLE" ] || fail "versão diverge: version.go=$V_GO build.gradle=$V_GRADLE"
[ "$V_GO" = "$V_MAP" ] || fail "versão diverge: version.go=$V_GO doc/CODE_MAP.md=$V_MAP"

# 5. O changelog humanizado da versão existe e é legível para a pessoa.
CHANGELOG="doc/changelog/$V_GO.md"
[ -f "$CHANGELOG" ] || fail "sem $CHANGELOG — o commit que cria a versão escreve o seu changelog"
BULLETS="$(grep -c '^[[:space:]]*•' "$CHANGELOG" || true)"
[ "$BULLETS" -ge 1 ] || fail "$CHANGELOG não tem nenhum bullet •"
BAD_PREFIX="$(grep -nE '^[[:space:]]*[^•#[:space:]<!-]' "$CHANGELOG" | head -3 || true)"
[ -z "$BAD_PREFIX" ] || fail "$CHANGELOG tem linha fora do formato • (regras no ADR 0031): $BAD_PREFIX"
SHORT="$(awk '/^[[:space:]]*•/ && NF < 8 { print NR ": " $0 }' "$CHANGELOG" | head -3 || true)"
[ -z "$SHORT" ] || fail "$CHANGELOG tem bullet com menos de 8 palavras: $SHORT"
SHA="$(grep -ohE '\b[0-9a-f]{7,40}\b' "$CHANGELOG" | grep -E '[a-f]' | head -3 || true)"
[ -z "$SHA" ] || fail "$CHANGELOG cita um hash de commit: $SHA"
JARGON="$(grep -inE '\b(bump|refactor|chore)\b|merge branch' "$CHANGELOG" | head -3 || true)"
[ -z "$JARGON" ] || fail "$CHANGELOG usa palavra de desenvolvedor (regras no ADR 0031): $JARGON"
FILE_REF="$(grep -nE '[A-Za-z0-9_./-]+\.(go|yml|yaml|json|gradle|java|js|sh|md)\b' "$CHANGELOG" | head -3 || true)"
[ -z "$FILE_REF" ] || fail "$CHANGELOG cita nome de arquivo (regras no ADR 0031): $FILE_REF"

echo "GATE-RELEASE: OK — issue #$ISSUE, spec $SPEC_DIR, versão $V_GO, changelog $CHANGELOG"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "## ✅ Gate de release"
    echo ""
    echo "| Prova | Resultado |"
    echo "| --- | --- |"
    echo "| Issue #$ISSUE | fechada com \`spec:done\` |"
    echo "| Tasks | $DONE concluídas, 0 pendentes, todas com Verificação/Validação |"
    echo "| Versão | $V_GO (version.go = build.gradle = CODE_MAP) |"
    echo "| Changelog | humanizado em \`doc/changelog/$V_GO.md\` |"
  } >> "$GITHUB_STEP_SUMMARY"
fi
