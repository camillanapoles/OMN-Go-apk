#!/usr/bin/env bash
# Install pinned global skills and the selected skills of one spec.
set -euo pipefail

fail() {
  printf 'agent-skills: %s\n' "$*" >&2
  exit 1
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
MANIFEST="$REPO_ROOT/local/skills/manifest.yml"
TARGET_ROOT="$REPO_ROOT/.claude/skills"
SPEC=""
declare -A SPEC_SELECTED=()

usage() {
  cat <<'EOF'
Usage: bash local/agent-setup/install.sh [--spec <change>]

Install global skills from the pinned manifest. With --spec, also install the
inactive skills selected for that OpenSpec change. The selection in its
AGENTS.md must match the manifest. Run without --spec when the change ends to
return to the global set.
EOF
}

case "$#" in
  0) ;;
  1)
    [[ "$1" == "--help" || "$1" == "-h" ]] || { usage >&2; exit 2; }
    usage
    exit 0
    ;;
  2)
    [[ "$1" == "--spec" ]] || { usage >&2; exit 2; }
    SPEC="$2"
    [[ "$SPEC" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] || fail "invalid spec id: $SPEC"
    [[ -f "$REPO_ROOT/openspec/changes/$SPEC/AGENTS.md" ]] || \
      fail "openspec/changes/$SPEC/AGENTS.md does not exist"
    ;;
  *) usage >&2; exit 2 ;;
esac

if [[ -n "$SPEC" ]]; then
  SPEC_AGENTS="$REPO_ROOT/openspec/changes/$SPEC/AGENTS.md"
  spec_skill_ids="$(awk -v spec="$SPEC" '
    function report(message) {
      print "agent-skills: " spec "/AGENTS.md: " message > "/dev/stderr"
      invalid = 1
    }
    /^## Skills ativas[[:space:]]*$/ {
      if (section_seen) report("duplicate Skills ativas section")
      section_seen = 1
      in_section = 1
      next
    }
    in_section && /^## / { in_section = 0 }
    in_section && /^Seleção por spec:/ {
      if (selection_seen) {
        report("duplicate Seleção por spec declaration")
        next
      }
      selection_seen = 1
      line = $0
      sub(/^Seleção por spec:[[:space:]]*/, "", line)
      if (line == "nenhuma adicional.") next
      if (line == "") {
        report("empty Seleção por spec declaration")
        next
      }
      count = split(line, selected, ", ")
      for (i = 1; i <= count; i++) {
        item = selected[i]
        if (i == count && item ~ /\.$/) sub(/\.$/, "", item)
        if (item !~ /^`[a-z0-9]+(-[a-z0-9]+)*`$/) {
          report("use only backticked skill IDs on the Seleção por spec line")
          continue
        }
        sub(/^`/, "", item)
        sub(/`$/, "", item)
        print item
      }
    }
    END {
      if (!section_seen) report("missing ## Skills ativas section")
      if (!selection_seen) report("missing Seleção por spec declaration")
      if (invalid) exit 1
    }
  ' "$SPEC_AGENTS")" || fail "cannot read selection from $SPEC_AGENTS"
  while IFS= read -r id; do
    [[ -n "$id" ]] || continue
    [[ -z "${SPEC_SELECTED[$id]+present}" ]] || fail "$SPEC/AGENTS.md selects $id more than once"
    SPEC_SELECTED["$id"]=1
  done <<< "$spec_skill_ids"
fi

[[ -r "$MANIFEST" ]] || fail "cannot read $MANIFEST"
command -v git >/dev/null 2>&1 || fail "git is required"
if [[ -L "$REPO_ROOT/.claude" || -L "$TARGET_ROOT" ]]; then
  fail ".claude and .claude/skills must not be symlinks"
fi
if [[ -e "$REPO_ROOT/.claude" && ! -d "$REPO_ROOT/.claude" ]]; then
  fail ".claude exists and is not a directory"
