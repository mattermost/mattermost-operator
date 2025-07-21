# Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
# See LICENSE.txt for license information.

################################################################################
##                             VERSION PARAMS                                 ##
################################################################################

## Tool Versions
VERSION ?= 1.8.0 # Current Operator version - used for bundle
GOLANG_VERSION := $(shell cat go.mod | grep "^go " | cut -d " " -f 2)
SDK_VERSION = v1.0.1

## Docker Build Versions
BUILD_IMAGE = golang:$(GOLANG_VERSION)
BASE_IMAGE = gcr.io/distroless/static:nonroot

## FIPS Docker Build Versions
BUILD_IMAGE_FIPS = cgr.dev/mattermost.com/go-msft-fips:1.24
BASE_IMAGE_FIPS = cgr.dev/mattermost.com/glibc-openssl-fips:15.1

################################################################################

GO ?= $(shell command -v go 2> /dev/null)
PACKAGES=$(shell go list ./...)
TEST_PACKAGES=$(shell go list ./... | grep -v test/e2e)
TEST_FLAGS ?= -v

OPERATOR_IMAGE_NAME ?= mattermost/mattermost-operator
OPERATOR_IMAGE_TAG ?= test
OPERATOR_IMAGE ?= $(OPERATOR_IMAGE_NAME):$(OPERATOR_IMAGE_TAG)

## FIPS Operator Image
OPERATOR_IMAGE_NAME_FIPS ?= mattermost/mattermost-operator-fips
OPERATOR_IMAGE_TAG_FIPS ?= $(OPERATOR_IMAGE_TAG)
OPERATOR_IMAGE_FIPS ?= $(OPERATOR_IMAGE_NAME_FIPS):$(OPERATOR_IMAGE_TAG_FIPS)

MACHINE = $(shell uname -m)
GOFLAGS ?= $(GOFLAGS:)
BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
BUILD_HASH := $(shell git rev-parse HEAD)
TARGET_OS ?= linux
TARGET_ARCH ?= amd64

BUNDLE_IMG ?= controller-bundle:$(VERSION) # Default bundle image tag
CRD_OPTIONS ?= "crd" # Image URL to use all building/pushing image targets

TRIVY_SEVERITY := CRITICAL
TRIVY_EXIT_CODE := 1
TRIVY_VULN_TYPE := os,library

################################################################################

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
	BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
	BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif

BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
	GOBIN=$(shell go env GOPATH)/bin
else
	GOBIN=$(shell go env GOBIN)
endif

GOROOT ?= $(shell go env GOROOT)
GOPATH ?= $(shell go env GOPATH)

GO_LINKER_FLAGS ?= -ldflags\
				   "-X 'github.com/mattermost/mattermost-operator/version.buildTime=$(BUILD_TIME)'\
					  -X 'github.com/mattermost/mattermost-operator/version.buildHash=$(BUILD_HASH)'"\


INSTALL_YAML=docs/mattermost-operator/mattermost-operator.yaml
GO_INSTALL = ./scripts/go_install.sh

KIND_CLUSTER ?= kind
KIND_CONFIG_FILE ?= kind-config-amd64.yaml

# Binaries.
TOOLS_BIN_DIR := $(abspath bin)

SHADOW_BIN := shadow
SHADOW_VER := master
SHADOW_GEN := $(TOOLS_BIN_DIR)/$(SHADOW_BIN)

OPENAPI_VER := release-1.22
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

CONTROLLER_GEN_VER := v0.16.5
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)

KUSTOMIZE_VER := v4.5.7
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)

## --------------------------------------
## Rules
## --------------------------------------

.PHONY: all check-style unittest generate build clean build-image operator-sdk yaml

all: generate check-style unittest build

unittest: ## Runs unit tests
	$(GO) test $(GO_LINKER_FLAGS) $(TEST_PACKAGES) ${TEST_FLAGS} -covermode=count -coverprofile=coverage.out

e2e-local:
	./test/e2e_local.sh

goverall: $(GOVERALLS_GEN) ## Runs goveralls
	$(GOVERALLS_GEN) -coverprofile=coverage.out -service=circle-ci -repotoken ${COVERALLS_REPO_TOKEN} || true

