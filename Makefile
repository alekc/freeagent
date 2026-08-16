GO ?= go
GOTOOLCHAIN ?= auto
export GOTOOLCHAIN

BIN := bin
PKG := ./...

.PHONY: all
all: lint test

.PHONY: build
build:
	$(GO) build $(PKG)

.PHONY: facli
facli:
	$(GO) build -o $(BIN)/facli ./cmd/facli

.PHONY: test
test:
	$(GO) test -race -count=1 $(PKG)

.PHONY: cover
cover:
	$(GO) test -race -count=1 -coverprofile=coverage.txt -covermode=atomic $(PKG)
	$(GO) tool cover -func=coverage.txt | tail -n 1

# Live suite against the FreeAgent sandbox. Never runs in PR CI: it needs real
# credentials and consumes the per-user rate limit. See docs in the README.
#
# Compiled to a stable path rather than run through `go test`, which builds to
# a fresh temp path every time. On machines with an outbound firewall that
# means authorising the binary once instead of on every run.
# Run from the package directory: a compiled test binary inherits the caller's
# working directory, and the fixtures are resolved relative to it.
.PHONY: test-integration
test-integration: $(BIN)/integration.test
	cd freeagent && $(CURDIR)/$(BIN)/integration.test -test.v -test.count=1 -test.run '$(RUN)'

$(BIN)/integration.test: $(wildcard freeagent/*.go)
	$(GO) test -c -tags=integration -o $@ ./freeagent

RUN ?= .

# golangci-lint refuses to analyse a module whose Go version is newer than the
# one it was itself built with, so a system copy will fail on a 1.26 target
# until it catches up. Prefer a locally built one when it is present.
GOLANGCI ?= $(if $(wildcard $(BIN)/golangci-lint),$(BIN)/golangci-lint,golangci-lint)

.PHONY: lint
lint:
	$(GOLANGCI) run

# Build the linter into ./bin so its Go version always matches the module's.
.PHONY: lint-tools
lint-tools:
	GOBIN=$(CURDIR)/$(BIN) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

GOLANGCI_VERSION ?= v2.12.2

.PHONY: fmt
fmt:
	$(GO) fmt $(PKG)
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN) coverage.txt
