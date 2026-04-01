# NogoChain Makefile
# Reproducible build system

.PHONY: build build-reproducible test lint fmt vuln docker-build docker-build-reproducible docker-up docker-down clean install-deps smoke testnet mainnet

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GOFLAGS := -trimpath -mod=readonly

# Default build (cmd/node)
build:
	go build ${GOFLAGS} -o nogo ./cmd/node

# Reproducible build (deterministic)
build-reproducible:
	GOWORK=off go build -ldflags="-s -w \
		-X main.version=${VERSION} \
		-X main.buildTime=${BUILD_TIME}" \
		-mod=vendor -o nogo ./cmd/node

# Build with debug info
build-debug:
	go build -o nogo ./cmd/node

# Run tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Vulnerability scan
vuln:
	go sec ./...

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