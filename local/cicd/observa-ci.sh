#!/usr/bin/env bash
# ============================================================================
# observa-ci — background observer for one pull request or branch.
# Waits for the checks to settle, then reports by Termux notification.
# It never blocks the session: run it with the async shell.
# Usage: bash local/cicd/observa-ci.sh [pr-number|branch]
# ============================================================================
set -uo pipefail
REPO="${REPO:-camillanapoles/OMN-Go-apk}"
TARGET="${1:-}"
[ -n "$TARGET" ] || TARGET=$(gh pr view --repo "$REPO" --json number --jq .number 2>/dev/null)
[ -n "$TARGET" ] || { echo "usage: observa-ci.sh [pr-number|branch]"; exit 1; }

say() {
  termux-notification --title "OMN-Go CI" --content "$1" --priority high 2>/dev/null \
    || termux-toast "$1" 2>/dev/null || echo "[ci-observer] $1"
}

T0=$(date +%s)
say "Watching $TARGET. I report when the checks settle."

if gh pr checks "$TARGET" --repo "$REPO" --watch >/dev/null 2>&1; then RC=0; else RC=$?; fi
T1=$(date +%s)
ELAPSED=$((T1 - T0))

STATE=$(gh pr view "$TARGET" --repo "$REPO" --json state,mergedAt \
  --jq '"\(.state)\(if .mergedAt then " merged " + .mergedAt else "" end")' 2>/dev/null || echo "state unknown")
SUMMARY="PR $TARGET: $STATE, wait ${ELAPSED}s, watch exit $RC."
say "$SUMMARY"

# One line of evidence for the performance ledger: the duration of the last
# run of each job on this ref.
gh run list --repo "$REPO" --limit 6 \
  --json displayTitle,name,createdAt,updatedAt,status,conclusion \
  --jq '.[] | select(.conclusion != null) | "\(.name): \(.conclusion) \(.createdAt) -> \(.updatedAt)"' \
  2>/dev/null || true
echo "$SUMMARY"
exit $RC
