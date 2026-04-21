GO ?= go

.PHONY: test build fmt

test:
	$(GO) test ./...

build:
	$(GO) build ./...

fmt:
	$(GO) fmt ./...
