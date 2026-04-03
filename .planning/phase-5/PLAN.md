# Phase 5: Deploy Manifests + Dev Environment — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/the-trail` (deploy scripts), `~/workspace/worktrees/mattermost-operator-the-trail` (operator)
> **Phase:** 5 of 6
> **Depends on:** Phase 1 (CRD must be generated with `llmGateway` + `mcpServers` fields before `agent.yaml` validates)
> **Goal:** Update the k3d dev environment so LiteLLM is part of the stack. The agent CR uses `spec.llmGateway.operatorManaged` instead of a raw `ANTHROPIC_API_KEY` env var.

---

## Context: Existing Files

All files are in `~/workspace/the-trail/deploy/dev/`. Study these before editing:

- **`agent.yaml`** — Agent CR, namespace `mattermost`, name `langgraph-demo`. Currently uses `egressPolicy: allowList` and injects `ANTHROPIC_API_KEY` directly via `spec.env`. This must change.
- **`agent-secrets.sh`** — Creates K8s Secret `anthropic-key` with key `ANTHROPIC_API_KEY` from env. Must change the secret name and key to match what the operator expects (`apiKey`).
- **`README.md`** — Step-by-step dev setup guide. Must add two new steps: creating LiteLLM secrets and applying the LiteLLM ConfigMap.
- **`setup-cluster.sh`**, **`postgres.yaml`**, **`mattermost.yaml`**, **`admin-secret.sh`**, **`db-secret.sh`**, **`validate.sh`** — do NOT touch these files.

The PostgreSQL cluster (`pg-dev`, CNPG) in `postgres.yaml` only has one database (`mattermost`). LiteLLM needs its own database. Options:
1. Create a second CNPG Cluster for LiteLLM — heavyweight for dev.
2. Use the existing `pg-dev` cluster and create a second database in it — simplest for dev.

**Use option 2**: the `litellm-secrets.sh` script creates the LiteLLM DB connection string pointing at the same `pg-dev` PostgreSQL instance, database name `litellm`, user `mattermost` (same user that already exists on the cluster). This is dev-only — production would use a separate database.

---

## Task 5.1: Create `deploy/dev/litellm-secrets.sh`

**File:** `~/workspace/the-trail/deploy/dev/litellm-secrets.sh`
**Action:** Create (new file)

This script creates two K8s Secrets required by the LiteLLM Deployment:

1. **`litellm-master-key`** — key `masterKey`, value `sk-litellm-dev-master-key`. In dev this is a fixed value for reproducibility. Production would generate a random value.
2. **`litellm-db-credentials`** — key `connectionString`, PostgreSQL DSN for the LiteLLM database.

The DB connection string is built from the existing CNPG secret `pg-dev-app` (same secret used by `db-secret.sh`) but with `dbname=litellm` substituted.

