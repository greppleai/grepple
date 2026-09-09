PREFIX ?= $(HOME)/.local
BIN_DIR ?= bin
LDFLAGS ?= -s -w -linkmode external -extldflags -static
CGO_ENABLED ?= 1
GO_TAGS ?= netgo,osusergo

COMMANDS := grepple
SOURCES := $(shell find cmd internal api parser search -type f -name '*.go') go.mod go.sum
SCHEMA_DIR := .grepple
SCHEMA_CORE_PACKAGES := api parser search
SCHEMA_PACKAGES := $(SCHEMA_CORE_PACKAGES) cli
PACKAGE_BUNDLES := $(addsuffix .package,$(addprefix $(SCHEMA_DIR)/,$(SCHEMA_PACKAGES)))
WORKSPACE_BUNDLES := $(SCHEMA_DIR)/project.workspace

.PHONY: build test lint revive-lint hook-build hook-lint hook-test schema-generate schema-check install clean

build: $(addprefix $(BIN_DIR)/,$(COMMANDS)) hook-build

$(BIN_DIR):
	mkdir -p $@

$(BIN_DIR)/grepple: $(SOURCES) | $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags='$(GO_TAGS)' -trimpath -ldflags='$(LDFLAGS)' -o $@ ./cmd/grepple

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
	@for package in $(SCHEMA_CORE_PACKAGES); do hooks/bin/mermaid-code generate package $$package --format bundle --output $(SCHEMA_DIR)/$$package.package || exit $$?; done
	@hooks/bin/mermaid-code generate package internal/cli --format bundle --output $(SCHEMA_DIR)/cli.package
	@hooks/bin/mermaid-code generate workspace . --output $(WORKSPACE_BUNDLES)

schema-check: hook-build
	@for bundle in $(PACKAGE_BUNDLES); do hooks/bin/mermaid-code check package $$bundle || exit $$?; done
	@for bundle in $(WORKSPACE_BUNDLES); do hooks/bin/mermaid-code check workspace $$bundle || exit $$?; done
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	for command in $(COMMANDS); do \
		install -m 0755 $(BIN_DIR)/$$command $(DESTDIR)$(PREFIX)/bin/$$command; \
	done

clean:
	rm -rf $(BIN_DIR)
	$(MAKE) -C hooks clean