fi
if [[ -e "$TARGET_ROOT" && ! -d "$TARGET_ROOT" ]]; then
  fail ".claude/skills exists and is not a directory"
fi

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/omn-go-agent-skills.XXXXXX")" || \
  fail "cannot create a temporary directory"
trap 'rm -rf -- "$TMP_ROOT"' EXIT

# Parse a small, fixed YAML subset. No manifest value is evaluated as code.
awk '
  function error(message) { print "manifest: " message > "/dev/stderr"; invalid = 1; exit 1 }
  function emit(    i) {
    if (id == "") return
    for (i = 1; i <= 6; i++) if (values[i] == "") error("skill " id " misses " names[i])
    printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, values[1], values[2], values[3], values[4], values[5], values[6]
    id = ""
    for (i = 1; i <= 6; i++) values[i] = ""
  }
  BEGIN {
    names[1] = "source"; names[2] = "pin"; names[3] = "path"
    names[4] = "status"; names[5] = "reason"; names[6] = "applies_to"
  }
  $0 == "version: 1" { if (version++) error("duplicate version"); next }
  $0 == "skills:" { if (!version || skills++) error("unexpected skills section"); next }
  /^  - id: / { if (!skills) error("missing skills section"); emit(); id = substr($0, 9); next }
  /^    (source|pin|path|status|reason|applies_to): / {
    if (id == "") error("field outside a skill entry")
    line = substr($0, 5)
    split_at = index(line, ": ")
    key = substr(line, 1, split_at - 1)
    value = substr(line, split_at + 2)
    for (i = 1; i <= 6; i++) if (key == names[i]) break
    if (i > 6) error("unknown field " key)
    if (values[i] != "") error("duplicate field " key " in " id)
    values[i] = value
    next
  }
  /^[[:space:]]*$/ || /^#/ { next }
  { error("unsupported line " NR) }
  END { if (!invalid) { if (!version || !skills) error("version: 1 and skills: are required"); emit() } }
' "$MANIFEST" > "$TMP_ROOT/entries" || fail "cannot parse $MANIFEST"

# Validate each entry before any file is installed.
declare -a IDS=() SOURCES=() PINS=() PATHS=() STATUSES=() APPLIES=()
declare -A SEEN=() SELECTED=() DESIRED_FP=() PAYLOADS=() MANIFEST_STATUS=()
while IFS=$'\t' read -r id source pin path status reason applies_to; do
  [[ "$id" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] || fail "invalid skill id: $id"
  [[ -z "${SEEN[$id]+present}" ]] || fail "duplicate skill id: $id"
  [[ "$source" =~ ^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\.git$ ]] || \
    fail "$id has an unsupported source: $source"
  [[ "$pin" =~ ^[0-9a-f]{40}$ ]] || fail "$id needs a full 40-character commit pin"
  [[ "$path" =~ ^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*\.md$ && "$path" != *'..'* ]] || \
    fail "$id has an invalid source path: $path"
  case "$status" in global|inactive|rejected) ;; *) fail "$id has an invalid status: $status" ;; esac
  [[ -n "$reason" ]] || fail "$id has no audit reason"
  case "$status:$applies_to" in
    global:all|rejected:none|inactive:none) ;;
    inactive:all|global:*|rejected:*) fail "$id has an invalid applies_to value: $applies_to" ;;
    inactive:*)
      IFS='|' read -r -a spec_ids <<< "$applies_to"
      for spec_id in "${spec_ids[@]}"; do
        [[ "$spec_id" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] || fail "$id has an invalid spec id: $spec_id"
      done
      ;;
    *) fail "$id has no valid applies_to value" ;;
  esac
  SEEN["$id"]=1
  MANIFEST_STATUS["$id"]="$status"
  IDS+=("$id")
  SOURCES+=("$source")
  PINS+=("$pin")
  PATHS+=("$path")
  STATUSES+=("$status")
  APPLIES+=("$applies_to")
  if [[ "$status" == global ]]; then
    SELECTED["$id"]=1
  elif [[ "$status" == inactive && -n "$SPEC" ]]; then
    applies_to_spec=no
    declared_by_spec=no
    case "|$applies_to|" in *"|$SPEC|"*) applies_to_spec=yes ;; esac
    [[ -z "${SPEC_SELECTED[$id]+present}" ]] || declared_by_spec=yes
    [[ "$applies_to_spec" == "$declared_by_spec" ]] || \
      fail "$SPEC/AGENTS.md selection and manifest applies_to disagree for $id"
    if [[ "$declared_by_spec" == yes ]]; then
      SELECTED["$id"]=1
    fi
  fi
