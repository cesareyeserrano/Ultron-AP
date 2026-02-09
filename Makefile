BINARY_NAME=ultron-ap
BUILD_DIR=bin
GO=go

.PHONY: build build-arm test clean run fmt vet

build:
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ultron-ap/

build-arm:
	GOOS=linux GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/ultron-ap/

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
