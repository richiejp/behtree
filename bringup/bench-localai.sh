#!/usr/bin/env bash
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

BASE_URL="http://ledbx.lan:8081"

models=(
    nanbeige4.1-3b-q8
    qwen3-4b
)

for model in "${models[@]}"; do
    echo "=== $model ==="
    go run ./cmd/beht/ benchmark \
        -model "$model" \
        -base-url "$BASE_URL" \
        -provider localai \
        -verbose \
        -trace-dir "/tmp/behtree-traces/$model" \
        -save-trees "/tmp/behtree-trees/$model" \
        "$@"
    echo
done
