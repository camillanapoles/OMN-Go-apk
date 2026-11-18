#!/usr/bin/env bash
# ============================================================================
# acompanha-ci — live view of one Fork CI run. Redraws every 10 seconds
# with the state of each job, the step in progress, and the elapsed time.
# Ends with a Termux notification when the run settles.
# Usage: bash local/cicd/acompanha-ci.sh [run-id|branch]
# ============================================================================
set -uo pipefail
REPO="${REPO:-camillanapoles/OMN-Go-apk}"
ARG="${1:-}"
if [ -z "$ARG" ]; then
  RUN=$(gh run list --repo "$REPO" --workflow "Fork CI" --limit 1 \
    --json databaseId --jq '.[0].databaseId' 2>/dev/null)
else
  case "$ARG" in
    *[!0-9]*)
      RUN=$(gh run list --repo "$REPO" --workflow "Fork CI" --branch "$ARG" \
        --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null) ;;
    *) RUN="$ARG" ;;
  esac
fi
[ -n "${RUN:-}" ] || { echo "no Fork CI run found"; exit 1; }

T0=$(date +%s)
while :; do
  VIEW=$(gh api "repos/$REPO/actions/runs/$RUN/jobs" --jq '
    .jobs[] | [
      .name, .status, (.conclusion // "-"),
      ([.steps[] | select(.status == "in_progress") | .name] | join("/")),
      ((now - (.started_at | fromdateiso8601)) | floor | tostring + "s")
    ] | @tsv' 2>/dev/null || echo "  (run not readable yet)")
  STATUS=$(gh api "repos/$REPO/actions/runs/$RUN" --jq .status 2>/dev/null || echo "?")
  clear
  echo "═══ Fork CI run $RUN · $STATUS · $(date -u +%H:%M:%SZ) ═══"
  printf '%s\n' "$VIEW" | column -t -s "$(printf '\t')" 2>/dev/null || printf '%s\n' "$VIEW"
  [ "$STATUS" = "completed" ] && break
  sleep 10
done

ELAPSED=$(( $(date +%s) - T0 ))
STATE=$(gh run view "$RUN" --repo "$REPO" --json conclusion,updatedAt \
  --jq '.conclusion + " at " + .updatedAt' 2>/dev/null || echo "unknown")
SUMMARY="Run $RUN done: $STATE (watched ${ELAPSED}s)."
termux-notification --title "OMN-Go CI" --content "$SUMMARY" --priority high 2>/dev/null \
  || termux-toast "$SUMMARY" 2>/dev/null || echo "[acompanha] $SUMMARY"
echo "$SUMMARY"
