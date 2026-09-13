PREFIX ?= $(HOME)/.local
BIN_DIR ?= bin
LDFLAGS ?= -s -w -linkmode external -extldflags -static
CGO_ENABLED ?= 1
GO_TAGS ?= netgo,osusergo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || echo unknown)
VERSION_LDFLAGS := -X github.com/greppleai/grepple/internal/cli.Version=$(VERSION) -X github.com/greppleai/grepple/internal/cli.Commit=$(COMMIT) -X github.com/greppleai/grepple/internal/cli.BuildDate=$(BUILD_DATE)

COMMANDS := grepple
SOURCES := $(shell find cmd internal api extract gritql gritqlapi parser rulespec search -type f -name '*.go') go.mod go.sum
SCHEMA_DIR := .grepple
SCHEMA_CORE_PACKAGES := api extract gritql gritqlapi parser rulespec search
SCHEMA_PACKAGES := $(SCHEMA_CORE_PACKAGES) cli
PACKAGE_BUNDLES := $(addsuffix .package,$(addprefix $(SCHEMA_DIR)/,$(SCHEMA_PACKAGES)))
WORKSPACE_BUNDLES := $(SCHEMA_DIR)/project.workspace

.PHONY: build test agent-benchmark architecture-benchmark lint revive-lint hook-build hook-lint hook-test parser-metadata-generate parser-metadata-check schema-generate schema-check docker-smoke install clean

build: $(addprefix $(BIN_DIR)/,$(COMMANDS)) hook-build

$(BIN_DIR):
	mkdir -p $@

$(BIN_DIR)/grepple: $(SOURCES) | $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags='$(GO_TAGS)' -trimpath -ldflags='$(LDFLAGS) $(VERSION_LDFLAGS)' -o $@ ./cmd/grepple

test: schema-check
	go test ./...
	$(MAKE) hook-test

agent-benchmark:
	go test ./internal/cli -run '^$$' -bench '^BenchmarkAgentWorkflows$$' -benchtime=10x -benchmem

architecture-benchmark:
	go test ./extract -run '^$$' -bench '^BenchmarkArchitectureWorkflows$$' -benchtime=10x -benchmem

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

parser-metadata-generate:
	cd parser && go generate

parser-metadata-check:
	cd parser && go run ./internal/generate -check

schema-generate: parser-metadata-generate $(BIN_DIR)/grepple
	@for package in $(SCHEMA_CORE_PACKAGES); do $(BIN_DIR)/grepple extract structure $$package --bundle --output $(SCHEMA_DIR)/$$package.package || exit $$?; done
	@$(BIN_DIR)/grepple extract structure internal/cli --bundle --output $(SCHEMA_DIR)/cli.package
	@$(BIN_DIR)/grepple extract structure . --workspace --output $(WORKSPACE_BUNDLES)

schema-check: parser-metadata-check $(BIN_DIR)/grepple
	@for bundle in $(PACKAGE_BUNDLES); do $(BIN_DIR)/grepple extract check package $$bundle || exit $$?; done
	@for bundle in $(WORKSPACE_BUNDLES); do $(BIN_DIR)/grepple extract check workspace $$bundle || exit $$?; done

# Opt-in final-image smoke gate; requires a running Docker daemon and is not part of test.
docker-smoke:
	./scripts/docker-smoke.sh

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	for command in $(COMMANDS); do \
		install -m 0755 $(BIN_DIR)/$$command $(DESTDIR)$(PREFIX)/bin/$$command; \
	done

clean:
	rm -rf $(BIN_DIR)
	$(MAKE) -C hooks clean
