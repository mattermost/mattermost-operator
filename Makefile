.PHONY: all check-style unittest generate build clean build-image operator-sdk yaml

# Current Operator version - used for bundle
VERSION ?= 1.8.0
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
OPERATOR_IMAGE ?= mattermost/mattermost-operator:test
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SDK_VERSION = v1.0.1
MACHINE = $(shell uname -m)
BUILD_IMAGE = golang:1.14.6
BASE_IMAGE = gcr.io/distroless/static:nonroot
GOROOT ?= $(shell go env GOROOT)
GOPATH ?= $(shell go env GOPATH)
GOFLAGS ?= $(GOFLAGS:) -mod=vendor
GO=go
BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
BUILD_HASH := $(shell git rev-parse HEAD)
GO_LINKER_FLAGS ?= -ldflags\
				   "-X 'github.com/mattermost/mattermost-operator/version.buildTime=$(BUILD_TIME)'\
					  -X 'github.com/mattermost/mattermost-operator/version.buildHash=$(BUILD_HASH)'"\

PACKAGES=$(shell go list ./...)
TEST_PACKAGES=$(shell go list ./...| grep -v test/e2e)
INSTALL_YAML=docs/mattermost-operator/mattermost-operator.yaml
GO_INSTALL = ./scripts/go_install.sh

KIND_CLUSTER ?= "kind"

# Binaries.
TOOLS_BIN_DIR := $(abspath bin)

SHADOW_BIN := shadow
SHADOW_VER := master
SHADOW_GEN := $(TOOLS_BIN_DIR)/$(SHADOW_BIN)

OPENAPI_VER := master
OPENAPI_BIN := openapi-gen
OPENAPI_GEN := $(TOOLS_BIN_DIR)/$(OPENAPI_BIN)

GOVERALLS_VER := master
GOVERALLS_BIN := goveralls
GOVERALLS_GEN := $(TOOLS_BIN_DIR)/$(GOVERALLS_BIN)

OUTDATED_VER := master
OUTDATED_BIN := go-mod-outdated
OUTDATED_GEN := $(TOOLS_BIN_DIR)/$(OUTDATED_BIN)

YQ_VER := master
YQ_BIN := yq
YQ_GEN := $(TOOLS_BIN_DIR)/$(YQ_BIN)

## --------------------------------------
## Rules
## --------------------------------------

all: generate check-style unittest build

unittest: ## Runs unit tests
	$(GO) test -mod=vendor $(GO_LINKER_FLAGS) $(TEST_PACKAGES) -v -covermode=count -coverprofile=coverage.out

goverall: $(GOVERALLS_GEN) ## Runs goveralls
	$(GOVERALLS_GEN) -coverprofile=coverage.out -service=circle-ci -repotoken ${COVERALLS_REPO_TOKEN} || true

build: ## Build the mattermost-operator
	@echo Building Mattermost-operator
	GO111MODULE=on GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -installsuffix cgo -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./main.go

build-image:  ## Build the docker image for mattermost-operator
	@echo Building Mattermost-operator Docker Image
	docker build \
	--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	. -f Dockerfile -t $(OPERATOR_IMAGE) \
	--no-cache

check-style: $(SHADOW_GEN) gofmt vet ## Runs go vet, gofmt

gofmt: ## Validates gofmt against all packages.
	@echo Running GOFMT

	@for package in $(PACKAGES); do \
		echo "Checking "$$package; \
		files=$$(go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}} {{end}}' $$package); \
		if [ "$$files" ]; then \
			gofmt_output=$$(gofmt -d -s $$files 2>&1); \
			if [ "$$gofmt_output" ]; then \
				echo "$$gofmt_output"; \
				echo "gofmt failure"; \
				exit 1; \
			fi; \
		fi; \
	done
	@echo "gofmt success"; \

yaml: $(YQ_GEN) kustomize manifests ## Generate the YAML file for easy operator installation
	cd config/manager && $(KUSTOMIZE) edit set image mattermost-operator="mattermost/mattermost-operator:latest"

	$(KUSTOMIZE) build config/default > $(INSTALL_YAML)

	## Remove "metadata.namespace" keys to allow configuration
	$(YQ_GEN) d -d'*' --inplace $(INSTALL_YAML) metadata.namespace
	echo --- >> $(INSTALL_YAML)

operator-sdk: ## Download sdk only if it's not available. Used when creating bundle.
	build/get-operator-sdk.sh $(SDK_VERSION)

