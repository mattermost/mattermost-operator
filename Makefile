.PHONY: all check-style test build clean docker

GOPATH ?= $(shell go env GOPATH)
GOFLAGS ?= $(GOFLAGS:)
GO=go

BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
BUILD_HASH := $(shell git rev-parse HEAD)
GO_LINKER_FLAGS ?= -ldflags\
				   "-X 'github.com/mattermost/mattermost-operator/version.buildTime=$(BUILD_TIME)'\
					  -X 'github.com/mattermost/mattermost-operator/version.buildHash=$(BUILD_HASH)'"\

PACKAGES=$(shell go list ./...)

all: check-style test build

# Run tests
test:
	go test $(GO_LINKER_FLAGS) $(PACKAGES) -coverprofile cover.out

build:
	@echo Building Mattermost-operator
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -gcflags all=-trimpath=$(GOPATH) -asmflags all=-trimpath=$(GOPATH) -a -installsuffix cgo -o build/_output/bin/mattermost-operator $(GO_LINKER_FLAGS) ./cmd/manager/main.go

docker:
	@echo Building Mattermost-operator Docker Image
	docker build . -f build/Dockerfile -t ctadeu/test:latest --no-cache

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

clean:
	rm -Rf build/_output
	go clean $(GOFLAGS) -i ./...
	rm -f *.out
	rm -f *.test
