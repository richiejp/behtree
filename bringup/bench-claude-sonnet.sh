#!/usr/bin/env bash
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

: "${ANTHROPIC_API_KEY:?Set API_KEY to your Anthropic API key}"

go run ./cmd/beht/ benchmark \
    -model claude-sonnet-4-6 \
    -base-url https://api.anthropic.com/v1 \
    -api-key "$ANTHROPIC_API_KEY" \
    -provider openai \
    -verbose \
    -trace-dir /tmp/behtree-traces/claude-sonnet-4-6 \
    -save-trees /tmp/behtree-trees/claude-sonnet-4-6 \
    "$@"
