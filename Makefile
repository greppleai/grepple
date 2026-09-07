PREFIX ?= $(HOME)/.local
BIN_DIR ?= bin
LDFLAGS ?= -s -w -linkmode external -extldflags -static
CGO_ENABLED ?= 1
GO_TAGS ?= netgo,osusergo

COMMANDS := grepple shard router
ZOEKT_COMMANDS := zoekt-git-index zoekt-webserver
ZOEKT_VERSION ?= v0.0.0-20260814112500-b0de0bb820f5
SOURCES := $(shell find cmd internal -type f -name '*.go') go.mod go.sum
SCHEMA_DIR := .grepple
STRUCTURE_SCHEMAS := $(wildcard $(SCHEMA_DIR)/*.structure.mmd)
SCHEMA_PACKAGES := api command grepplecli parser repository router search shard
PACKAGE_BUNDLES := $(addsuffix .package,$(addprefix $(SCHEMA_DIR)/,$(SCHEMA_PACKAGES)))
WORKSPACE_BUNDLES := $(SCHEMA_DIR)/project.workspace
SHARD_FLOW_SCHEMA := $(SCHEMA_DIR)/shard-lifecycle.flow.mmd

.PHONY: build test lint revive-lint hook-build hook-lint hook-test schema-generate schema-check install clean zoekt-tools

# zoekt-tools mirrors the throwaway `zoektbuild` module build recipe in the
# Dockerfile's build stage (kept in sync manually) and carries the same four
# `go mod edit -replace` overrides:
#   - github.com/go-git/go-git/v5 -> v5.19.2 (CVE-2026-71556 / CVE-2026-71557)
#   - go.opentelemetry.io/otel/bridge/opentracing -> v1.45.0 (CVE-2026-45404)
#   - golang.org/x/crypto -> v0.55.0 (CVE-2026-56854)
#   - google.golang.org/grpc -> v1.83.1 (CVE-2026-84304)
# See the Dockerfile for full rationale on each; all are removable once
# zoekt's own go.mod requires the patched versions directly.
build: $(addprefix $(BIN_DIR)/,$(COMMANDS)) zoekt-tools hook-build

$(BIN_DIR):
	mkdir -p $@

$(BIN_DIR)/grepple: $(SOURCES) | $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags='$(GO_TAGS)' -trimpath -ldflags='$(LDFLAGS)' -o $@ ./cmd/grepple

$(BIN_DIR)/shard: $(SOURCES) | $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags='$(GO_TAGS)' -trimpath -ldflags='$(LDFLAGS)' -o $@ ./cmd/shard

$(BIN_DIR)/router: $(SOURCES) | $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags='$(GO_TAGS)' -trimpath -ldflags='$(LDFLAGS)' -o $@ ./cmd/router

zoekt-tools: | $(BIN_DIR)
	cd $$(mktemp -d) \
		&& go mod init zoektbuild \
		&& go get github.com/sourcegraph/zoekt@$(ZOEKT_VERSION) \
		&& go mod edit -replace github.com/go-git/go-git/v5=github.com/go-git/go-git/v5@v5.19.2 \
		&& go mod edit -replace go.opentelemetry.io/otel/bridge/opentracing=go.opentelemetry.io/otel/bridge/opentracing@v1.45.0 \
		&& go mod edit -replace golang.org/x/crypto=golang.org/x/crypto@v0.55.0 \
		&& go mod edit -replace google.golang.org/grpc=google.golang.org/grpc@v1.83.1 \
		&& CGO_ENABLED=0 GOFLAGS=-mod=mod GOBIN=$(abspath $(BIN_DIR)) go install -trimpath -ldflags='-s -w' \
			github.com/sourcegraph/zoekt/cmd/zoekt-git-index \
			github.com/sourcegraph/zoekt/cmd/zoekt-webserver

test: schema-check
	go test ./...
	$(MAKE) hook-test

# Example fixtures under examples/ are intentionally excluded from linting.
lint: revive-lint hook-lint hook-test schema-check

revive-lint:
	@command -v revive >/dev/null 2>&1 || { echo "revive not found: go install github.com/mgechev/revive@latest" >&2; exit 1; }
	revive -config revive.toml $(shell go list ./... | grep -v /examples/)

hook-build:
	$(MAKE) -C hooks build

hook-lint:
	$(MAKE) -C hooks lint

hook-test:
	$(MAKE) -C hooks test

schema-generate: hook-build
	@for package in $(SCHEMA_PACKAGES); do hooks/bin/mermaid-code generate package internal/$$package --format bundle --output $(SCHEMA_DIR)/$$package.package || exit $$?; done
	@hooks/bin/mermaid-code generate workspace . --output $(WORKSPACE_BUNDLES)
	@hooks/bin/mermaid-code generate flow internal/shard/shard.go shardImpl.Run --source . --depth 3 --max-nodes 40 --output $(SHARD_FLOW_SCHEMA)

schema-check: hook-build
	@for schema in $(STRUCTURE_SCHEMAS); do hooks/bin/mermaid-code check structure $$schema . || exit $$?; done
	@for bundle in $(PACKAGE_BUNDLES); do hooks/bin/mermaid-code check package $$bundle || exit $$?; done
	@for bundle in $(WORKSPACE_BUNDLES); do hooks/bin/mermaid-code check workspace $$bundle || exit $$?; done
	@tmp=$$(mktemp); trap 'rm -f $$tmp' EXIT; hooks/bin/mermaid-code generate flow internal/shard/shard.go shardImpl.Run --source . --depth 3 --max-nodes 40 --output $$tmp >/dev/null && cmp -s $(SHARD_FLOW_SCHEMA) $$tmp || { echo "generated flow schema differs: run 'make schema-generate'" >&2; exit 1; }
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	for command in $(COMMANDS) $(ZOEKT_COMMANDS); do \
		install -m 0755 $(BIN_DIR)/$$command $(DESTDIR)$(PREFIX)/bin/$$command; \
	done

clean:
	rm -rf $(BIN_DIR)
	$(MAKE) -C hooks clean
