GO ?= go

BIN_DIR ?= ./bin
CLI_NAME ?= mh2c
CLI_PATH := $(BIN_DIR)/$(CLI_NAME)

.PHONY: test build build-cli install fmt

test:
	$(GO) test ./...

build:
	$(GO) build ./...

build-cli:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(CLI_PATH) ./cmd/mh2c

install:
	$(GO) install ./cmd/mh2c

fmt:
	$(GO) fmt ./...
