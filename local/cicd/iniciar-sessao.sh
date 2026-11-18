#!/usr/bin/env bash
# Session start helper for the fork pipeline. Run it from the repo root.
# A missing remote, a missing file or a missing tool prints a hint, not a
# failure.
set -uo pipefail

echo "== Branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')"
echo "== Last 3 commits:"
git log --oneline -3 2>/dev/null || echo "  (git log failed)"

echo "== Open pull requests (limit 5):"
if command -v gh >/dev/null 2>&1; then
  gh pr list --limit 5 || echo "  (gh pr list failed)"
else
  echo "  (gh not installed)"
fi

echo "== Upstream drift, master..upstream/master:"
if git remote get-url upstream >/dev/null 2>&1; then
  echo "  $(git rev-list --count master..upstream/master 2>/dev/null || echo '?') commit(s) behind, as of the last fetch"
else
  echo "  (no upstream remote. Add it: git remote add upstream https://github.com/mvbasov/OMN-Go.git)"
fi

if [ -f local/cicd/HISTORY.md ]; then
  echo "== Last 12 lines of local/cicd/HISTORY.md:"
  tail -12 local/cicd/HISTORY.md
fi

cat <<'RULES'

== Session loop reminder (R1-R7):
  R1 Spec: write the task spec and the acceptance criteria.
  R2 Branch: cut a feat/fix/refactor/chore/docs branch from dev.
  R3 Push: push the branch to origin.
  R4 CI green loop: iterate until Quality Gate and APK Build pass.
  R5 PR: open the feature PR into dev, then dev into main.
  R6 Auto-merge: let the merge queue land it in main.
  R7 Release: tag main, and the Release workflow publishes.
RULES
