APP_NAME=em
BUILD_DIR=bin
GOFILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Default values (can be overridden)
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Version metadata baked into the binary via -ldflags. VERSION uses git tags
# when available so `em --version` reports something humans can reason about.
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_HASH   := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X 'main.BuildVersion=$(VERSION)' \
              -X 'main.BuildGitHash=$(GIT_HASH)' \
              -X 'main.BuildDate=$(BUILD_DATE)'

.PHONY: all build build-all install clean test run deploy

# Usage:
#   make build           # builds for your current system, output: bin/em (+ bin/em-<os>-<arch>-<hash>)
#   make build-all       # builds for all supported systems
#   make deploy          # interactive release: bump version, collect notes, build, commit, push
#   GOOS=linux GOARCH=amd64 make build  # cross-compiles for linux/amd64

all: build

build:
	@echo "Building $(APP_NAME) $(VERSION) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./main.go

build-all:
	@echo "Building $(APP_NAME) $(VERSION) for all supported OS/ARCH combinations..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./main.go
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64-$(GIT_HASH) ./main.go
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64-$(GIT_HASH) ./main.go
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64-$(GIT_HASH) ./main.go
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64-$(GIT_HASH) ./main.go

install: build
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(APP_NAME) ~/.local/bin/$(APP_NAME)
	@echo "Installed $(APP_NAME) to ~/.local/bin/$(APP_NAME)"

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

run:
	go run ./main.go $(ARGS)

deploy:
	@bash scripts/deploy.sh
