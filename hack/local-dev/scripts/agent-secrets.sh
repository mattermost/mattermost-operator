#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "ERROR: ANTHROPIC_API_KEY environment variable is not set" >&2
  exit 1
fi

kubectl create secret generic anthropic-key \
  -n "$NAMESPACE" \
  --from-literal=ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret 'anthropic-key' created in namespace '$NAMESPACE'"
