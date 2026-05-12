# NogoChain Makefile
# Reproducible build system

.PHONY: build build-no-race build-reproducible test test-race lint vet fmt vuln docker-build docker-build-reproducible docker-up docker-down clean install-deps smoke testnet mainnet

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
CGOFLAGS := CGO_ENABLED=0
GOFLAGS := -trimpath -mod=readonly
LDFLAGS := -ldflags="-s -w"

# Default build with CGO disabled (avoids AV false positives)
build: vet
	$(CGOFLAGS) go build $(LDFLAGS) -o nogo ./blockchain/cmd

# Build with race detector (requires CGO, may trigger AV on some systems)
build-no-race:
	$(CGOFLAGS) go build -ldflags="-s -w" -o nogo ./blockchain/cmd

# Reproducible build (deterministic, CGO disabled)
build-reproducible: vet
	$(CGOFLAGS) GOWORK=off go build -ldflags="-s -w \
		-X main.version=${VERSION} \
		-X main.buildTime=${BUILD_TIME}" \
		-mod=vendor -o nogo ./blockchain/cmd

# Build with debug info (CGO disabled)
build-debug: vet
	$(CGOFLAGS) go build -o nogo ./blockchain/cmd

# Run tests with race detector
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with race detector only
test-race:
	go test -race ./...

# Run go vet for static analysis
vet:
	go vet ./...

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Vulnerability scan
vuln:
	gosec ./...

# Docker build (production)
docker-build:
	docker build -t nogochain/blockchain:${VERSION} -t nogochain/blockchain:latest -f docker/Dockerfile .

# Docker build (reproducible)
docker-build-reproducible:
	docker build --build-arg VERSION=${VERSION} --build-arg BUILD_TIME=${BUILD_TIME} \
		-t nogochain/blockchain:${VERSION} -t nogochain/blockchain:latest -f docker/Dockerfile.reproducible .

# Docker compose up
docker-up:
	docker compose -f docker/docker-compose.yml up -d

# Docker compose down
docker-down:
	docker compose -f docker/docker-compose.yml down

# Clean build artifacts
clean:
	rm -f nogo coverage.out

# Install dependencies
install-deps:
	go mod download

# Run smoke tests
smoke:
	docker compose -f docker-compose.smoke.yml up --build --abort-on-container-exit

# Run testnet
testnet:
	docker compose -f docker-compose.testnet.yml up -d

# Run mainnet
mainnet:
	docker compose -f docker-compose.mainnet.yml up -d