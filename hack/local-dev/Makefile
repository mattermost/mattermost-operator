# The Trail — Local Development Setup (k3d)
#
# Usage:
#   make all             — Full setup from zero to working stack
#   make setup           — Create k3d cluster with registry
#   make build           — Build all Docker images
#   make push            — Push all images to k3d registry
#   make deploy          — Deploy full stack
#   make validate        — Run e2e validation
#   make teardown        — Delete k3d cluster
#   make clean           — Teardown + remove built images

SHELL := /bin/bash
.DEFAULT_GOAL := help

# --- Configuration ---
CLUSTER_NAME       ?= dev
REGISTRY_NAME      ?= dev-registry
REGISTRY_PORT      ?= 5000
NAMESPACE          ?= mattermost
OPERATOR_NAMESPACE ?= mattermost-operator
PLATFORM           ?= linux/arm64

# Image names (host-side, for building/pushing)
HOST_REGISTRY   := localhost:$(REGISTRY_PORT)
MM_SERVER_IMAGE := $(HOST_REGISTRY)/mattermost-server:dev
OPERATOR_IMAGE  := $(HOST_REGISTRY)/mattermost-operator:dev
AGENT_BASE_IMAGE := $(HOST_REGISTRY)/agent-base:python
AGENT_IMAGE     := $(HOST_REGISTRY)/langgraph-demo:dev

# In-cluster registry name (used in K8s manifests)
CLUSTER_REGISTRY := $(REGISTRY_NAME):$(REGISTRY_PORT)

# Source paths
OPERATOR_DIR     ?= $(shell cd ../.. && pwd)
MM_SERVER_DIR    ?= $(HOME)/workspace/worktrees/mattermost-python-in-plugins/mattermost-python-in-plugins/server
THE_TRAIL_DIR    ?= $(HOME)/workspace/the-trail

# CNPG version
CNPG_VERSION ?= 1.28.0

# Derived
SCRIPT_DIR := $(shell pwd)/scripts
MANIFEST_DIR := $(shell pwd)/manifests
IMAGE_DIR := $(shell pwd)/images

# --- Top-level targets ---

.PHONY: all
all: setup build push deploy ## Full setup: cluster + build + push + deploy

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Cluster setup ---

.PHONY: setup
setup: ## Create k3d cluster with registry (idempotent)
	@echo "=== Setting up k3d cluster ==="
	@if k3d cluster list $(CLUSTER_NAME) &>/dev/null; then \
		echo "Cluster '$(CLUSTER_NAME)' already exists, skipping creation."; \
	else \
		k3d cluster create $(CLUSTER_NAME) \
			--k3s-arg "--disable=traefik@server:*" \
			--registry-create "$(REGISTRY_NAME):localhost:$(REGISTRY_PORT)"; \
	fi
	@kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@echo "Cluster ready. Registry: localhost:$(REGISTRY_PORT)"

# --- Build images ---

.PHONY: build
build: build-mm-server build-operator build-agent-base build-agent ## Build all Docker images

.PHONY: build-mm-server
build-mm-server: ## Build Mattermost server image
	@echo "=== Building Mattermost server ==="
	cd "$(MM_SERVER_DIR)" && make build-linux-arm64 && make package-linux-arm64
	@TARBALL=$$(ls -1 "$(MM_SERVER_DIR)/dist"/mattermost-*-linux-arm64.tar.gz 2>/dev/null | head -1); \
	if [ -z "$$TARBALL" ]; then \
		echo "ERROR: No linux-arm64 tarball found in $(MM_SERVER_DIR)/dist/" >&2; \
		exit 1; \
	fi; \
	cp "$$TARBALL" "$(IMAGE_DIR)/mattermost-server/mattermost-server.tar.gz"
	docker build --platform $(PLATFORM) \
		-t $(MM_SERVER_IMAGE) \
		$(IMAGE_DIR)/mattermost-server/
	@rm -f "$(IMAGE_DIR)/mattermost-server/mattermost-server.tar.gz"
	@echo "Built: $(MM_SERVER_IMAGE)"

.PHONY: build-operator
build-operator: ## Build operator image
	@echo "=== Building operator ==="
	cd "$(OPERATOR_DIR)" && make build-image
	docker tag mattermost/mattermost-operator:test $(OPERATOR_IMAGE)
	@echo "Built: $(OPERATOR_IMAGE)"

.PHONY: build-agent-base
build-agent-base: ## Build agent base image
	@echo "=== Building agent base image ==="
	docker build --platform $(PLATFORM) \
		-t $(AGENT_BASE_IMAGE) \
		-f "$(THE_TRAIL_DIR)/images/python/Dockerfile" \
		"$(THE_TRAIL_DIR)"
	@echo "Built: $(AGENT_BASE_IMAGE)"

.PHONY: build-agent
build-agent: ## Build agent demo image
	@echo "=== Building agent image ==="
	docker build --platform $(PLATFORM) \
		-t $(AGENT_IMAGE) \
		-f "$(THE_TRAIL_DIR)/agents/langgraph-demo/Dockerfile" \
		"$(THE_TRAIL_DIR)"
	@echo "Built: $(AGENT_IMAGE)"

