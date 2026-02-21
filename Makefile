# Stratavore Makefile
# Version is the single source of truth in VERSION file.
# Override at build time: make VERSION=1.5.0 build
# Bump everywhere at once: make bump-version V=1.5.0

.PHONY: all build install clean test test-integration test-load lint migration-up migration-down docker-setup proto bump-version help

BINARY_NAME=stratavore
DAEMON_NAME=stratavored
AGENT_NAME=stratavore-agent
MIGRATE_NAME=stratavore-migrate
VERSION?=$(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}"

all: proto build

deps:
	go mod download
	go mod tidy

# Generate protobuf code with auto-detection
proto:
	@echo "Checking for protoc..."
	@if command -v protoc >/dev/null 2>&1; then \
		echo "[OK] protoc found"; \
		if command -v protoc-gen-go >/dev/null 2>&1 && command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
			echo "[OK] Go plugins found"; \
			echo "Generating protobuf code..."; \
			mkdir -p pkg/api/generated; \
			protoc --go_out=pkg/api/generated --go_opt=paths=source_relative \
			       --go-grpc_out=pkg/api/generated --go-grpc_opt=paths=source_relative \
			       --proto_path=pkg/api \
			       pkg/api/stratavore.proto && \
			echo "[OK] Protobuf code generated in pkg/api/generated/"; \
		else \
			echo "[WARN] Go plugins not found"; \
			echo "[INFO] Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; \
			echo "[INFO] Install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"; \
			echo "[INFO] Using fallback API types"; \
		fi \
	else \
		echo "[WARN] protoc not found - using fallback types"; \
		echo "[INFO] Install protoc: https://grpc.io/docs/protoc-installation/"; \
	fi

build:
	@echo "Building Stratavore v${VERSION}..."
	@mkdir -p bin
	go build ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/stratavore
	@echo "[OK] bin/${BINARY_NAME}"
	go build ${LDFLAGS} -o bin/${DAEMON_NAME} ./cmd/stratavored
	@echo "[OK] bin/${DAEMON_NAME}"
	go build ${LDFLAGS} -o bin/${AGENT_NAME} ./cmd/stratavore-agent
	@echo "[OK] bin/${AGENT_NAME}"
	go build ${LDFLAGS} -o bin/${MIGRATE_NAME} ./cmd/stratavore-migrate
	@echo "[OK] bin/${MIGRATE_NAME}"
	@echo ""
	@echo "Build complete! Binaries in bin/"

quick:
	@mkdir -p bin
	@go build -o bin/${BINARY_NAME} ./cmd/stratavore
	@go build -o bin/${DAEMON_NAME} ./cmd/stratavored
	@go build -o bin/${AGENT_NAME} ./cmd/stratavore-agent
	@go build -o bin/${MIGRATE_NAME} ./cmd/stratavore-migrate
	@echo "Quick build complete"

install: build
	@echo "Installing Stratavore to /usr/local/bin..."
	sudo cp bin/${BINARY_NAME} /usr/local/bin/
	sudo cp bin/${DAEMON_NAME} /usr/local/bin/
	sudo cp bin/${AGENT_NAME} /usr/local/bin/
	sudo cp bin/${MIGRATE_NAME} /usr/local/bin/
	sudo chmod +x /usr/local/bin/${BINARY_NAME}
	sudo chmod +x /usr/local/bin/${DAEMON_NAME}
	sudo chmod +x /usr/local/bin/${AGENT_NAME}
	sudo chmod +x /usr/local/bin/${MIGRATE_NAME}
	@echo "Creating config directory..."
	mkdir -p ~/.config/stratavore
	@echo "Installation complete!"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf pkg/api/generated/
	rm -f stratavore.db
	@echo "Clean complete"

test:
	go test -short ./...

test-integration:
	go test ./test/integration/... -v -timeout 60s

test-load:
	go test ./test/load/... -v -load-addr http://localhost:8080

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; fi

migration-up:
	@echo "Running database migrations (up)..."
	./scripts/migrate.sh up

migration-down:
	@echo "Rolling back database migrations..."
	./scripts/migrate.sh down

docker-setup:
	@echo "Setting up Docker infrastructure integration..."
	./scripts/setup-docker-integration.sh

# Build the full protobuf-capable image and export binaries to ./dist
docker-build-proto:
	@echo "Building Stratavore with protobuf support (Docker)..."
	mkdir -p dist
	docker build -f Dockerfile.builder --target export \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t stratavore-builder:$(VERSION) .
	docker run --rm -v "$(PWD)/dist:/dist" stratavore-builder:$(VERSION)
	@echo "[OK] Artifacts in dist/"

# Launch the full stack using the gRPC-capable daemon image
docker-up-grpc:
	@echo "Starting full Stratavore stack (gRPC build)..."
	VERSION=$(VERSION) COMMIT=$(COMMIT) \
	docker compose -f docker-compose.yml -f docker-compose.builder.yml up --build

# Open an interactive protobuf dev shell
docker-proto-shell:
	docker compose -f docker-compose.builder.yml run --rm proto-dev

# Bump version across all files. Usage: make bump-version V=1.5.0
bump-version:
	@if [ -z "$(V)" ]; then echo "Usage: make bump-version V=x.y.z" >&2; exit 1; fi
	@bash scripts/bump-version.sh $(V)

systemd-install:
	@echo "Installing systemd service..."
	sudo cp deployments/systemd/stratavored.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable stratavored
	@echo "Service installed. Start with: sudo systemctl start stratavored"

format:
	gofmt -w -s .
	@if command -v goimports >/dev/null 2>&1; then goimports -w .; fi

run-daemon:
	go run ./cmd/stratavored

run-cli:
	go run ./cmd/stratavore

install-proto-tools:
	@echo "Installing protobuf Go plugins..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Done! Make sure \$$GOPATH/bin is in your PATH"

help:
	@echo "Stratavore Build System v${VERSION}"
	@echo ""
	@echo "Targets:"
	@echo "  all                  - Generate protobuf and build all components (default)"
	@echo "  deps                 - Download dependencies"
	@echo "  proto                - Generate protobuf Go code (auto-detects tools)"
	@echo "  build                - Build CLI, daemon, agent, and migrate tools"
	@echo "  quick                - Quick build without protobuf (development)"
	@echo "  install              - Install binaries to /usr/local/bin"
	@echo "  clean                - Remove build artifacts"
	@echo "  test                 - Run unit tests (short, no live daemon required)"
	@echo "  test-integration     - Run integration tests (requires live daemon on :8080)"
	@echo "  test-load            - Run load tests (requires live daemon on :8080)"
	@echo "  lint                 - Run linters"
	@echo "  migration-up         - Apply database migrations"
	@echo "  migration-down       - Rollback database migrations"
	@echo "  bump-version         - Bump version everywhere: make bump-version V=1.5.0"
	@echo "  docker-setup         - Configure Docker integration (infra only)"
	@echo "  docker-build-proto   - Build protobuf-capable image, export bins to ./dist"
	@echo "  docker-up-grpc       - Start full stack with gRPC daemon (Compose)"
	@echo "  docker-proto-shell   - Interactive shell with protoc + Go plugins"
	@echo "  systemd-install      - Install systemd service"
	@echo "  format               - Format Go code"
	@echo "  run-daemon           - Run daemon in dev mode"
	@echo "  run-cli              - Run CLI in dev mode"
	@echo "  install-proto-tools  - Install protobuf Go plugins"
