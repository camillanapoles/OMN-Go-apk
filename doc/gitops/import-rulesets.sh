#!/bin/sh
# Import the rulesets of rulesets/ into the repository.
#
# One file of rulesets/ is one ruleset. The script creates a ruleset that
# the repository has not, and it updates one that already exists. Run it
# from anywhere; it needs gh authenticated with Administration (write) on
# the repository.
#
#   REPO=owner/name sh doc/gitops/import-rulesets.sh
set -eu

REPO="${REPO:-camillanapoles/OMN-Go-apk}"
HERE=$(dirname "$0")

for f in "$HERE"/rulesets/*.json; do
	name=$(basename "$f" .json)
	id=$(gh api "repos/$REPO/rulesets" --jq ".[] | select(.name == \"$name\") | .id" || true)
	if [ -n "$id" ]; then
		echo "update $name (id $id)"
		gh api -X PUT "repos/$REPO/rulesets/$id" --input "$f" > /dev/null
	else
		echo "create $name"
		gh api -X POST "repos/$REPO/rulesets" --input "$f" > /dev/null
	fi
done

echo "Done. The active rulesets of $REPO are:"
gh api "repos/$REPO/rulesets" --jq '.[] | "  \(.enforcement)\t\(.name)"'
