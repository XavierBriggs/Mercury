.PHONY: test test-unit test-integration test-coverage build run clean lint setup

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=mercury
BINARY_PATH=./bin/$(BINARY_NAME)

# Build the application
build:
	@echo "Building Mercury..."
	@mkdir -p bin
	@$(GOBUILD) -o $(BINARY_PATH) ./cmd/mercury
	@echo "✓ Build complete: $(BINARY_PATH)"

# Run the application
run: build
	@echo "Starting Mercury..."
	@$(BINARY_PATH)

# Run all tests
test: test-unit

# Run unit tests only (fast, no external dependencies)
test-unit:
	@echo "Running unit tests..."
	@$(GOTEST) -v -race -count=1 ./tests/unit/...
	@echo "✓ Unit tests passed"

# Run integration tests (requires Alexandria DB + Redis)
test-integration:
	@echo "Running integration tests..."
	@$(GOTEST) -v -tags=integration -count=1 ./tests/integration/...
	@echo "✓ Integration tests passed"

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	@$(GOTEST) -bench=. -benchmem -run=^$$ ./...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run --timeout=5m
	@echo "✓ Lint passed"

# Format code
fmt:
	@echo "Formatting code..."
	@$(GOCMD) fmt ./...
	@echo "✓ Format complete"

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	@$(GOMOD) tidy
	@echo "✓ Dependencies tidied"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@$(GOGET) -v ./...
	@echo "✓ Dependencies downloaded"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "✓ Clean complete"

# Setup test database
setup-test-db:
	@echo "Setting up test database..."
	@psql -c "DROP DATABASE IF EXISTS alexandria_test;" -U postgres
	@psql -c "CREATE DATABASE alexandria_test;" -U postgres
	@for file in infra/alexandria/migrations/*.sql; do \
		echo "Running $$file..."; \
		psql alexandria_test -U postgres -f $$file; \
	done
	@for file in infra/alexandria/seed/*.sql; do \
		echo "Running $$file..."; \
		psql alexandria_test -U postgres -f $$file; \
	done
	@echo "✓ Test database ready"

# Run migrations on dev database
migrate:
	@echo "Running migrations on Alexandria DB..."
	@for file in infra/alexandria/migrations/*.sql; do \
		echo "Running $$file..."; \
		psql $$ALEXANDRIA_DSN -f $$file || exit 1; \
	done
	@echo "✓ Migrations complete"

# Seed dev database
seed:
	@echo "Seeding Alexandria DB..."
	@for file in infra/alexandria/seed/*.sql; do \
		echo "Running $$file..."; \
		psql $$ALEXANDRIA_DSN -f $$file || exit 1; \
	done
	@echo "✓ Seed data loaded"

# Docker build
docker-build:
	@echo "Building Docker image..."
	@docker build -t mercury:latest .
	@echo "✓ Docker image built"

# Docker run
docker-run:
	@echo "Running Mercury in Docker..."
	@docker run --rm --env-file .env mercury:latest

# CI pipeline (what runs in GitHub Actions)
ci: lint test-unit test-integration
	@echo "✓ CI pipeline passed"

# Setup everything (recommended for first time)
setup:
	@echo "Running automated Mercury setup..."
	@./scripts/setup-mercury.sh

# Help
help:
	@echo "Mercury Makefile Commands:"
	@echo ""
	@echo "SETUP:"
	@echo "  make setup              🚀 Full automated setup (DB + Redis + migrations + seed)"
	@echo ""
	@echo "BUILD & RUN:"
	@echo "  make build              Build the binary"
	@echo "  make run                Build and run"
	@echo ""
	@echo "TESTING:"
	@echo "  make test               Run all tests"
	@echo "  make test-unit          Run unit tests only"
	@echo "  make test-integration   Run integration tests (needs DB + Redis)"
	@echo "  make test-coverage      Generate coverage report"
	@echo "  make bench              Run benchmarks"
	@echo ""
	@echo "DATABASE:"
	@echo "  make setup-test-db      Create and migrate test database"
	@echo "  make migrate            Run migrations on dev DB"
	@echo "  make seed               Seed dev DB with reference data"
	@echo ""
	@echo "CODE QUALITY:"
	@echo "  make lint               Run linter"
	@echo "  make fmt                Format code"
	@echo "  make tidy               Tidy dependencies"
	@echo "  make clean              Remove build artifacts"
	@echo ""
	@echo "DOCKER:"
	@echo "  make docker-build       Build Docker image"
	@echo ""
	@echo "CI/CD:"
	@echo "  make ci                 Run full CI pipeline"

