GO_ENV := CGO_ENABLED=0

.PHONY: all build test fmt vet lint tidy ci install

all: fmt vet tidy build

build:
	@echo "Building binaries..."
	@$(GO_ENV) go build ./cmd/...

install:
	@$(GO_ENV) go install ./cmd/...

test:
	@$(GO_ENV) go test -v ./...

fmt:
	@gofmt -w -s .

vet:
	@$(GO_ENV) go vet ./...

lint:
	@$(GO_ENV) golangci-lint run ./...

tidy:
	@go mod tidy

ci: fmt vet lint test build
