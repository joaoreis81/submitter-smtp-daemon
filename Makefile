.PHONY: build test lint clean install docker-build docker-push help

# Variables
BINARY_NAME=smtp-edge-proxy
VERSION?=v0.2.0
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Docker variables
DOCKER_IMAGE?=smtp-edge-proxy
DOCKER_TAG?=$(VERSION)
DOCKER_REGISTRY?=

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd

# Build with race detector
build-race:
	@echo "Building $(BINARY_NAME) with race detector..."
	go build -race $(LDFLAGS) -o $(BINARY_NAME) ./cmd

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Lint the code
lint:
	@echo "Linting code..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	go clean

# Install binary to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) ./cmd

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	go mod verify

# Run the binary
run: build
	./$(BINARY_NAME) -config config.yaml

# Build Docker image
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		.

# Push Docker image
docker-push:
	@echo "Pushing Docker image $(DOCKER_REGISTRY)$(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)$(DOCKER_IMAGE):$(DOCKER_TAG)

# Run Docker container
docker-run:
	docker run --rm -it \
		-p 587:587 \
		-p 465:465 \
		-p 8080:8080 \
		-p 9090:9090 \
		-v $(PWD)/certs:/certs:ro \
		-v $(PWD)/config.yaml:/app/config.yaml:ro \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  build-race     - Build with race detector"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  lint           - Lint code with golangci-lint"
	@echo "  clean          - Remove build artifacts"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  fmt            - Format code"
	@echo "  tidy           - Tidy dependencies"
	@echo "  verify         - Verify dependencies"
	@echo "  run            - Build and run the binary"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-push    - Push Docker image to registry"
	@echo "  docker-run     - Run Docker container"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION        - Version tag (default: dev)"
	@echo "  DOCKER_IMAGE   - Docker image name (default: smtp-edge-proxy)"
	@echo "  DOCKER_TAG     - Docker image tag (default: VERSION)"
	@echo "  DOCKER_REGISTRY- Docker registry (default: empty)"
