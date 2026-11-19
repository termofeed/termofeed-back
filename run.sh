#!/usr/bin/env sh
# Inicia o servidor local carregando o .env.
# Variaveis ja definidas no ambiente tem prioridade sobre o .env.
set -eu

cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
    echo "Erro: Go nao encontrado no PATH." >&2
    exit 1
fi

if [ ! -f .env ]; then
    cp .env.example .env
    echo "Arquivo .env criado a partir de .env.example."
fi

cr=$(printf '\r')
while IFS= read -r line || [ -n "$line" ]; do
    line=${line%"$cr"}
    case $line in
        '' | \#*) continue ;;
        *=*) ;;
        *) continue ;;
    esac

    key=${line%%=*}
    value=${line#*=}
    case $key in
        '' | [0-9]* | *[!A-Za-z0-9_]*) continue ;;
    esac

    eval "already_set=\${$key+x}"
    if [ -z "$already_set" ]; then
        export "$key=$value"
    fi
done < .env

exec go run ./cmd/termofeed "$@"
