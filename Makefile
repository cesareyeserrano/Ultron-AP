BINARY_NAME=ultron-ap
BUILD_DIR=bin
GO=go
TAILWIND=./tailwindcss

# Build metadata baked into the binary via ldflags so /version and the
# startup log line know what commit they came from. (BL-021 / BG-033.)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
VERSION_PKG=github.com/cesareyeserrano/ultron-ap/internal/server
LDFLAGS=-ldflags "-X '$(VERSION_PKG).BuildCommit=$(COMMIT)'"

# Pi deployment defaults — override on the command line when targeting
# a different host, e.g. `make deploy-verify PI_HOST=user@host`.
PI_HOST ?= cesareyeserrano@192.168.1.29
PI_BIN  ?= /opt/ultron-ap/ultron-ap
PI_HELPER ?= /opt/ultron-ap/ultron-helper

.PHONY: build build-arm test clean run fmt vet css css-watch deploy-verify

build: css
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ultron-ap/
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/ultron-helper ./cmd/ultron-helper/

build-arm: css
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/ultron-ap/
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/ultron-helper-linux-arm64 ./cmd/ultron-helper/

test:
	$(GO) test ./... -v

clean:
	rm -rf $(BUILD_DIR)

run: build
	$(BUILD_DIR)/$(BINARY_NAME)

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# app.css is a committed build artifact; rebuild after editing web/css/input.css.
# If the ./tailwindcss standalone binary is missing, the v4 CLI moved to npm:
#   npm install --no-save --prefix /tmp/twbuild tailwindcss@4.1.18 @tailwindcss/cli@4.1.18
#   ln -sfn /tmp/twbuild/node_modules ./node_modules   # resolver needs it near web/css
#   /tmp/twbuild/node_modules/.bin/tailwindcss -i "$PWD/web/css/input.css" -o web/static/css/app.css --minify
#   rm -f ./node_modules
# A rebuild of an unchanged input.css is byte-identical to the committed artifact.
css:
	$(TAILWIND) -i web/css/input.css -o web/static/css/app.css --minify

css-watch:
	$(TAILWIND) -i web/css/input.css -o web/static/css/app.css --watch

# Compares the local arm64 binaries against what is running on the Pi.
# Exits 0 when both ultron-ap and ultron-helper SHAs match; exits 1
# when either differs (= deploy is stale). Run this after build-arm
# and before / after rsync to confirm prod is in sync with HEAD.
deploy-verify:
	@command -v sha256sum >/dev/null 2>&1 || { command -v shasum >/dev/null 2>&1 || { echo "need sha256sum or shasum"; exit 2; }; }
	@test -f $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 || { echo "missing $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 — run 'make build-arm' first"; exit 2; }
	@test -f $(BUILD_DIR)/ultron-helper-linux-arm64 || { echo "missing $(BUILD_DIR)/ultron-helper-linux-arm64 — run 'make build-arm' first"; exit 2; }
	@LOCAL_AP=$$(sha256sum $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 2>/dev/null | awk '{print $$1}' || shasum -a 256 $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 | awk '{print $$1}'); \
	 LOCAL_HELPER=$$(sha256sum $(BUILD_DIR)/ultron-helper-linux-arm64 2>/dev/null | awk '{print $$1}' || shasum -a 256 $(BUILD_DIR)/ultron-helper-linux-arm64 | awk '{print $$1}'); \
	 PI_OUT=$$(ssh -o ConnectTimeout=5 $(PI_HOST) "sha256sum $(PI_BIN) $(PI_HELPER) 2>&1" || true); \
	 echo "$$PI_OUT" | grep -q "$$LOCAL_AP" || { echo "MISMATCH ultron-ap (local=$$LOCAL_AP)"; echo "$$PI_OUT"; exit 1; }; \
	 echo "$$PI_OUT" | grep -q "$$LOCAL_HELPER" || { echo "MISMATCH ultron-helper (local=$$LOCAL_HELPER)"; echo "$$PI_OUT"; exit 1; }; \
	 echo "OK both binaries on $(PI_HOST) match local arm64 build"; \
	 curl -fsS http://$$(echo $(PI_HOST) | sed 's/.*@//'):8080/version 2>/dev/null && echo
