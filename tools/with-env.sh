#!/bin/sh
# tools/with-env.sh -- runs a command with the settings in ./.env added to its
# environment, for `make run` and `make dev`. A variable already set in the
# environment wins over .env, the same order docker compose uses, so an
# exported DB_HOST still points a source run at another database.
#
# .env lines are KEY=VALUE; blank lines and # comments are skipped, and one
# pair of surrounding quotes is removed from a value.
#
# Usage: tools/with-env.sh COMMAND [ARGS...]

set -eu

ENV_FILE="${ENV_FILE:-.env}"

if [ -f "$ENV_FILE" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
        case "$line" in ''|'#'*) continue ;; esac
        case "$line" in *=*) ;; *) continue ;; esac
        key="${line%%=*}"
        val="${line#*=}"
        key="${key#export }"
        case "$key" in ''|[0-9]*|*[!A-Za-z0-9_]*) continue ;; esac
        case "$val" in
            \"*\") val="${val#\"}"; val="${val%\"}" ;;
            \'*\') val="${val#\'}"; val="${val%\'}" ;;
        esac
        # Set means set, even to empty: `DB_PASSWORD=` in the shell is a choice.
        if eval "[ -z \"\${$key+x}\" ]"; then
            export "$key=$val"
        fi
    done < "$ENV_FILE"
fi

exec "$@"