# --- Push images ---

.PHONY: push
push: push-mm-server push-operator push-agent-base push-agent ## Push all images to k3d registry

.PHONY: push-mm-server
push-mm-server: ## Push Mattermost server image
	docker push $(MM_SERVER_IMAGE)

.PHONY: push-operator
push-operator: ## Push operator image
	docker push $(OPERATOR_IMAGE)

.PHONY: push-agent-base
push-agent-base: ## Push agent base image
	docker push $(AGENT_BASE_IMAGE)

.PHONY: push-agent
push-agent: ## Push agent image
	docker push $(AGENT_IMAGE)

# --- Deploy ---

.PHONY: deploy
deploy: deploy-cnpg deploy-postgres deploy-db-secret deploy-operator deploy-mattermost deploy-admin-token deploy-agent-secrets deploy-agent ## Deploy full stack

.PHONY: deploy-cnpg
deploy-cnpg: ## Install CloudNativePG operator
	@echo "=== Installing CNPG ==="
	kubectl apply --server-side -f \
		https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-$(shell echo $(CNPG_VERSION) | cut -d. -f1-2)/releases/cnpg-$(CNPG_VERSION).yaml
	kubectl wait --for=condition=Available deployment/cnpg-controller-manager \
		-n cnpg-system --timeout=120s
	@echo "CNPG ready."

.PHONY: deploy-postgres
deploy-postgres: ## Deploy PostgreSQL via CNPG
	@echo "=== Deploying PostgreSQL ==="
	kubectl apply -f $(MANIFEST_DIR)/postgres.yaml
	kubectl wait --for=condition=Ready pod/pg-dev-1 -n $(NAMESPACE) --timeout=180s
	@echo "PostgreSQL ready."

.PHONY: deploy-db-secret
deploy-db-secret: ## Create Mattermost DB bridge secret from CNPG secret
	@echo "=== Creating DB bridge secret ==="
	bash $(SCRIPT_DIR)/db-secret.sh

.PHONY: deploy-operator
deploy-operator: ## Deploy the Mattermost operator
	@echo "=== Deploying operator ==="
	cd "$(OPERATOR_DIR)" && make install && make deploy
	kubectl set image deployment/mattermost-operator \
		mattermost-operator=$(CLUSTER_REGISTRY)/mattermost-operator:dev \
		-n $(OPERATOR_NAMESPACE)
	kubectl rollout status deployment/mattermost-operator \
		-n $(OPERATOR_NAMESPACE) --timeout=120s
	@echo "Operator ready."

.PHONY: deploy-mattermost
deploy-mattermost: ## Deploy Mattermost CR
	@echo "=== Deploying Mattermost ==="
	kubectl apply -f $(MANIFEST_DIR)/mattermost.yaml
	@echo "Waiting for Mattermost to become stable..."
	@until [ "$$(kubectl get mm mattermost-dev -n $(NAMESPACE) -o jsonpath='{.status.state}' 2>/dev/null)" = "stable" ]; do \
		sleep 5; echo -n "."; \
	done; echo " Stable!"

.PHONY: deploy-admin-token
deploy-admin-token: ## Create admin user and token secret
	@echo "=== Creating admin token ==="
	bash $(SCRIPT_DIR)/admin-secret.sh

.PHONY: deploy-agent-secrets
deploy-agent-secrets: ## Create agent secrets (requires ANTHROPIC_API_KEY)
	@echo "=== Creating agent secrets ==="
	bash $(SCRIPT_DIR)/agent-secrets.sh

.PHONY: deploy-agent
deploy-agent: ## Deploy Agent CR
	@echo "=== Deploying Agent ==="
	kubectl apply -f $(MANIFEST_DIR)/agent.yaml
	@echo "Waiting for Agent to become stable..."
	@until [ "$$(kubectl get agent langgraph-demo -n $(NAMESPACE) -o jsonpath='{.status.state}' 2>/dev/null)" = "stable" ]; do \
		sleep 5; echo -n "."; \
	done; echo " Stable!"

# --- Validate ---

.PHONY: validate
validate: ## Run e2e validation
	@echo "=== Running validation ==="
	bash $(SCRIPT_DIR)/validate.sh

# --- Teardown ---

.PHONY: teardown
teardown: ## Delete k3d cluster
	@echo "=== Tearing down cluster ==="
	k3d cluster delete $(CLUSTER_NAME) 2>/dev/null || true
	@echo "Cluster deleted."

.PHONY: clean
clean: teardown ## Teardown + remove built images
	@echo "=== Cleaning built images ==="
	-docker rmi $(MM_SERVER_IMAGE) 2>/dev/null
	-docker rmi $(OPERATOR_IMAGE) 2>/dev/null
	-docker rmi $(AGENT_BASE_IMAGE) 2>/dev/null
	-docker rmi $(AGENT_IMAGE) 2>/dev/null
	-docker rmi mattermost/mattermost-operator:test 2>/dev/null
	@echo "Clean complete."
