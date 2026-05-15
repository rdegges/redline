SHELL := /usr/bin/env bash

VERSION ?= 0.0.0-dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X github.com/rdegges/redline/internal/version.Version=$(VERSION) \
           -X github.com/rdegges/redline/internal/version.Commit=$(COMMIT) \
           -X github.com/rdegges/redline/internal/version.Date=$(DATE)

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "redline Makefile targets:"
	@echo "  build         Compile ./bin/redline"
	@echo "  install       go install the binary"
	@echo "  test          Unit tests (fast)"
	@echo "  test-int      Integration tests (with -tags=integration)"
	@echo "  e2e           End-to-end tests (with -tags=e2e)"
	@echo "  test-ollama   Live Ollama tests (requires OLLAMA_LIVE=1)"
	@echo "  coverage      Coverage report; fails if <80%"
	@echo "  lint          golangci-lint"
	@echo "  fmt           gofmt -s -w"
	@echo "  vuln          govulncheck"
	@echo "  release       goreleaser"
	@echo "  clean         Remove bin/, coverage, *.db"
	@echo "  demo          Run scan against fixture site + fake Ollama"

.PHONY: build
build:
	@mkdir -p bin
	go build -ldflags '$(LDFLAGS)' -o bin/redline ./cmd/redline

.PHONY: install
install:
	go install -ldflags '$(LDFLAGS)' ./cmd/redline

.PHONY: test
test:
	go test -race -count=1 ./...

.PHONY: test-int
test-int:
	go test -race -count=1 -tags=integration ./...

.PHONY: e2e
e2e: build
	go test -race -count=1 -tags=e2e ./e2e/...

.PHONY: test-ollama
test-ollama:
	OLLAMA_LIVE=1 go test -count=1 -tags=ollama_live ./internal/llm/ollama/...

.PHONY: coverage
coverage:
	go test -race -count=1 -tags='integration e2e' \
	    -coverpkg=./internal/... -coverprofile=coverage.out \
	    ./internal/... ./e2e/...
	@total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${total}%"; \
	awk -v t=$${total} 'BEGIN { if (t+0 < 80) { print "coverage below 80%"; exit 1 } }'

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	gofmt -s -w .

.PHONY: vuln
vuln:
	govulncheck ./...

.PHONY: release
release:
	goreleaser release --clean

.PHONY: clean
clean:
	rm -rf bin/ coverage.out coverage.html
	find . -maxdepth 2 -name '*.db' -delete 2>/dev/null || true

.PHONY: demo
demo: build
	./bin/redline scan --site http://127.0.0.1:0 --prompts testdata/prompts/acme-minimal.yaml --dry-run --yes
