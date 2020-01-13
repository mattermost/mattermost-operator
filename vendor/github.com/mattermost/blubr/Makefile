GO ?= $(shell command -v go 2> /dev/null)

export GO111MODULE=on

## Checks the code style and tests
all: check-style test

## Runs govet and gofmt against all packages.
.PHONY: check-style
check-style: govet lint
	@echo Checking for style guide compliance

## Runs lint against all packages.
.PHONY: lint
lint:
	@echo Running lint
	env GO111MODULE=off $(GO) get -u golang.org/x/lint/golint
	golint -set_exit_status ./...
	@echo lint success

## Runs govet against all packages.
.PHONY: vet
govet:
	@echo Running govet
	$(GO) vet ./...
	@echo Govet success

## Runs go test against all packages.
.PHONY: test
test:
	@echo Running go tests
	go test ./...
	@echo Go test success