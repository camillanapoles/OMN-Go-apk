#!/usr/bin/env bash
# local/agent-setup/install.sh — materializa as skills do manifest.
#
# Uso:
#   bash local/agent-setup/install.sh              # instala as globais
#   bash local/agent-setup/install.sh <id> [<id>]  # globais + seleção do pool
#
# Fonte da verdade: local/skills/manifest.yml (abrir para as regras).
# Uma máquina nova fica completa com: git clone && bash local/agent-setup/install.sh.
#
# Comportamento:
#   - Instala exatamente o conjunto global do manifest em .claude/skills/.
#   - Reexecutar não duplica: o conteúdo é comparado por hash (git hash-object).
#   - rm -rf .claude/skills && re-instalar reproduz os mesmos hashes.
#   - Escreve só em .claude/skills/. Nunca toca em backend/, android/ ou CLAUDE.md.
#   - Fonte externa: clone raso no pin do manifest + cópia do SKILL.md.
#     Requer só bash, git e curl. Funciona em Termux.
#
# Saída: 0 em sucesso. 1 com a causa escrita em stderr.

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
MANIFEST="$ROOT/local/skills/manifest.yml"
TARGET="$ROOT/.claude/skills"

err() {
    echo "install.sh: $*" >&2
    exit 1
}

[ -f "$MANIFEST" ] || err "manifest não encontrado: $MANIFEST"

# ---- leitura do manifest (formato em blocos, ver o cabeçalho do arquivo) ----
declare -a M_ID=() M_SOURCE=() M_PIN=() M_PATH=() M_STATUS=() M_REASON=()
cur=-1
while IFS= read -r line || [ -n "$line" ]; do
    # remove o fim de linha do Windows e espaços das bordas
    line=${line%$'\r'}
    trimmed=$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    case "$trimmed" in
        ''|'#'*|'skills:') continue ;;
        '- id:'*)
            M_ID+=("${trimmed#- id: }")
            M_SOURCE+=("") M_PIN+=("") M_PATH+=("") M_STATUS+=("") M_REASON+=("")
            cur=$((${#M_ID[@]} - 1))
            ;;
        'source:'*|'pin:'*|'path:'*|'status:'*|'reason:'*|'applies_to:'*)
            [ "$cur" -ge 0 ] || err "campo antes de - id: no manifest: $trimmed"
            case "$trimmed" in
                'source:'*) M_SOURCE[$cur]=${trimmed#source: } ;;
                'pin:'*)    M_PIN[$cur]=${trimmed#pin: } ;;
                'path:'*)   M_PATH[$cur]=${trimmed#path: } ;;
                'status:'*) M_STATUS[$cur]=${trimmed#status: } ;;
                'reason:'*) M_REASON[$cur]=${trimmed#reason: } ;;
            esac
            ;;
        *) err "linha ilegível no manifest: $trimmed" ;;
    esac
done < "$MANIFEST"

[ "$cur" -ge 0 ] || err "manifest sem entradas"

# remove as aspas dos valores
strip_quotes() {
    local v=$1
    v=${v#\"}
    v=${v%\"}
    echo "$v"
}

idx_of() {
    local want=$1 i
    for i in "${!M_ID[@]}"; do
        if [ "${M_ID[$i]}" = "$want" ]; then
            echo "$i"
            return 0
        fi
    done
    return 1
}

# ---- conjunto alvo: as globais + os ids pedidos na linha de comando ----
declare -a TARGET_IDS=()
for i in "${!M_ID[@]}"; do
    if [ "${M_STATUS[$i]}" = "global" ]; then
        TARGET_IDS+=("${M_ID[$i]}")
    fi
done
for want in "$@"; do
    i=$(idx_of "$want") || err "id desconhecido no manifest: $want"
    [ "${M_STATUS[$i]}" != "rejected" ] || err "id rejeitado no manifest: $want"
    already=no
    for t in "${TARGET_IDS[@]}"; do
        if [ "$t" = "$want" ]; then
            already=yes
        fi
    done
    if [ "$already" = "no" ]; then
        TARGET_IDS+=("$want")
    fi
done

[ "${#TARGET_IDS[@]}" -gt 0 ] || err "conjunto alvo vazio"

# ---- materialização de uma skill ----
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

frontmatter_ok() {
    local f=$1
    awk '
        NR == 1 { if ($0 != "---") exit 1; next }
        /^name:/ { name = 1 }
        /^description:/ { desc = 1 }
        $0 == "---" { closed = 1; exit }
        END { exit (closed && name && desc) ? 0 : 1 }
    ' "$f"
}

installed=0
updated=0
same=0
for id in "${TARGET_IDS[@]}"; do
    i=$(idx_of "$id")
    src_path=${M_PATH[$i]}
    src_path=$(strip_quotes "$src_path")
    source=$(strip_quotes "${M_SOURCE[$i]}")
    pin=$(strip_quotes "${M_PIN[$i]}")

    case "$src_path" in
        local/*)
            src_file="$ROOT/$src_path"
            [ -f "$src_file" ] || err "$id: skill do tree ausente: $src_path"
            ;;
        -)
            err "$id: entrada sem SKILL.md instalável (path '-')"
            ;;
        *)
            case "$source" in
                */*) : ;;
                *) err "$id: fonte externa inválida: $source" ;;
            esac
            clone="$workdir/$id"
            git init -q "$clone"
            git -C "$clone" remote add origin "https://github.com/$source"
            if git -C "$clone" fetch -q --depth 1 origin "$pin" 2>/dev/null; then
                git -C "$clone" checkout -q FETCH_HEAD
            else
                # pin não é buscável direto: só o tip da fonte serve, e
                # só com igualdade exata de hash (fidelidade do pin)
                git -C "$clone" fetch -q --depth 1 origin HEAD
                git -C "$clone" checkout -q FETCH_HEAD
                got=$(git -C "$clone" rev-parse HEAD)
                [ "$got" = "$pin" ] || err "$id: pin $pin não alcançável em $source (tip é $got)"
            fi
            src_file="$clone/$src_path"
            [ -f "$src_file" ] || err "$id: skill ausente no pin $pin: $src_path"
            ;;
    esac

    frontmatter_ok "$src_file" || err "$id: frontmatter sem name/description em $src_path"

    dest_dir="$TARGET/$id"
    dest="$dest_dir/SKILL.md"
    new_hash=$(git hash-object "$src_file")
    if [ -f "$dest" ]; then
        old_hash=$(git hash-object "$dest")
        if [ "$old_hash" = "$new_hash" ]; then
            same=$((same + 1))
            echo "igual     $id"
            continue
        fi
        mkdir -p "$dest_dir"
        cp "$src_file" "$dest"
        updated=$((updated + 1))
        echo "atualizado $id"
    else
        mkdir -p "$dest_dir"
        cp "$src_file" "$dest"
        installed=$((installed + 1))
        echo "instalado $id"
    fi
done

echo "install.sh: $installed instalada(s), $updated atualizada(s), $same inalterada(s)."
