.PHONY: build test clean run run-dry run-built run-dry-built lint fmt mod install docker-build docker-run release help

BINARY_NAME=aegis
GO_FILES=$(shell find . -name "*.go" -type f)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Default values that can be overridden
CONFIG ?= config.yaml
ARGS ?=
LOG_LEVEL ?= info

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

build:
	@echo "$(BLUE)Building $(BINARY_NAME) version $(VERSION)...$(NC)"
	go build -ldflags="-X main.version=${VERSION}" -o bin/${BINARY_NAME} cmd/${BINARY_NAME}/main.go
	@echo "$(GREEN) Build complete!$(NC)"

test:
	@echo "$(BLUE)Running tests...$(NC)"
	go test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN) Tests complete!$(NC)"

clean:
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	go clean
	rm -rf bin/ logs/ coverage.out
	@echo "$(GREEN) Clean complete!$(NC)"

# Run with go run (no build needed, faster for development)
run:
	@echo "$(BLUE)Running $(BINARY_NAME) with config: $(CONFIG)$(NC)"
	go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) $(ARGS)

# Run with go run in dry-run mode
run-dry:
	@echo "$(BLUE)Running $(BINARY_NAME) in DRY-RUN mode with config: $(CONFIG)$(NC)"
	go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) --dry-run $(ARGS)

# Run with built binary
run-built: build
	@echo "$(BLUE)Running built $(BINARY_NAME) with config: $(CONFIG)$(NC)"
	./bin/${BINARY_NAME} --config $(CONFIG) $(ARGS)
# Run with built binary in dry-run mode
run-dry-built: build
	@echo "$(BLUE)Running built $(BINARY_NAME) in DRY-RUN mode with config: $(CONFIG)$(NC)"
	./bin/${BINARY_NAME} --config $(CONFIG) --dry-run $(ARGS)

# Run with debug logging
run-debug:
	@echo "$(BLUE)Running $(BINARY_NAME) with DEBUG logging...$(NC)"
	go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) --log-level debug $(ARGS)

# Run with auto-approve (use with caution!)
run-auto:
	@echo "$(YELLOW)  Running $(BINARY_NAME) with AUTO-APPROVE enabled$(NC)"
	@echo "$(YELLOW)  This will automatically approve all deletions!$(NC)"
	@read -p "Are you sure? (y/N) " -n 1 -r; \
	echo ""; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) --auto-approve $(ARGS); \
	else \
		echo "$(RED)Cancelled$(NC)"; \
		exit 1; \
	fi

# Run with override token
run-override:
	@echo "$(YELLOW)  Running $(BINARY_NAME) with OVERRIDE token$(NC)"
	@read -p "Enter override token: " token; \
	go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) --override $$token $(ARGS)

# Run without interactive mode
run-noninteractive:
	@echo "$(BLUE)Running $(BINARY_NAME) in NON-INTERACTIVE mode...$(NC)"
	go run cmd/${BINARY_NAME}/main.go --config $(CONFIG) --interactive=false $(ARGS)

lint:
	@echo "$(BLUE)Running linter...$(NC)"
	golangci-lint run ./...
	@echo "$(GREEN) Lint complete!$(NC)"

fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN) Format complete!$(NC)"

mod:
	@echo "$(BLUE)Tidying and vendoring modules...$(NC)"
	go mod tidy
	go mod vendor
	@echo "$(GREEN) Module maintenance complete!$(NC)"

install: build
	@echo "$(BLUE)Installing $(BINARY_NAME)...$(NC)"
	@INSTALL_DIR=""; \
	for d in "$$(go env GOPATH)/bin" "$$HOME/.local/bin" "/opt/homebrew/bin" "/usr/local/bin"; do \
		if echo "$$PATH" | tr ':' '\n' | grep -qx "$$d" && [ -d "$$d" ] && [ -w "$$d" ]; then \
			INSTALL_DIR="$$d"; break; \
		fi; \
	done; \
	if [ -z "$$INSTALL_DIR" ]; then \
		echo "$(RED)No writable directory found on your PATH.$(NC)"; \
		echo "$(RED)Ensure one of these is on PATH and writable, then rerun:$$(printf '\n  ')$(GOPATH)/bin, $(HOME)/.local/bin, /opt/homebrew/bin, /usr/local/bin$(NC)"; \
		echo "$(RED)Or install manually: cp bin/${BINARY_NAME} <writable PATH dir>$(NC)"; \
		exit 1; \
	fi; \
	cp bin/${BINARY_NAME} "$$INSTALL_DIR/" && \
	echo "$(GREEN) Installed $(BINARY_NAME) to $$INSTALL_DIR. You can now run '$(BINARY_NAME)' from anywhere$(NC)"

docker-build:
	@echo "$(BLUE)Building Docker image $(BINARY_NAME):$(VERSION)...$(NC)"
	docker build -t ${BINARY_NAME}:${VERSION} -f Dockerfile .
	@echo "$(GREEN) Docker build complete!$(NC)"

# Runtime helper for the non-root Docker image: mounts the kubeconfig and
# config file into the container's HOME (/config), persists logs, and passes
# the host Docker socket group through so the non-root user can reach it.
DOCKER_SOCK_GID=$(shell stat -c %g /var/run/docker.sock 2>/dev/null || echo 0)