build: ## Build the mattermost-operator
	@echo Building Mattermost-operator
	GO111MODULE=on GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) CGO_ENABLED=0 $(GO) build $(GOFLAGS) -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -installsuffix cgo -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./main.go

.PHONY: buildx-image
buildx-image:  ## Builds and pushes the docker image for mattermost-operator
	@echo Building Mattermost-operator Docker Image
	BUILD_IMAGE=$(BUILD_IMAGE) BASE_IMAGE=$(BASE_IMAGE) OPERATOR_IMAGE=$(OPERATOR_IMAGE) ./scripts/build_image.sh buildx

.PHONY: build-image
build-image:  ## Build the docker image for mattermost-operator
	@echo Building Mattermost-operator Docker Image
	BUILD_IMAGE=$(BUILD_IMAGE) BASE_IMAGE=$(BASE_IMAGE) OPERATOR_IMAGE=$(OPERATOR_IMAGE) ./scripts/build_image.sh local

.PHONY: push-image
push-image: ## Push the docker image using base docker (for local development)
	docker push $(OPERATOR_IMAGE)

## --------------------------------------
## FIPS Build Targets
## --------------------------------------

_build-fips-internal: ## Internal FIPS build target (used by Dockerfile.fips and build-fips)
	@echo "Building Mattermost-operator (FIPS)"
	@mkdir -p build/_output/bin
	GO111MODULE=on GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) CGO_ENABLED=1 $(GO) build -tags=requirefips $(GOFLAGS) -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./main.go

.PHONY: build-fips
build-fips: ## Build the mattermost-operator with FIPS-compliant settings using containerized build
	@echo "Building Mattermost-operator (FIPS - containerized)"
	docker run --rm -v $(shell pwd):/workspace -w /workspace \
		--entrypoint="" \
		-e TARGET_OS=$(TARGET_OS) \
		-e TARGET_ARCH=$(TARGET_ARCH) \
		-e CGO_ENABLED=1 \
		-e GOFIPS=1 \
		-e GOEXPERIMENT=systemcrypto \
		-e HOST_UID=$(shell id -u) \
		-e HOST_GID=$(shell id -g) \
		$(BUILD_IMAGE_FIPS) \
		sh -c "make _build-fips-internal TARGET_OS=\$$TARGET_OS TARGET_ARCH=\$$TARGET_ARCH && mv build/_output/bin/mattermost-operator build/_output/bin/mattermost-operator-fips && chown \$$HOST_UID:\$$HOST_GID build/_output/bin/mattermost-operator-fips"

.PHONY: buildx-image-fips
buildx-image-fips:  ## Builds and pushes the FIPS docker image for mattermost-operator
	@echo Building Mattermost-operator FIPS Docker Image
	BUILD_IMAGE=$(BUILD_IMAGE_FIPS) BASE_IMAGE=$(BASE_IMAGE_FIPS) OPERATOR_IMAGE=$(OPERATOR_IMAGE_FIPS) ./scripts/build_image.sh buildx fips

.PHONY: build-image-fips
build-image-fips:  ## Build the FIPS docker image for mattermost-operator
	@echo Building Mattermost-operator FIPS Docker Image
	BUILD_IMAGE=$(BUILD_IMAGE_FIPS) BASE_IMAGE=$(BASE_IMAGE_FIPS) OPERATOR_IMAGE=$(OPERATOR_IMAGE_FIPS) ./scripts/build_image.sh local fips

.PHONY: push-image-fips
push-image-fips: ## Push the FIPS docker image using base docker (for local development)
	docker push $(OPERATOR_IMAGE_FIPS)

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

yaml: kustomize manifests ## Generate the YAML file for easy operator installation
	cd config/manager && $(KUSTOMIZE) edit set image mattermost-operator="mattermost/mattermost-operator:latest"

	$(KUSTOMIZE) build config/default > $(INSTALL_YAML)

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
	kubectl create ns mattermost-operator --dry-run -oyaml | kubectl apply -f -
	cd config/manager && $(KUSTOMIZE) edit set image mattermost-operator="mattermost/mattermost-operator:test"
	$(KUSTOMIZE) build config/default | kubectl apply -n mattermost-operator -f -

