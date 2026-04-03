#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"
MM_SERVICE="svc/mattermost-dev"
ADMIN_USER="admin"
ADMIN_PASS="Admin1234!"
ADMIN_EMAIL="admin@example.com"
SECRET_NAME="mm-admin-token"

echo "=== Port-forwarding to Mattermost ==="
kubectl port-forward "$MM_SERVICE" 8065:8065 -n "$NAMESPACE" &
PF_PID=$!
sleep 3

cleanup() {
  kill $PF_PID 2>/dev/null || true
}
trap cleanup EXIT

MM_URL="http://localhost:8065"

echo "=== Creating admin user ==="
# Create initial admin user (will fail if already exists -- that's OK)
curl -sf -X POST "$MM_URL/api/v4/users" \
  -H 'Content-Type: application/json' \
  -d "{
    \"email\": \"$ADMIN_EMAIL\",
    \"username\": \"$ADMIN_USER\",
    \"password\": \"$ADMIN_PASS\"
  }" > /dev/null 2>&1 || echo "Admin user may already exist, continuing..."

echo "=== Logging in ==="
LOGIN_RESPONSE=$(curl -sf -X POST "$MM_URL/api/v4/users/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login_id\": \"$ADMIN_USER\", \"password\": \"$ADMIN_PASS\"}" \
  -D - 2>/dev/null)

TOKEN=$(echo "$LOGIN_RESPONSE" | grep -i '^token:' | awk '{print $2}' | tr -d '\r\n')

if [ -z "$TOKEN" ]; then
  echo "ERROR: Failed to get login token" >&2
  echo "$LOGIN_RESPONSE" >&2
  exit 1
fi

echo "=== Creating personal access token ==="
PAT_RESPONSE=$(curl -sf -X POST "$MM_URL/api/v4/users/me/tokens" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"description": "operator admin token"}')

PAT=$(echo "$PAT_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

if [ -z "$PAT" ]; then
  echo "ERROR: Failed to create personal access token" >&2
  echo "$PAT_RESPONSE" >&2
  exit 1
fi

echo "=== Creating K8s secret ==="
kubectl create secret generic "$SECRET_NAME" \
  -n "$NAMESPACE" \
  --from-literal=token="$PAT" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Done! Secret '$SECRET_NAME' created with admin access token."
echo "The Agent CR should reference: adminCredentialsSecret: $SECRET_NAME"