done < "$TMP_ROOT/entries"
[[ "${#IDS[@]}" -gt 0 ]] || fail "manifest has no skills"
for id in "${!SPEC_SELECTED[@]}"; do
  [[ -n "${SEEN[$id]+present}" ]] || fail "$SPEC/AGENTS.md selects unknown skill $id"
  [[ "${MANIFEST_STATUS[$id]}" == inactive ]] || fail "$SPEC/AGENTS.md selects non-inactive skill $id"
done

validate_frontmatter() {
  local skill_id="$1" file="$2"
  awk -v expected="$skill_id" '
    NR == 1 { if ($0 != "---") exit 1; inside = 1; next }
    inside && $0 == "---" { closed = 1; exit }
    inside && /^name:[[:space:]]*/ { value = $0; sub(/^name:[[:space:]]*/, "", value); if (value == expected) has_name = 1 }
    inside && /^description:[[:space:]]*[^[:space:]]/ { has_description = 1 }
    END { if (!closed || !has_name || !has_description) exit 1 }
  ' "$file" || fail "$file has invalid SKILL.md frontmatter for $skill_id"
}

fingerprint() {
  local output crc bytes
  output="$(cksum "$1")" || fail "cannot fingerprint $1"
  read -r crc bytes _ <<< "$output"
  printf '%s:%s' "$crc" "$bytes"
}

# Download and validate all selected skills before changing the workspace.
mkdir -p "$TMP_ROOT/repos" "$TMP_ROOT/payloads"
for i in "${!IDS[@]}"; do
  id="${IDS[$i]}"
  [[ -n "${SELECTED[$id]+present}" ]] || continue
  source="${SOURCES[$i]}"
  pin="${PINS[$i]}"
  repo_key="${source#https://github.com/}"
  repo_key="${repo_key//\//_}"
  repo_key="${repo_key%.git}"
  repo_dir="$TMP_ROOT/repos/${repo_key}-${pin}"
  if [[ ! -d "$repo_dir/.git" ]]; then
    mkdir -p "$repo_dir"
    git -C "$repo_dir" init -q || fail "cannot initialize $source"
    git -C "$repo_dir" remote add origin "$source" || fail "cannot set source $source"
    git -C "$repo_dir" fetch --quiet --depth=1 origin "$pin" || fail "cannot fetch $source at $pin"
    git -C "$repo_dir" checkout --quiet --detach FETCH_HEAD || fail "cannot check out $source at $pin"
  fi
  [[ "$(git -C "$repo_dir" rev-parse HEAD)" == "$pin" ]] || fail "$source did not resolve to $pin"
  source_file="$repo_dir/${PATHS[$i]}"
  [[ -f "$source_file" && ! -L "$source_file" ]] || \
    fail "pinned source path is missing: $source@$pin:${PATHS[$i]}"
  validate_frontmatter "$id" "$source_file"
  payload="$TMP_ROOT/payloads/$id/SKILL.md"
  mkdir -p "$(dirname -- "$payload")"
  cp -- "$source_file" "$payload"
  PAYLOADS["$id"]="$payload"
  DESIRED_FP["$id"]="$(fingerprint "$payload")"
done