```bash
#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"
CNPG_SECRET="pg-dev-app"

# ── LiteLLM master key ─────────────────────────────────────────────────────
# Fixed value in dev for reproducibility. Production should use a random value:
#   MASTER_KEY="sk-$(openssl rand -hex 16)"
MASTER_KEY="sk-litellm-dev-master-key"

kubectl create secret generic litellm-master-key \
  -n "$NAMESPACE" \
  --from-literal=masterKey="$MASTER_KEY" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret 'litellm-master-key' created in namespace '$NAMESPACE'"

# ── LiteLLM database credentials ──────────────────────────────────────────
# Reuse the pg-dev PostgreSQL instance but with a separate database 'litellm'.
# The mattermost user has superuser privileges on the CNPG cluster so it can
# create the litellm database.

echo "Waiting for CNPG secret ${CNPG_SECRET}..."
until kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" &>/dev/null; do
  sleep 2
done

# Extract the base URI from the existing CNPG secret (points at 'mattermost' db)
URI=$(kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.uri}' | base64 -d)

# Normalise postgresql:// -> postgres:// (LiteLLM uses psycopg2 which requires postgres://)
URI="${URI/postgresql:\/\//postgres://}"

# Replace the database name in the URI with 'litellm'
# URI format: postgres://user:pass@host:port/dbname[?params]
# Use sed to replace the last path component before any '?' with 'litellm'
LITELLM_URI=$(echo "$URI" | sed 's|/[^/?]*\([?].*\)\?$|/litellm|')

# Append sslmode=disable (consistent with db-secret.sh)
if [[ "$LITELLM_URI" == *"?"* ]]; then
  LITELLM_URI="${LITELLM_URI}&sslmode=disable"
else
  LITELLM_URI="${LITELLM_URI}?sslmode=disable"
fi

# Create the litellm database on the pg-dev cluster if it doesn't exist.
# Run psql via a temporary pod using the existing pg-dev-app credentials.
echo "Creating 'litellm' database on pg-dev cluster (if not exists)..."
DB_HOST=$(kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.host}' | base64 -d)
DB_USER=$(kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.user}' | base64 -d)
DB_PASS=$(kubectl get secret "$CNPG_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.password}' | base64 -d)

kubectl run litellm-db-init --rm --restart=Never --image=postgres:16 \
  -n "$NAMESPACE" \
  --env="PGPASSWORD=$DB_PASS" \
  --command -- \
  psql -h "$DB_HOST" -U "$DB_USER" -c "CREATE DATABASE litellm;" 2>/dev/null \
  || echo "  Database 'litellm' may already exist — continuing."

kubectl create secret generic litellm-db-credentials \
  -n "$NAMESPACE" \
  --from-literal=connectionString="$LITELLM_URI" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret 'litellm-db-credentials' created in namespace '$NAMESPACE'"
echo "  Connection string: ${LITELLM_URI//:*@/://<redacted>@}"
```

Make it executable after creating:

```bash
chmod +x ~/workspace/the-trail/deploy/dev/litellm-secrets.sh
```

---

## Task 5.2: Create `deploy/dev/litellm-config.yaml`

**File:** `~/workspace/the-trail/deploy/dev/litellm-config.yaml`
**Action:** Create (new file)

This ConfigMap is applied manually once (not managed by the operator). It contains only `general_settings` — all models are registered via the LiteLLM management API by the operator reconciler. Do NOT add `model_list` here; the spike confirmed that config-file models are lost after restart when `STORE_MODEL_IN_DB=True`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: litellm-config
  namespace: mattermost
  labels:
    app: litellm
data:
  config.yaml: |
    general_settings:
      store_model_in_db: true
```

**Note:** The operator also generates this ConfigMap during reconciliation (via `GenerateLiteLLMConfigMap` from Phase 1). Applying it manually here ensures it exists before the first reconcile and that `kubectl apply` in the README is idempotent (the operator uses the same name/namespace/content so no conflict arises).

---

## Task 5.3: Modify `deploy/dev/agent-secrets.sh`

**File:** `~/workspace/the-trail/deploy/dev/agent-secrets.sh`
**Action:** Modify — change secret name, key name, and secret lookup

Current file creates Secret `anthropic-key` with key `ANTHROPIC_API_KEY`.

The operator now looks for the provider secret specified in `spec.llmGateway.operatorManaged.llmProviders[].secret` with key `apiKey`. The agent CR (Task 5.4) will set `secret: anthropic-key` and the operator reads key `apiKey` from it.

**Change the `--from-literal` key from `ANTHROPIC_API_KEY` to `apiKey`:**

Full new file content:

```bash
#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="mattermost"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "ERROR: ANTHROPIC_API_KEY environment variable is not set" >&2
  exit 1
fi

kubectl create secret generic anthropic-key \
  -n "$NAMESPACE" \
  --from-literal=apiKey="$ANTHROPIC_API_KEY" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret 'anthropic-key' created in namespace '$NAMESPACE'"
