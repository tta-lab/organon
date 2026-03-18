.PHONY: all build test fmt vet lint tidy ci install

all: fmt vet tidy build

build:
	@echo "Building binaries..."
	@go build ./cmd/...

install:
	@go install ./cmd/...

test:
	@go test -v ./...

fmt:
	@gofmt -w -s .

vet:
	@go vet ./...

lint:
	@golangci-lint run ./...

tidy:
	@go mod tidy

ci: fmt vet lint test build
