GO ?= go
BIN := bin/agent-pwd
export CGO_ENABLED=0

VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X github.com/tyr-zhang/agent-password/internal/build.Version=$(VERSION) \
  -X github.com/tyr-zhang/agent-password/internal/build.Commit=$(COMMIT) \
  -X github.com/tyr-zhang/agent-password/internal/build.Date=$(DATE)

.PHONY: all build test race vet cross docs docs-check i18n-check samples-check helper clean

all: build test

build:
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/agent-pwd

test:
	$(GO) test ./...

race:
	CGO_ENABLED=1 $(GO) test -race ./...

vet:
	$(GO) vet ./...

cross:
	@mkdir -p bin
	@for target in "darwin arm64" "darwin amd64" "linux amd64" "windows amd64"; do \
		set -- $$target; \
		echo "building $$1/$$2"; \
		GOOS=$$1 GOARCH=$$2 $(GO) build -ldflags '$(LDFLAGS)' -o bin/agent-pwd-$$1-$$2 ./cmd/agent-pwd || exit 1; \
	done

docs: build
	@mkdir -p docs
	AGENT_PWD_LANG=en ./$(BIN) docs > docs/contract.md
	AGENT_PWD_LANG=zh-CN ./$(BIN) docs > docs/contract.zh-CN.md

docs-check: build
	@AGENT_PWD_LANG=en ./$(BIN) docs > /tmp/agent-pwd-contract-en.md
	@AGENT_PWD_LANG=zh-CN ./$(BIN) docs > /tmp/agent-pwd-contract-zh.md
	@diff -u docs/contract.md /tmp/agent-pwd-contract-en.md
	@diff -u docs/contract.zh-CN.md /tmp/agent-pwd-contract-zh.md
	@echo "generated docs are in sync"

i18n-check:
	$(GO) test ./internal/i18n ./internal/cli

samples-check:
	@find samples -type f -name '*.sh' -exec bash -n {} +
	@python3 -c "import ast,pathlib; [ast.parse(p.read_text()) for p in pathlib.Path('samples').rglob('*.py')]"
	@echo "sample scripts are syntactically valid"

helper:
	swiftc -O -target arm64-apple-macos11 helpers/touchid/main.swift -o internal/platform/assets/touchid-helper-arm64
	swiftc -O -target x86_64-apple-macos11 helpers/touchid/main.swift -o internal/platform/assets/touchid-helper-amd64

clean:
	rm -rf bin
