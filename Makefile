BINARY_NAME := teslausb
MODULE := github.com/ejaramilla/teslausb-neo
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: binary-arm64 binary-local test vet clean

binary-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o build/$(BINARY_NAME)-linux-arm64 ./cmd/teslausb

binary-local:
	go build $(LDFLAGS) -o build/$(BINARY_NAME) ./cmd/teslausb

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf build/
