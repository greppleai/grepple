PI_HOOK_BINARY := bin/pi-hook
MODULE_FILES := go.mod go.sum Makefile
ROOT_EXTRACT_SOURCES := $(shell find ../extract -type f -name '*.go') ../go.mod ../go.sum
PI_HOOK_SOURCES := $(shell find cmd/pi-hook internal/pihooks -type f -name '*.go') $(ROOT_EXTRACT_SOURCES) $(MODULE_FILES)
LDFLAGS ?= -s -w
REVIVE_CONFIG ?= ../revive.toml

.PHONY: build test lint clean run-grep-guard run-lint-check

build: $(PI_HOOK_BINARY)

$(PI_HOOK_BINARY): $(PI_HOOK_SOURCES)
	@mkdir -p $(dir $@)
	@go build -trimpath -ldflags='$(LDFLAGS)' -o $@ ./cmd/pi-hook


test:
	@go test ./...

lint:
	@command -v revive >/dev/null 2>&1 || { echo "revive not found: go install github.com/mgechev/revive@latest" >&2; exit 1; }
	@revive -config $(REVIVE_CONFIG) ./...

run-grep-guard: $(PI_HOOK_BINARY)
	@./$(PI_HOOK_BINARY) grep-guard

run-lint-check: $(PI_HOOK_BINARY)
	@./$(PI_HOOK_BINARY) lint-check

clean:
	@rm -rf bin
