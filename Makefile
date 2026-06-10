GO_ENV := CGO_ENABLED=0

.PHONY: all build test fmt vet lint tidy ci install

all: fmt vet tidy build

build:
	@echo "Building binaries..."
	@$(GO_ENV) go build ./cmd/...

install:
	@$(GO_ENV) go install ./cmd/...

test:
	@if command -v gotestsum >/dev/null 2>&1; then \
		$(GO_ENV) gotestsum --format dots; \
	else \
		$(GO_ENV) go test ./...; \
	fi

test-verbose:
	@if command -v gotestsum >/dev/null 2>&1; then \
		$(GO_ENV) gotestsum --format standard-verbose; \
	else \
		$(GO_ENV) go test -v ./...; \
	fi

fmt:
	@gofmt -w -s .

vet:
	@$(GO_ENV) go vet ./...

lint:
	@$(GO_ENV) golangci-lint run ./...

tidy:
	@go mod tidy

ci: fmt vet lint test build