read_marker() {
  local id="$1" directory="$2" marker="$2/.omn-go-skill"
  local magic marker_id source_line pin_line fp_line
  [[ -f "$marker" && ! -L "$marker" ]] || fail "$marker is not a regular installer marker"
  {
    IFS= read -r magic
    IFS= read -r marker_id
    IFS= read -r source_line
    IFS= read -r pin_line
    IFS= read -r fp_line
  } < "$marker" || fail "cannot read $marker"
  [[ "$magic" == "omn-go-agent-skill-v1" && "$marker_id" == "id=$id" ]] || \
    fail "$marker has an unknown format"
  [[ "$source_line" =~ ^source=https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\.git$ ]] || \
    fail "$marker has an invalid source"
  [[ "$pin_line" =~ ^pin=[0-9a-f]{40}$ && "$fp_line" =~ ^fingerprint=[0-9]+:[0-9]+$ ]] || \
    fail "$marker has invalid pin or fingerprint data"
  MARKER_FP="${fp_line#fingerprint=}"
}

write_marker() {
  local id="$1" directory="$2" source="$3" pin="$4" fp="$5"
  local temporary="$directory/.omn-go-skill.$$"
  printf 'omn-go-agent-skill-v1\nid=%s\nsource=%s\npin=%s\nfingerprint=%s\n' \
    "$id" "$source" "$pin" "$fp" > "$temporary"
  chmod 0644 "$temporary"
  mv -f -- "$temporary" "$directory/.omn-go-skill"
}

# Refuse to replace a file that is not ours or was edited after installation.
for i in "${!IDS[@]}"; do
  id="${IDS[$i]}"
  directory="$TARGET_ROOT/$id"
  target="$directory/SKILL.md"
  marker="$directory/.omn-go-skill"
  selected="${SELECTED[$id]+yes}"
  if [[ -L "$directory" ]]; then
    [[ -n "$selected" ]] && fail "$directory is a symlink; refusing to write through it"
    continue
  fi
  [[ ! -e "$directory" || -d "$directory" ]] || fail "$directory is not a directory"
  if [[ -e "$marker" || -L "$marker" ]]; then
    if [[ -z "$selected" && -L "$marker" ]]; then continue; fi
    read_marker "$id" "$directory"
    [[ -f "$target" && ! -L "$target" ]] || fail "$target is missing or is not a regular file"
    [[ "$(fingerprint "$target")" == "$MARKER_FP" ]] || \
      fail "$target changed after installation; refusing to overwrite or remove it"
  elif [[ -n "$selected" && ( -e "$target" || -L "$target" ) ]]; then
    [[ -f "$target" && ! -L "$target" ]] || fail "$target is not a regular file"
    [[ "$(fingerprint "$target")" == "${DESIRED_FP[$id]}" ]] || \
      fail "$target already exists and is not managed; move it before installing"
  fi
done

mkdir -p "$TARGET_ROOT"
for i in "${!IDS[@]}"; do
  id="${IDS[$i]}"
  directory="$TARGET_ROOT/$id"
  target="$directory/SKILL.md"
  marker="$directory/.omn-go-skill"
  source="${SOURCES[$i]}"
  pin="${PINS[$i]}"
  if [[ -n "${SELECTED[$id]+present}" ]]; then
    mkdir -p "$directory"
    if [[ ! -f "$target" ]] || [[ "$(fingerprint "$target")" != "${DESIRED_FP[$id]}" ]]; then
      temporary="$directory/.SKILL.md.$$"
      cp -- "${PAYLOADS[$id]}" "$temporary"
      chmod 0644 "$temporary"
      mv -f -- "$temporary" "$target"
    fi
    write_marker "$id" "$directory" "$source" "$pin" "${DESIRED_FP[$id]}"
  elif [[ "${STATUSES[$i]}" != global && ! -L "$directory" && -d "$directory" && -f "$marker" && ! -L "$marker" ]]; then
    rm -- "$target" "$marker"
    rmdir "$directory" 2>/dev/null || true
  fi
done

if [[ -n "$SPEC" ]]; then
  printf 'agent-skills: installed global and selected skills for %s\n' "$SPEC"
else
  printf 'agent-skills: installed the global skill set\n'
fi
