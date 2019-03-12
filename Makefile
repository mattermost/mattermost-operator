
GOPATH ?= $(shell go env GOPATH)
GOFLAGS ?= $(GOFLAGS:)
GO=go

BUILDTIME := $(shell date -u +%Y%m%d.%H%M%S)
LDFLAGS ?= -ldflags '-X ${PKG_PATH}/pkg/version.version=${VERSION} -X ${PKG_PATH}/pkg/version.buildtime=${BUILDTIME}'
PACKAGES=$(shell go list ./...)

all: test

# Run tests
test: dep
	go test $(LDFLAGS) $(PACKAGES) -coverprofile cover.out

check-style: gofmt govet

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

# Get dependencies
dep:
	dep ensure -v
