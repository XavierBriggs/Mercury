#!/bin/bash
set -e

echo "================================"
echo "Mercury Test Suite Runner"
echo "================================"
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check dependencies
echo "Checking dependencies..."

if ! command -v go &> /dev/null; then
    echo -e "${RED}✗ Go is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go is installed$(go version)${NC}"

if ! command -v psql &> /dev/null; then
    echo -e "${YELLOW}⚠ PostgreSQL client not found (needed for integration tests)${NC}"
fi

if ! command -v redis-cli &> /dev/null; then
    echo -e "${YELLOW}⚠ Redis client not found (needed for integration tests)${NC}"
fi

echo ""

# Parse arguments
RUN_UNIT=true
RUN_INTEGRATION=false
RUN_BENCH=false
COVERAGE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --integration)
            RUN_INTEGRATION=true
            shift
            ;;
        --all)
            RUN_INTEGRATION=true
            RUN_BENCH=true
            shift
            ;;
        --bench)
            RUN_BENCH=true
            shift
            ;;
        --coverage)
            COVERAGE=true
            shift
            ;;
        --help)
            echo "Usage: ./scripts/run-tests.sh [options]"
            echo ""
            echo "Options:"
            echo "  --integration    Run integration tests (requires DB + Redis)"
            echo "  --bench          Run benchmarks"
            echo "  --all            Run everything"
            echo "  --coverage       Generate coverage report"
            echo "  --help           Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run unit tests
if [ "$RUN_UNIT" = true ]; then
    echo "================================"
    echo "Running Unit Tests"
    echo "================================"
    echo ""
    
    if [ "$COVERAGE" = true ]; then
        go test -v -race -coverprofile=coverage.out -covermode=atomic ./tests/unit/...
        go tool cover -func=coverage.out | tail -1
    else
        go test -v -race ./tests/unit/...
    fi
    
    if [ $? -eq 0 ]; then
        echo -e "\n${GREEN}✓ Unit tests PASSED${NC}\n"
    else
        echo -e "\n${RED}✗ Unit tests FAILED${NC}\n"
        exit 1
    fi
fi

# Run integration tests
if [ "$RUN_INTEGRATION" = true ]; then
    echo "================================"
    echo "Running Integration Tests"
    echo "================================"
    echo ""
    
    # Check if test DB exists
    if ! psql -lqt | cut -d \| -f 1 | grep -qw alexandria_test; then
        echo -e "${YELLOW}⚠ Test database 'alexandria_test' not found${NC}"
        echo "Creating test database..."
        make setup-test-db
    fi
    
    # Check if Redis is running
    if ! redis-cli ping > /dev/null 2>&1; then
        echo -e "${RED}✗ Redis is not running${NC}"
        echo "Start Redis with: redis-server --daemonize yes"
        exit 1
    fi
    
    go test -v -tags=integration ./tests/integration/...
    
    if [ $? -eq 0 ]; then
        echo -e "\n${GREEN}✓ Integration tests PASSED${NC}\n"
    else
        echo -e "\n${RED}✗ Integration tests FAILED${NC}\n"
        exit 1
    fi
fi

# Run benchmarks
if [ "$RUN_BENCH" = true ]; then
    echo "================================"
    echo "Running Benchmarks"
    echo "================================"
    echo ""
    
    go test -bench=. -benchmem -run=^$$ ./tests/unit/...
    
    if [ $? -eq 0 ]; then
        echo -e "\n${GREEN}✓ Benchmarks completed${NC}\n"
    else
        echo -e "\n${RED}✗ Benchmarks FAILED${NC}\n"
        exit 1
    fi
fi

# Generate coverage HTML if requested
if [ "$COVERAGE" = true ] && [ -f coverage.out ]; then
    echo "Generating HTML coverage report..."
    go tool cover -html=coverage.out -o coverage.html
    echo -e "${GREEN}✓ Coverage report: coverage.html${NC}"
fi

echo ""
echo "================================"
echo -e "${GREEN}All tests completed successfully!${NC}"
echo "================================"




