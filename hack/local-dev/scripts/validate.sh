#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"
AGENT_NAME="langgraph-demo"
MM_SERVICE="svc/mattermost-dev"
TIMEOUT=60

echo "=== Validation: Full Stack ==="

# --- Prereq: port-forward ---
kubectl port-forward "$MM_SERVICE" 8065:8065 -n "$NAMESPACE" &
PF_PID=$!
sleep 3
cleanup() { kill $PF_PID 2>/dev/null || true; }
trap cleanup EXIT
MM_URL="http://localhost:8065"

# --- Get admin token ---
ADMIN_TOKEN=$(kubectl get secret mm-admin-token -n "$NAMESPACE" -o jsonpath='{.data.token}' | base64 -d)

# --- Step 1: Verify Agent CR is Stable ---
echo "1. Checking Agent CR status..."
STATE=$(kubectl get agent "$AGENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.state}')
if [ "$STATE" != "stable" ]; then
  echo "FAIL: Agent state is '$STATE', expected 'stable'" >&2
  exit 1
fi
echo "   Agent is stable."

# --- Step 2: Get bot username ---
BOT_USERNAME=$(kubectl get agent "$AGENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.botUsername}')
BOT_USER_ID=$(kubectl get agent "$AGENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.botUserID}')
echo "   Bot: $BOT_USERNAME (ID: $BOT_USER_ID)"

# --- Step 3: Create test channel ---
echo "2. Creating test channel..."
TEAM_ID=$(curl -sf "$MM_URL/api/v4/teams/name/default" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

if [ -z "$TEAM_ID" ]; then
  echo "   No 'default' team found. Creating one..."
  TEAM_ID=$(curl -sf -X POST "$MM_URL/api/v4/teams" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{"name":"default","display_name":"Default","type":"O"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
fi

CHANNEL_RESPONSE=$(curl -sf -X POST "$MM_URL/api/v4/channels" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"team_id\":\"$TEAM_ID\",\"name\":\"trail-test\",\"display_name\":\"Trail Test\",\"type\":\"O\"}" 2>/dev/null || true)

CHANNEL_ID=$(echo "$CHANNEL_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

if [ -z "$CHANNEL_ID" ]; then
  # Channel may already exist
  CHANNEL_ID=$(curl -sf "$MM_URL/api/v4/teams/$TEAM_ID/channels/name/trail-test" \
    -H "Authorization: Bearer $ADMIN_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
fi
echo "   Channel ID: $CHANNEL_ID"

# --- Step 4: Invite bot to channel ---
echo "3. Inviting bot to channel..."
curl -sf -X POST "$MM_URL/api/v4/channels/$CHANNEL_ID/members" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$BOT_USER_ID\"}" > /dev/null 2>&1 || true

# --- Step 5: Post a test message ---
echo "4. Posting test message..."
POST_RESPONSE=$(curl -sf -X POST "$MM_URL/api/v4/posts" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"channel_id\":\"$CHANNEL_ID\",\"message\":\"Hello, what is 2+2? Reply with just the number.\"}")
POST_ID=$(echo "$POST_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "   Post ID: $POST_ID"

# --- Step 6: Poll for bot reply ---
echo "5. Waiting for bot reply (timeout: ${TIMEOUT}s)..."
DEADLINE=$((SECONDS + TIMEOUT))
REPLY_FOUND=false
while [ $SECONDS -lt $DEADLINE ]; do
  POSTS=$(curl -sf "$MM_URL/api/v4/channels/$CHANNEL_ID/posts?since=$(date -v-2M +%s000 2>/dev/null || date -d '2 minutes ago' +%s000)" \
    -H "Authorization: Bearer $ADMIN_TOKEN" 2>/dev/null || true)

  # Check if any post is from the bot
  BOT_REPLY=$(echo "$POSTS" | python3 -c "
import sys,json
try:
    data = json.load(sys.stdin)
    for pid, post in data.get('posts', {}).items():
        if post.get('user_id') == '$BOT_USER_ID' and post.get('root_id') == '$POST_ID':
            print(post['message'][:100])
            sys.exit(0)
except: pass
" 2>/dev/null)

  if [ -n "$BOT_REPLY" ]; then
    REPLY_FOUND=true
    break
  fi
  sleep 3
  echo -n "."
done
echo ""

if [ "$REPLY_FOUND" = true ]; then
  echo "   PASS: Bot replied: $BOT_REPLY"
else
  echo "   FAIL: No bot reply within ${TIMEOUT}s" >&2
  echo "   Check agent logs: kubectl logs -l app=agent -n mattermost"
  exit 1
fi

echo ""
echo "=== All validations passed ==="
