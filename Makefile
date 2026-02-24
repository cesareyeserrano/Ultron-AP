BINARY_NAME=ultron-ap
BUILD_DIR=bin
GO=go
TAILWIND=./tailwindcss

.PHONY: build build-arm test clean run fmt vet css css-watch

build: css
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ultron-ap/
	$(GO) build -o $(BUILD_DIR)/ultron-helper ./cmd/ultron-helper/

build-arm: css
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/ultron-ap/
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/ultron-helper-linux-arm64 ./cmd/ultron-helper/

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

css:
	$(TAILWIND) -i web/css/input.css -o web/static/css/app.css --minify

css-watch:
	$(TAILWIND) -i web/css/input.css -o web/static/css/app.css --watch
