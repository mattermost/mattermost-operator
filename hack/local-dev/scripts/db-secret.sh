#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"
CNPG_SECRET="pg-dev-app"
MM_SECRET="mm-db"

echo "Waiting for CNPG secret ${CNPG_SECRET}..."
until kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" &>/dev/null; do
  sleep 2
done

# Read the URI from the CNPG secret
URI=$(kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.uri}' | base64 -d)

# CNPG uses "postgresql://" prefix; Mattermost requires "postgres://"
URI="${URI/postgresql:\/\//postgres://}"

# Append sslmode=disable (CNPG local cluster doesn't use SSL by default,
# and Mattermost will fail to connect without this)
if [[ "$URI" == *"?"* ]]; then
  URI="${URI}&sslmode=disable"
else
  URI="${URI}?sslmode=disable"
fi

echo "Creating Mattermost DB secret..."

# Use --from-literal to avoid trailing newline
# (mattermost-operator issue #342)
kubectl create secret generic "$MM_SECRET" \
  -n "$NAMESPACE" \
  --from-literal=DB_CONNECTION_STRING="$URI" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret ${MM_SECRET} created with DB_CONNECTION_STRING"
