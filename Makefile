.PHONY: all check-style unittest generate build clean build-image operator-sdk yaml

OPERATOR_IMAGE ?= mattermost/mattermost-operator:test
SDK_VERSION = v0.17.1
MACHINE = $(shell uname -m)
BUILD_IMAGE = golang:1.14.4
BASE_IMAGE = alpine:3.12
GOROOT ?= $(shell go env GOROOT)
GOPATH ?= $(shell go env GOPATH)
GOFLAGS ?= $(GOFLAGS:) -mod=vendor
GO=go
IMAGE_TAG=
BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
BUILD_HASH := $(shell git rev-parse HEAD)
GO_LINKER_FLAGS ?= -ldflags\
				   "-X 'github.com/mattermost/mattermost-operator/version.buildTime=$(BUILD_TIME)'\
					  -X 'github.com/mattermost/mattermost-operator/version.buildHash=$(BUILD_HASH)'"\

PACKAGES=$(shell go list ./...)
TEST_PACKAGES=$(shell go list ./...| grep -v test/e2e)
INSTALL_YAML=docs/mattermost-operator/mattermost-operator.yaml

all: check-style unittest build ## Run all the things

unittest: ## Runs unit tests
	go test $(GO_LINKER_FLAGS) $(TEST_PACKAGES) -v -covermode=count -coverprofile=coverage.out

build: ## Build the mattermost-operator
	@echo Building Mattermost-operator
	GO111MODULE=on GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -installsuffix cgo -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./cmd/manager/main.go

build-image: operator-sdk ## Build the docker image for mattermost-operator
	@echo Building Mattermost-operator Docker Image
	docker build \
	--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	. -f build/Dockerfile -t $(OPERATOR_IMAGE) \
	--no-cache

check-style: gofmt govet ## Runs govet/gofmt

gofmt: ## Runs gofmt against all packages.
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

govet: ## Runs govet against all packages.
	@echo Running GOVET
	$(GO) get golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow
	$(GO) vet $(GOFLAGS) $(PACKAGES)
	$(GO) vet $(GOFLAGS) -vettool=$(GOPATH)/bin/shadow $(PACKAGES)
	@echo "govet success";

generate: operator-sdk ## Runs the kubernetes code-generators and openapi
	## We have to manually export GOROOT here to get around the following issue:
	## https://github.com/operator-framework/operator-sdk/issues/1854#issuecomment-525132306
	GOROOT=$(GOROOT) build/operator-sdk generate k8s
	build/operator-sdk generate crds

	which ./bin/openapi-gen > /dev/null || GO111MODULE=on go build -o ./bin/openapi-gen k8s.io/kube-openapi/cmd/openapi-gen
	GOROOT=$(GOROOT) ./bin/openapi-gen --logtostderr=true -o "" -i ./pkg/apis/mattermost/v1alpha1 -O zz_generated.openapi -p ./pkg/apis/mattermost/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

	vendor/k8s.io/code-generator/generate-groups.sh all github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/pkg/apis "mattermost:v1alpha1" -h ./hack/boilerplate.go.txt

yaml: ## Generate the YAML file for easy operator installation
	cat deploy/service_account.yaml > $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/crds/mattermost.com_clusterinstallations_crd.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/crds/mattermost.com_mattermostrestoredbs_crd.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/role.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/role_binding.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/operator.yaml >> $(INSTALL_YAML)
	sed -i '' 's/mattermost-operator:test/mattermost-operator:latest/g' ./$(INSTALL_YAML)

operator-sdk: ## Download sdk only if it's not available. Used in the docker build
	build/get-operator-sdk.sh $(SDK_VERSION)

clean: ## Clean up everything
	rm -Rf build/_output
	rm -Rf build/operator-sdk
	go clean $(GOFLAGS) -i ./...
	rm -f *.out
	rm -f *.test


## Help documentatin à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
