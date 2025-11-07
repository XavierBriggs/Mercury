# Mercury Tests

Comprehensive test suite for the Mercury odds aggregator.

## Structure

```
tests/
├── unit/              # Fast, isolated unit tests (no external dependencies)
│   ├── delta/        # Delta detection engine tests
│   ├── adapters/     # Vendor adapter tests (mocked HTTP)
│   └── sports/       # Sport module tests (config, validation)
├── integration/       # Integration tests (requires DB + Redis)
│   └── integration_test.go
├── fixtures/          # Test data and golden files
└── testutil/          # Shared test utilities (in pkg/testutil/)
```

## Running Tests

### Unit Tests (Fast - No Dependencies)
```bash
# Run all unit tests
make test-unit

# Run specific package
go test -v ./tests/unit/delta/...
go test -v ./tests/unit/adapters/...
go test -v ./tests/unit/sports/...
```

### Integration Tests (Requires Infrastructure)
```bash
# Setup test database first
make setup-test-db

# Start Redis (if not running)
redis-server --daemonize yes

# Run integration tests
make test-integration
```

### All Tests
```bash
make test
```

### Coverage Report
```bash
make test-coverage
# Opens coverage.html in browser
```

### Benchmarks
```bash
make bench

# Specific benchmarks
go test -bench=DetectChanges -benchmem ./tests/unit/delta/
```

## Test Categories

### 1. Unit Tests

**Delta Engine** (`tests/unit/delta/`)
- ✅ `TestDetectChanges_NewOutcome` - First time seeing an odd
- ✅ `TestDetectChanges_PriceChange` - Price moves detected
- ✅ `TestDetectChanges_PointChange` - Spread/total line moves
- ✅ `TestDetectChanges_NoChange` - Unchanged odds filtered out
- ✅ `BenchmarkDetectChanges` - Verify <1ms SLO

**Adapters** (`tests/unit/adapters/`)
- ✅ `TestSupportsMarket` - Market support checks
- ✅ `TestNewClient` - Client initialization
- ✅ `TestGetRateLimits` - Rate limit tracking
- ⚠️  HTTP mocking tests need refactoring (see TODO in file)

**Sports Modules** (`tests/unit/sports/`)
- ✅ `TestDefaultConfig` - Plan A configuration
- ✅ `TestGetFeaturedInterval_*` - Featured market polling cadence
- ✅ `TestGetPropsInterval` - Props ramping tiers
- ✅ `TestRampTiersOrdering` - Tier connectivity validation
- ✅ Benchmarks for interval calculations

### 2. Integration Tests

**End-to-End Pipeline** (`tests/integration/`)
- ✅ `TestEndToEnd_FetchDetectWrite` - Full pipeline test
  - Fetch → Delta Detect → Write → Stream → Cache
  - Verifies `is_latest` flag updates
  - Validates Redis Stream publishing
- ✅ `TestIntegration_LatencySLO` - Verify <30ms Mercury SLO
  - Delta detection <1ms
  - Write + cache update <30ms total
  - Tests with 100 concurrent odds

### 3. Test Utilities

**Fixtures** (`pkg/testutil/fixtures.go`)
- `NewTestEvent()` - Create test events
- `NewTestOdd()` - Create test odds
- `GetGoldenFixtures()` - Known odds with expected normalizations
- `MockVendorAdapter` - Stub adapter for testing

## Writing Tests

### Unit Test Template

```go
package mypackage_test

import (
	"testing"
	"github.com/XavierBriggs/Mercury/internal/mypackage"
)

func TestMyFunction(t *testing.T) {
	// Arrange
	input := "test"

	// Act
	result := mypackage.MyFunction(input)

	// Assert
	if result != expected {
		t.Errorf("got %v, want %v", result, expected)
	}
}
```

### Integration Test Template

```go
// +build integration

package integration_test

import (
	"context"
	"testing"
	// ... imports
)

func TestMyIntegration(t *testing.T) {
	ctx := context.Background()

	// Setup infrastructure
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Test integration
	// ...
}
```

## CI/CD

Tests run in GitHub Actions:

```yaml
# Unit tests (always run)
- run: make test-unit

# Integration tests (requires services)
- run: |
    docker-compose -f deploy/docker-compose.test.yml up -d
    make test-integration
```

## Test Data

### Golden Fixtures

Located in `pkg/testutil/fixtures.go`, these provide known odds with expected calculations:

- **Even Money Spread** - -110/-110, expects 50% no-vig prob
- **Favorite/Underdog** - -105/-115, expects asymmetric probabilities
- **Sharp vs Soft** - Pinnacle vs FanDuel edge detection

Used for validating normalizer math in downstream services.

## Performance Targets

| Component | Target | Test |
|-----------|--------|------|
| Delta Detection | <1ms for 100 odds | `BenchmarkDetectChanges` |
| DB Write Batch | <20ms for 100 rows | `TestIntegration_LatencySLO` |
| Total Mercury | <30ms (excl. vendor) | `TestIntegration_LatencySLO` |

## Troubleshooting

### "Connection refused" errors
```bash
# Check PostgreSQL is running
psql -l

# Check Redis is running
redis-cli ping
```

### "Database does not exist"
```bash
make setup-test-db
```

### Stale cache in tests
```bash
# Tests should call FlushDB, but if needed:
redis-cli FLUSHDB
```

### Import cycle errors
- Keep `_test` package suffix
- Import production packages, don't mix test/prod code

## Coverage Goals

- **Unit Tests:** >80% coverage for core logic
- **Integration Tests:** Cover all happy paths + critical error cases
- **Benchmarks:** Validate all SLO targets

Current coverage:
```bash
make test-coverage
# View coverage.html
```

---

**Next Steps:**
1. Add HTTP mocking for adapter tests (needs client refactor)
2. Add contract tests for VendorAdapter interface
3. Add chaos/fault injection tests
4. Add load tests for scheduler under high volume

Last Updated: 2025-11-06

