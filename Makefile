.PHONY: all check-style unittest generate build clean build-image operator-sdk yaml

OPERATOR_IMAGE ?= mattermost/mattermost-operator:test
SDK_VERSION = v0.8.0
MACHINE = $(shell uname -m)
BUILD_IMAGE = golang:1.12
BASE_IMAGE = alpine:3.9
GOPATH ?= $(shell go env GOPATH)
GOFLAGS ?= $(GOFLAGS:)
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
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -installsuffix cgo -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./cmd/manager/main.go

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
	$(GO) vet $(PACKAGES)
	$(GO) vet -vettool=$(GOPATH)/bin/shadow $(PACKAGES)
	@echo "govet success";

generate: ## Runs the kubernetes code-generators and openapi
	operator-sdk generate k8s
	operator-sdk generate openapi
	vendor/k8s.io/code-generator/generate-groups.sh all github.com/mattermost/mattermost-operator/pkg/client github.com/mattermost/mattermost-operator/pkg/apis mattermost:v1alpha1

yaml: ## Generate the YAML file for easy operator installation
	cat deploy/service_account.yaml > $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/crds/mattermost_v1alpha1_clusterinstallation_crd.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/role.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/role_binding.yaml >> $(INSTALL_YAML)
	echo --- >> $(INSTALL_YAML)
	cat deploy/operator.yaml >> $(INSTALL_YAML)


dep: ## Get dependencies
	dep ensure -v

operator-sdk: ## Download sdk only if it's not available. Used in the docker build
	@if [ ! -f build/operator-sdk ]; then \
		curl -Lo build/operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/$(SDK_VERSION)/operator-sdk-$(SDK_VERSION)-$(MACHINE)-linux-gnu && \
		chmod +x build/operator-sdk; \
	fi

clean: ## Clean up everything
	rm -Rf build/_output
	go clean $(GOFLAGS) -i ./...
	rm -f *.out
	rm -f *.test


## Help documentatin à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'