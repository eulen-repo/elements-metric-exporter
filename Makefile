# Copyright 2026 Eulen
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# elements-exporter — Prometheus exporter for Elements (Blockstream) nodes
# Run `make help` for available targets.

BINARY   := elements-exporter
MODULE   := $(shell go list -m)
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GOFLAGS  ?=
LDFLAGS  := -s -w \
            -X main.version=$(VERSION) \
            -X main.commit=$(COMMIT) \
            -X main.date=$(DATE)

# Default: build for the host platform
.DEFAULT_GOAL := build

.PHONY: all build build-linux test test-race test-cover lint vet fmt clean check help

all: check build ## Run checks and build

build: ## Build binary for the host platform
	go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(BINARY) .

build-linux: ## Cross-compile for linux/amd64
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(BINARY)-linux .

test: ## Run unit tests
	go test $(GOFLAGS) -count=1 ./...

test-race: ## Run unit tests with race detector
	go test $(GOFLAGS) -count=1 -race ./...

test-cover: ## Run tests with coverage report
	go test $(GOFLAGS) -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo "To view HTML report: go tool cover -html=coverage.out"

lint: ## Run staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting (exits non-zero if files need gofmt)
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "Run 'gofmt -w .' to fix"; exit 1; }

check: fmt vet test ## Run fmt, vet, and tests

clean: ## Remove build artefacts
	rm -f $(BINARY) $(BINARY)-linux coverage.out

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
