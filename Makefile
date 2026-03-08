APP_NAME := Koko Tools
BINARY_NAME := koko-tools

.PHONY: build test fmt build-macos-arm64 package-macos-arm64

build:
	go build -o $(BINARY_NAME)

test:
	go test ./...

fmt:
	gofmt -w main.go $$(find src -name '*.go')

build-macos-arm64:
	./scripts/build-macos-arm64.sh

package-macos-arm64:
	./scripts/build-macos-arm64.sh --bundle
