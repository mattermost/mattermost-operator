# The Trail: Local Dev Setup (k3d)

Repeatable, idempotent local deployment of the full Trail agent platform stack using k3d.

## Prerequisites

| Tool | Install | Notes |
|------|---------|-------|
| Docker Desktop | [docker.com](https://www.docker.com/products/docker-desktop/) | Must be running. Apple Silicon / arm64 assumed. |
| k3d | `brew install k3d` | Lightweight K8s via k3d. |
| kubectl | `brew install kubectl` | Kubernetes CLI. |
| Go 1.24+ | `brew install go` | For building MM server and operator. |
| python3 | Pre-installed on macOS | Used by validation and admin-secret scripts. |

### Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `ANTHROPIC_API_KEY` | Yes | API key for the demo agent's LLM calls. |

### Worktrees

The following repos must be checked out locally:

| Repo | Default Path | Override |
|------|-------------|----------|
| mattermost-operator | `../../` (this repo) | `OPERATOR_DIR` |
| mattermost-server | `~/workspace/worktrees/mattermost-python-in-plugins/mattermost-python-in-plugins/server` | `MM_SERVER_DIR` |
| the-trail | `~/workspace/the-trail` | `THE_TRAIL_DIR` |

## Quick Start

```bash
cd hack/local-dev

# Full stack from zero (cluster + build + push + deploy):
make all

# Validate everything works end-to-end:
make validate
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make setup` | Create k3d cluster with local registry (idempotent) |
| `make build` | Build all 4 Docker images (mm-server, operator, agent-base, agent) |
| `make push` | Push all images to the k3d registry |
| `make deploy` | Deploy full stack (CNPG, PostgreSQL, secrets, operator, MM, agent) |
| `make validate` | Run e2e validation (posts a message, checks bot reply) |
| `make all` | setup + build + push + deploy |
| `make teardown` | Delete k3d cluster |
| `make clean` | teardown + remove all built Docker images |

### Individual Build/Deploy Targets

You can also run individual steps:

```bash
make build-mm-server   # Just rebuild MM server image
make build-operator    # Just rebuild operator image
make build-agent       # Just rebuild agent image
make push-agent        # Just push agent image
make deploy-mattermost # Just redeploy Mattermost CR
make deploy-agent      # Just redeploy Agent CR
```

## Iterating on Changes

After the initial `make all`, you typically only need to rebuild and redeploy the component you changed:

```bash
# Changed operator code:
make build-operator push-operator deploy-operator

# Changed agent code:
make build-agent push-agent deploy-agent

# Changed MM server code:
make build-mm-server push-mm-server deploy-mattermost
```

## Configuration

All paths and names are configurable via environment variables or Make overrides:

```bash
make all MM_SERVER_DIR=~/my/custom/server/path
make all CLUSTER_NAME=my-cluster REGISTRY_PORT=5001
```

## Troubleshooting

### 1. PVC RWX not supported by k3d local-path provisioner

**Symptom:** PVC bound but pod stuck in `Pending` with `ReadWriteMany` access mode error.

**Cause:** k3d's default `local-path` storage provisioner only supports `ReadWriteOnce`. The Mattermost operator may request RWX for shared file storage.

**Fix:** The `mattermost.yaml` manifest uses `fileStore.local.enabled: true` with a 1Gi volume, which uses RWO. If you see this error, check whether a custom storageClass is being requested and ensure it uses `ReadWriteOnce`.

### 2. MM server binary not in PATH

**Symptom:** `exec: "mattermost": executable file not found in $PATH`

**Cause:** The operator sets `command: ["mattermost"]` on the pod, but the binary lives at `/opt/mattermost/bin/mattermost`.

**Fix:** The Dockerfile in `images/mattermost-server/` creates symlinks: `ln -s /opt/mattermost/bin/mattermost /usr/local/bin/mattermost`. This is already applied.

### 3. mmctl --insecure-skip-verify not available in v7.8.15

**Symptom:** `Error: unknown flag: --insecure-skip-verify` when the operator runs mmctl commands inside the agent base image.

**Cause:** mmctl v7.8.15 (the last standalone release) does not support the `--insecure-skip-verify` flag.

**Fix:** This is handled in the operator code -- the operator conditionally omits the flag. No action needed.

### 4. mmctl home directory permission issue

**Symptom:** `Error: unable to create config directory` or similar permission errors when mmctl runs.

**Cause:** mmctl tries to write a config file to the user's home directory, but the container user may not have a writable home.

**Fix:** The agent base image Dockerfile creates a proper home directory for the `agent` user. No action needed.

### 5. k3d image caching / stale images

**Symptom:** After rebuilding an image, the cluster still runs the old version.

**Cause:** k3d (and containerd) caches images by tag. Pushing `localhost:5000/foo:dev` again doesn't automatically force k3d nodes to re-pull.

**Fix:** The manifests set `imagePullPolicy: Always` to force re-pulls on pod restarts. If a pod is already running, delete it to trigger re-creation:
```bash
kubectl delete pod -l app=mattermost -n mattermost
kubectl delete pod -l app=agent -n mattermost
```

### 6. k3d not installed

**Symptom:** `command not found: k3d`

**Fix:** `brew install k3d`

### General: CNPG port 8000 conflict

Traefik is disabled in the k3d cluster (`--k3s-arg "--disable=traefik@server:*"`) to avoid a port 8000 conflict with the CNPG controller webhook.

### General: sslmode error on Mattermost startup

The `db-secret.sh` script appends `?sslmode=disable` to the connection string. If you see SSL errors, re-run:
```bash
make deploy-db-secret
```

### General: Image not found / ErrImagePull

K8s manifests must use `dev-registry:5000` (the in-cluster registry name), not `localhost:5000`. The `localhost:5000` address only works for pushing from the host machine.

### Checking logs

```bash
# Operator logs
kubectl logs -l control-plane=controller-manager -n mattermost-operator

# Mattermost server logs
kubectl logs -l app=mattermost -n mattermost

# Agent logs
kubectl logs -l app=agent -n mattermost
```