clean: ## Clean up everything
	rm -Rf build/_output
	rm -Rf build/operator-sdk
	go clean $(GOFLAGS) -i ./...
	rm -f *.out
	rm -f *.test
	rm -f bin/*

## -------------------------------------------------------------
## Below - modified rules generated by operator-sdk/kubebuilder
## -------------------------------------------------------------

manager: generate fmt vet ## Build manager binary
	go build -o bin/manager main.go

run: generate fmt vet manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./main.go

install: manifests kustomize ## Install CRDs into a cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from a cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	kubectl create ns mattermost-operator --dry-run=client -oyaml | kubectl apply -f -
	cd config/manager && $(KUSTOMIZE) edit set image mattermost-operator="mattermost/mattermost-operator:test"
	$(KUSTOMIZE) build config/default | kubectl apply -n mattermost-operator -f -

mysql-minio-operators: ## Deploys MinIO and MySQL Operators to the active cluster
	./scripts/install-mysql-minio.sh

manifests: controller-gen ## Runs CRD generator
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=config/crd/bases

fmt: ## Run go fmt against code
	go fmt ./...

vet: ## Run go vet against against all packages.
	@echo Running GOVET
	$(GO) vet $(GOFLAGS) $(PACKAGES)
	$(GO) vet $(GOFLAGS) -vettool=$(SHADOW_GEN) $(PACKAGES)
	@echo "govet success";

generate: $(OPENAPI_GEN) controller-gen ## Runs the kubernetes code-generators and openapi
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

	GOROOT=$(GOROOT) $(OPENAPI_GEN) --logtostderr=true -o "" -i ./apis/mattermost/v1alpha1 -O zz_generated.openapi -p ./apis/mattermost/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

	## Do not generate deepcopy as it is handled by controller-gen
	vendor/k8s.io/code-generator/generate-groups.sh client github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" -h ./hack/boilerplate.go.txt
	vendor/k8s.io/code-generator/generate-groups.sh lister github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" -h ./hack/boilerplate.go.txt
	vendor/k8s.io/code-generator/generate-groups.sh informer github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" -h ./hack/boilerplate.go.txt

	GOROOT=$(GOROOT) $(OPENAPI_GEN) --logtostderr=true -o "" -i ./apis/mattermost/v1beta1 -O zz_generated.openapi -p ./apis/mattermost/v1beta1 -h ./hack/boilerplate.go.txt -r "-"

	vendor/k8s.io/code-generator/generate-groups.sh client github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" -h ./hack/boilerplate.go.txt
	vendor/k8s.io/code-generator/generate-groups.sh lister github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" -h ./hack/boilerplate.go.txt
	vendor/k8s.io/code-generator/generate-groups.sh informer github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" -h ./hack/boilerplate.go.txt

docker-push: ## Push the docker image
	docker push ${OPERATOR_IMAGE}

kind-start: ## Setup Kind cluster capable of running Mattermost Operator
	KIND_CLUSTER="${KIND_CLUSTER}" ./scripts/setup_kind.sh

kind-load-image: ## Loads Mattermost Operator image to Kind cluster
	kind load --name "${KIND_CLUSTER}" docker-image $(OPERATOR_IMAGE)

kind-destroy: ## Destroy Kind cluster
	kind delete cluster --name "${KIND_CLUSTER}"

controller-gen: ## Find or download controller-gen
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: bundle
bundle: operator-sdk manifests ## Generate bundle manifests and metadata, then validate generated files.
	build/operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	$(KUSTOMIZE) build config/manifests | build/operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	build/operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .


## --------------------------------------
## Tooling Binaries
## --------------------------------------

$(SHADOW_GEN): ## Build shadow
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow $(SHADOW_BIN) $(SHADOW_VER)

$(OPENAPI_GEN): ## Build open-api
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) k8s.io/kube-openapi/cmd/openapi-gen $(OPENAPI_BIN) $(OPENAPI_VER)

$(GOVERALLS_GEN): ## Build goveralls
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/mattn/goveralls $(GOVERALLS_BIN) $(GOVERALLS_VER)

$(OUTDATED_GEN): ## Build go-mod-outdated
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/psampaz/go-mod-outdated $(OUTDATED_BIN) $(OUTDATED_VER)

$(YQ_GEN): ## Build yq
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/mikefarah/yq $(YQ_BIN) $(YQ_VER)

.PHONY: check-modules
check-modules: $(OUTDATED_GEN) ## Check outdated modules
	@echo Checking outdated modules
	$(GO) list -mod=mod -u -m -json all | $(OUTDATED_GEN) -update -direct

## Help documentatin à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