mysql-minio-operators: ## Deploys MinIO and MySQL Operators to the active cluster
	./scripts/install-mysql-minio.sh

manifests: $(CONTROLLER_GEN) ## Runs CRD generator
	echo "Generating CRDs"
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./apis/..." output:crd:artifacts:config=config/crd/bases

fmt: ## Run go fmt against code
	go fmt ./...

vet: ## Run go vet against against all packages.
	@echo Running GOVET
	$(GO) vet $(GOFLAGS) $(PACKAGES)
	$(GO) vet $(GOFLAGS) -vettool=$(SHADOW_GEN) $(PACKAGES)
	@echo "govet success";

generate: $(OPENAPI_GEN) $(CONTROLLER_GEN) ## Runs the kubernetes code-generators and openapi
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

	# Revert any modification to the mysql-operator files
	git checkout pkg/database/mysql_operator/*

	## Grant permissions to execute generation script
	chmod +x scripts/k8s.io/code-generator/generate-groups.sh

	GOROOT=$(GOROOT) $(OPENAPI_GEN) --logtostderr=true -o "" -i ./apis/mattermost/v1alpha1 -O zz_generated.openapi -p ./apis/mattermost/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

	## Do not generate deepcopy as it is handled by controller-gen
	scripts/k8s.io/code-generator/generate-groups.sh client github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" --go-header-file ./hack/boilerplate.go.txt
	scripts/k8s.io/code-generator/generate-groups.sh lister github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" --go-header-file ./hack/boilerplate.go.txt
	scripts/k8s.io/code-generator/generate-groups.sh informer github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/apis "mattermost:v1alpha1" --go-header-file ./hack/boilerplate.go.txt

	GOROOT=$(GOROOT) $(OPENAPI_GEN) --logtostderr=true -o "" -i ./apis/mattermost/v1beta1 -O zz_generated.openapi -p ./apis/mattermost/v1beta1 -h ./hack/boilerplate.go.txt -r "-"

	scripts/k8s.io/code-generator/generate-groups.sh client github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" --go-header-file ./hack/boilerplate.go.txt
	scripts/k8s.io/code-generator/generate-groups.sh lister github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" --go-header-file ./hack/boilerplate.go.txt
	scripts/k8s.io/code-generator/generate-groups.sh informer github.com/mattermost/mattermost-operator/pkg/client/v1beta1 github.com/mattermost/mattermost-operator/apis "mattermost:v1beta1" --go-header-file ./hack/boilerplate.go.txt

kind-start: ## Setup Kind cluster capable of running Mattermost Operator
	KIND_CLUSTER="${KIND_CLUSTER}" KIND_CONFIG_FILE=${KIND_CONFIG_FILE} ./scripts/setup_kind.sh

kind-load-image: ## Loads Mattermost Operator image to Kind cluster
	kind load --name "${KIND_CLUSTER}" docker-image $(OPERATOR_IMAGE)

kind-destroy: ## Destroy Kind cluster
	kind delete cluster --name "${KIND_CLUSTER}"

kustomize: $(KUSTOMIZE)

.PHONY: bundle
bundle: operator-sdk manifests ## Generate bundle manifests and metadata, then validate generated files.
	build/operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	$(KUSTOMIZE) build config/manifests | build/operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	build/operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

## Checks for vulnerabilities
trivy: build-image
	@echo running trivy
	@trivy image --format table --exit-code $(TRIVY_EXIT_CODE) --ignore-unfixed --vuln-type $(TRIVY_VULN_TYPE) --severity $(TRIVY_SEVERITY) $(OPERATOR_IMAGE)

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

$(CONTROLLER_GEN): ## Build controller-gen
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) sigs.k8s.io/controller-tools/cmd/controller-gen $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

$(KUSTOMIZE): ## Build kustomize
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) sigs.k8s.io/kustomize/kustomize/v4 $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)


.PHONY: check-modules
check-modules: $(OUTDATED_GEN) ## Check outdated modules
	@echo Checking outdated modules
	$(GO) list -mod=mod -u -m -json all | $(OUTDATED_GEN) -update -direct

## Help documentatin à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z-][a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