```

**Only change:** `--from-literal=ANTHROPIC_API_KEY=` becomes `--from-literal=apiKey=`. The secret name (`anthropic-key`) stays the same — it is referenced by name in `agent.yaml`.

---

## Task 5.4: Modify `deploy/dev/agent.yaml`

**File:** `~/workspace/the-trail/deploy/dev/agent.yaml`
**Action:** Modify — replace `spec.env` ANTHROPIC_API_KEY with `spec.llmGateway.operatorManaged`

Current file uses `egressPolicy: allowList` (because the agent needed direct internet access to reach Anthropic). With LiteLLM as the gateway, the agent only needs to reach the LiteLLM pod — so `egressPolicy: deny` is correct now.

**Full new file content:**

```yaml
apiVersion: installation.mattermost.com/v1beta1
kind: Agent
metadata:
  name: langgraph-demo
  namespace: mattermost
spec:
  image: dev-registry:5000/langgraph-demo:dev
  mattermostRef:
    name: mattermost-dev
  adminCredentialsSecret: mm-admin-token
  egressPolicy: deny
  hooks:
    - MessageHasBeenPosted
  llmGateway:
    operatorManaged:
      llmProviders:
        - name: anthropic
          secret: anthropic-key
          models:
            - claude-3-5-sonnet-20241022
```

**Changes from original:**
1. `egressPolicy: allowList` → `egressPolicy: deny` (agent no longer calls Anthropic directly)
2. Removed `spec.env` block (no more raw `ANTHROPIC_API_KEY` in agent pod)
3. Added `spec.llmGateway.operatorManaged` block

**What the operator will do with this CR:**
- Deploy LiteLLM Deployment + Service + ConfigMap in namespace `mattermost`
- Register model `anthropic/claude-3-5-sonnet-20241022` via `POST /model/new` with `api_key: os.environ/ANTHROPIC_API_KEY`
- Create virtual key Secret `agent-langgraph-demo-litellm-key`
- Inject `LITELLM_BASE_URL`, `OPENAI_BASE_URL`, `ANTHROPIC_BASE_URL`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY` (pointing to virtual key) into agent pod

---

## Task 5.5: Update `deploy/dev/README.md`

**File:** `~/workspace/the-trail/deploy/dev/README.md`
**Action:** Modify — insert two new steps and update step 10

### Change 1: Insert new step after step 4 (create database secret)

Current step 4 is "Create database secret". Insert a new **step 5** ("Create LiteLLM secrets") after it, and renumber all subsequent steps (+1 each).

Insert after the `bash deploy/dev/db-secret.sh` block:

```markdown
### 5. Create LiteLLM secrets

```bash
bash deploy/dev/litellm-secrets.sh
```

This creates two secrets in the `mattermost` namespace:
- `litellm-master-key` — master key for the LiteLLM management API
- `litellm-db-credentials` — PostgreSQL connection string for LiteLLM's own database (`litellm` db on pg-dev)
```

### Change 2: Insert LiteLLM ConfigMap application in the new step 7 (was step 6: build/deploy operator)

After `kubectl rollout status deployment/mattermost-operator ...`, add:

```markdown
# Apply LiteLLM base ConfigMap
kubectl apply -f deploy/dev/litellm-config.yaml
```

### Change 3: Update prerequisites

In the **Prerequisites** section, change:
```
- `ANTHROPIC_API_KEY` environment variable set
```
to:
```
- `ANTHROPIC_API_KEY` environment variable set (used as LiteLLM provider credential, never injected directly into agent pods)
```

### Change 4: Update step 10 (was step 9, now step 11) "Create agent secrets and deploy"

The command `bash deploy/dev/agent-secrets.sh` is unchanged. But add a note below it:

```markdown
> **Note:** The Anthropic API key is stored in Secret `anthropic-key` with key `apiKey`.
> The operator reads it to configure LiteLLM — the agent pod never receives the raw key.
```

### Change 5: Update "Checking logs" section

Add LiteLLM pod logs:

```markdown
# LiteLLM gateway logs
kubectl logs -l app=litellm -n mattermost
```

---

## Task 5.6: Validate the changes

After implementing tasks 5.1–5.5, run these validation checks:

```bash
# 1. Verify the new agent.yaml validates against the generated CRD
# (requires Phase 1 `make generate manifests` to have been run first)
kubectl apply --dry-run=client -f ~/workspace/the-trail/deploy/dev/agent.yaml

# 2. Verify the litellm-config.yaml is valid K8s YAML
kubectl apply --dry-run=client -f ~/workspace/the-trail/deploy/dev/litellm-config.yaml

# 3. Verify litellm-secrets.sh is executable and has no syntax errors
bash -n ~/workspace/the-trail/deploy/dev/litellm-secrets.sh

# 4. Verify agent-secrets.sh has no syntax errors
bash -n ~/workspace/the-trail/deploy/dev/agent-secrets.sh
```

**Expected:** all four commands succeed with no errors.

---

## Definition of Done

- [ ] `deploy/dev/litellm-secrets.sh` exists, is executable (`chmod +x`), creates `litellm-master-key` and `litellm-db-credentials` secrets idempotently
- [ ] `deploy/dev/litellm-config.yaml` exists, contains ConfigMap `litellm-config` in namespace `mattermost` with `general_settings: store_model_in_db: true` only
- [ ] `deploy/dev/agent-secrets.sh` uses `--from-literal=apiKey=` (not `ANTHROPIC_API_KEY=`)
- [ ] `deploy/dev/agent.yaml` uses `spec.llmGateway.operatorManaged` with Anthropic provider, `egressPolicy: deny`, no `spec.env` block
- [ ] `deploy/dev/README.md` has two new steps (5 and ConfigMap apply in step 7), updated prerequisites note, updated agent secrets note, LiteLLM logs section
- [ ] `kubectl apply --dry-run=client` passes for `agent.yaml` and `litellm-config.yaml`
- [ ] `bash -n` passes for both shell scripts

---

## Precise Change Map

| File | Location | Action | Summary |
|------|----------|--------|---------|
| `deploy/dev/litellm-secrets.sh` | New file | Create | Creates `litellm-master-key` and `litellm-db-credentials` secrets; creates `litellm` DB on pg-dev cluster |
| `deploy/dev/litellm-config.yaml` | New file | Create | ConfigMap `litellm-config` with `general_settings` only |
| `deploy/dev/agent-secrets.sh` | Line 13 | Edit | `ANTHROPIC_API_KEY=` → `apiKey=` in `--from-literal` |
| `deploy/dev/agent.yaml` | Entire file | Rewrite | Replace `egressPolicy: allowList` + `spec.env` with `egressPolicy: deny` + `spec.llmGateway.operatorManaged` |
| `deploy/dev/README.md` | Steps 5+, prerequisites, logs | Edit | Add step 5 (litellm-secrets.sh), add ConfigMap apply in step 7, update notes |

---

## Notes for the Implementation Engineer

**Why `egressPolicy: deny` now works:** Previously the agent pod needed `allowList` to reach `api.anthropic.com` directly. Now the agent sends requests to `ANTHROPIC_BASE_URL` which points to LiteLLM (`http://litellm.mattermost.svc.cluster.local:4000/v1`). LiteLLM makes the outbound call to Anthropic. The NetworkPolicy from Phase 1 adds an egress rule allowing agent pods to reach pods with label `app: litellm` on port 4000 — no internet egress needed from the agent pod itself.

**Why `litellm-config.yaml` is applied manually AND generated by the operator:** The manual `kubectl apply` in the README ensures the ConfigMap exists before the first reconcile completes. The operator's `GenerateLiteLLMConfigMap` generates the same content and uses the same name. On subsequent reconciles the operator updates it (no-op since content matches). There is no conflict.

**Database creation in `litellm-secrets.sh`:** The `kubectl run` command to create the database will output an error if the database already exists — this is suppressed with `|| true`. The script is fully idempotent: re-running it updates both secrets and the `CREATE DATABASE` no-ops.

**`litellm-master-key` value in dev:** Using a fixed value (`sk-litellm-dev-master-key`) means the secret stays stable across `kubectl apply` runs and cluster teardowns/recreations don't invalidate any manually issued keys. In production the operator would generate and rotate this.