docker-run:
	@echo "$(BLUE)Running $(BINARY_NAME) in Docker...$(NC)"
	docker run --rm \
		--user 65532:65532 \
		--group-add ${DOCKER_SOCK_GID} \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v ${HOME}/.kube/config:/config/.kube/config \
		-v ${PWD}/config.yaml:/config/config.yaml \
		-v ${PWD}/logs:/config/logs \
		${BINARY_NAME}:${VERSION} clean $(ARGS)

docker-run-dry:
	@echo "$(BLUE)Running $(BINARY_NAME) in Docker (DRY-RUN mode)...$(NC)"
	docker run --rm \
		--user 65532:65532 \
		--group-add ${DOCKER_SOCK_GID} \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v ${HOME}/.kube/config:/config/.kube/config \
		-v ${PWD}/config.yaml:/config/config.yaml \
		-v ${PWD}/logs:/config/logs \
		${BINARY_NAME}:${VERSION} clean --dry-run $(ARGS)

release: test build
	@echo "$(BLUE)Building release binaries...$(NC)"
	mkdir -p bin/release
	GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o bin/release/${BINARY_NAME}-linux-amd64 cmd/${BINARY_NAME}/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o bin/release/${BINARY_NAME}-darwin-amd64 cmd/${BINARY_NAME}/main.go
	GOOS=darwin GOARCH=arm64 go build -ldflags="-X main.version=${VERSION}" -o bin/release/${BINARY_NAME}-darwin-arm64 cmd/${BINARY_NAME}/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o bin/release/${BINARY_NAME}-windows-amd64.exe cmd/${BINARY_NAME}/main.go
	@echo "$(GREEN) Release binaries built in bin/release/$(NC)"
	@ls -la bin/release/

# Run with specific config file
config-%:
	@echo "$(BLUE)Using config: $*$(NC)"
	$(MAKE) run CONFIG=$*

config-dry-%:
	@echo "$(BLUE)Using config: $* (DRY-RUN)$(NC)"
	$(MAKE) run-dry CONFIG=$*

# Show version
version:
	@echo "$(BINARY_NAME) version: $(VERSION)"

# Check if binary exists
check:
	@if [ -f "bin/$(BINARY_NAME)" ]; then \
		echo "$(GREEN) Binary exists at bin/$(BINARY_NAME)$(NC)"; \
	else \
		echo "$(RED)❌ Binary not found. Run 'make build' first.$(NC)"; \
	fi

help:
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║                    $(BINARY_NAME) Makefile                     ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(YELLOW)BUILD & INSTALL:$(NC)"
	@echo "  build              - Build the binary"
	@echo "  install            - Install to GOPATH/bin"
	@echo "  clean              - Clean build artifacts"
	@echo ""
	@echo "$(YELLOW)RUN (with go run, no build needed):$(NC)"
	@echo "  run                - Run with default config (CONFIG=config.yaml)"
	@echo "  run-dry            - Run in DRY-RUN mode"
	@echo "  run-debug          - Run with DEBUG logging"
	@echo "  run-noninteractive - Run in NON-INTERACTIVE mode"
	@echo "  run-auto           - Run with AUTO-APPROVE (use with caution!)"
	@echo "  run-override       - Run with OVERRIDE token"
	@echo ""
	@echo "$(YELLOW)RUN (with built binary):$(NC)"
	@echo "  run-built          - Run the built binary"
	@echo "  run-dry-built      - Run built binary in DRY-RUN mode"
	@echo ""
	@echo "$(YELLOW)CONFIG OVERRIDES:$(NC)"
	@echo "  CONFIG=file.yaml   - Use a specific config file"
	@echo "  ARGS=\"--flag\"      - Pass additional flags"
	@echo "  LOG_LEVEL=debug    - Set log level"
	@echo ""
	@echo "$(YELLOW)DOCKER:$(NC)"
	@echo "  docker-build       - Build Docker image"
	@echo "  docker-run         - Run in Docker"
	@echo "  docker-run-dry     - Run in Docker (DRY-RUN)"
	@echo ""
	@echo "$(YELLOW)RELEASE:$(NC)"
	@echo "  release            - Build release binaries for all platforms"
	@echo "  version            - Show version"
	@echo "  check              - Check if binary exists"
	@echo ""
	@echo "$(YELLOW)EXAMPLES:$(NC)"
	@echo "  $(GREEN)make run CONFIG=config.dev.yaml$(NC)"
	@echo "  $(GREEN)make run-dry ARGS=\"--log-level debug\"$(NC)"
	@echo "  $(GREEN)make run CONFIG=config.prod.yaml ARGS=\"--auto-approve\"$(NC)"
	@echo "  $(GREEN)make run-dry CONFIG=config.yaml$(NC)"
	@echo ""
	@echo "$(YELLOW)DEVELOPMENT:$(NC)"
	@echo "  test               - Run tests"
	@echo "  lint               - Run linter"
	@echo "  fmt                - Format code"
	@echo "  mod                - Tidy and vendor modules"