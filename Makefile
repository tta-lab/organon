GO_ENV := CGO_ENABLED=0

.PHONY: all build test fmt vet lint tidy ci ci-scope install

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

ci-scope:
	@test -n "$(SCOPE_CMD)" || (echo "SCOPE_CMD is required (for example: web)" >&2; exit 2)
	@test -n "$(SCOPE_PACKAGES)" || (echo "SCOPE_PACKAGES is required" >&2; exit 2)
	@files="$$(for dir in $$($(GO_ENV) go list -f '{{.Dir}}' $(SCOPE_PACKAGES)); do rg --files "$$dir" -g '*.go'; done)"; \
		unformatted="$$(gofmt -l -s $$files)"; \
		if [ -n "$$unformatted" ]; then \
			echo "Go files need formatting:" >&2; \
			echo "$$unformatted" >&2; \
			exit 1; \
		fi
	@$(GO_ENV) go vet $(SCOPE_PACKAGES)
	@$(GO_ENV) golangci-lint run $(SCOPE_PACKAGES)
	@if command -v gotestsum >/dev/null 2>&1; then \
		$(GO_ENV) gotestsum --format dots -- $(SCOPE_PACKAGES); \
	else \
		$(GO_ENV) go test $(SCOPE_PACKAGES); \
	fi
	@$(GO_ENV) go build ./cmd/$(SCOPE_CMD)
