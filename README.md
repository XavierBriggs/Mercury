# Mercury

**God of Speed and Commerce** - Multi-Sport Odds Aggregator

Mercury is a high-performance odds aggregation service that polls vendor APIs, detects changes at sub-millisecond speed, and publishes odds deltas to downstream services.

## Features

- ⚡ **Redis-First Delta Detection** - <1ms change detection using in-memory comparison
- 📊 **Batched Writes** - Groups 100 rows / 5s for efficient Alexandria DB inserts
- 🔄 **Write-Through Cache** - Updates Redis after successful DB writes
- 📡 **Stream Publishing** - Emits deltas to Redis Streams for real-time processing
- 🏀 **Multi-Sport Ready** - NBA active in v0, NFL/MLB ready for v1
- 🔌 **Pluggable Adapters** - Clean VendorAdapter interface (FR8)

## Architecture

```
┌──────────────┐
│  Scheduler   │ ← Orchestrates polling with Plan A cadence
└──────┬───────┘
       │
       ├─> TheOddsAPI Adapter ──> Fetch Odds
       │         │
       │         ▼
       ├─> Delta Engine (Redis) ──> Detect Changes (<1ms)
       │         │
       │         ▼ (only deltas proceed)
       ├─> Writer ──────────────────> Batch Insert Alexandria
       │         │                    UPDATE is_latest flags
       │         │
       │         ├─> Publish to Redis Stream (odds.raw.basketball_nba)
       │         └─> Update Redis Cache (write-through)
       │
       └─> NBA Module ──> Plan A Config + Validation
```

## Components

### 1. Adapters
- **TheOddsAPI** (`adapters/theoddsapi/`) - Production adapter for The Odds API
- **Pinnacle** (`adapters/pinnacle/`) - Interface only (future)
- **Internal** (`adapters/internal/`) - Interface only (future)

### 2. Sport Modules
- **basketball_nba** (`sports/basketball_nba/`) - Active in v0
  - Plan A polling config (60s pre-match → 40s ramp → 40s in-play)
  - Props ramping (30min → 1min near tipoff)
  - Market mappings and validation

### 3. Internal Services
- **Scheduler** (`internal/scheduler/`) - Polling orchestrator
- **Delta Engine** (`internal/delta/`) - Redis-first change detection
- **Writer** (`internal/writer/`) - Batched Alexandria writes + stream publish

### 4. Infrastructure
- **Alexandria DB** (`infra/alexandria/`) - Raw odds database (Mercury's private store)
  - 6 migrations + 3 seed files
  - 2NF/3NF normalized schema
  - `is_latest` flag for fast current odds queries

## Modular Architecture & Expansion

Mercury uses a **Sport Registry pattern** to support multiple sports without modifying core code. Each sport is a pluggable module that implements the `SportModule` interface.

### Sport Module Interface

```go
type SportModule interface {
    GetSportKey() string                      // "basketball_nba"
    GetDisplayName() string                   // "NBA Basketball"
    GetFeaturedMarkets() []string             // ["h2h", "spreads", "totals"]
    GetRegions() []string                     // ["us", "us2"]
    GetFeaturedPollInterval() time.Duration   // 60s
    GetPropsPollInterval() time.Duration      // 30min
    GetPropsDiscoveryInterval() time.Duration // 6h
    GetPropsDiscoveryWindowHours() int        // 48
    ShouldPollProps() bool                    // true
    ValidateOdds(odds RawOdds) error          // Sport-specific validation
}
```

### Current Sports

| Sport | Status | Poll Interval | Markets | Props |
|-------|--------|---------------|---------|-------|
| **NBA** | ✅ Active (v0) | 60s | h2h, spreads, totals | ✅ Enabled |
| **NFL** | 🔜 Planned (v1) | 30s | h2h, spreads, totals, alt_lines | ✅ Enabled |
| **MLB** | 🔜 Planned (v1) | 90s | h2h, run_lines, totals, f5_innings | ✅ Enabled |

### Adding a New Sport

**1. Create Sport Module** (`sports/american_football_nfl/`)

```go
// sports/american_football_nfl/module.go
type Module struct {
    config *Config
}

func NewModule() *Module {
    return &Module{config: DefaultConfig()}
}

func (m *Module) GetSportKey() string {
    return "american_football_nfl"
}

func (m *Module) GetFeaturedPollInterval() time.Duration {
    return 30 * time.Second  // NFL lines move faster
}

// Implement remaining SportModule methods...
```

**2. Create Sport Configuration** (`sports/american_football_nfl/config.go`)

```go
func DefaultConfig() *Config {
    return &Config{
        SportKey:    "american_football_nfl",
        DisplayName: "NFL Football",
        Regions:     []string{"us"},
        Featured: FeaturedConfig{
            PollInterval: 30 * time.Second,  // Faster than NBA
        },
        // NFL-specific config...
    }
}
```

**3. Register Sport in `main.go`**

```go
// Initialize sport registry
sportRegistry := registry.NewSportRegistry()

// Register NBA
nbaModule := basketball_nba.NewModule()
sportRegistry.Register(nbaModule)

// Register NFL (future)
nflModule := american_football_nfl.NewModule()
sportRegistry.Register(nflModule)

// Mercury automatically polls both sports! ✅
```

**That's it!** No changes to scheduler, delta engine, or writer needed. Each sport runs independently with its own polling intervals and configuration.

### Why This Architecture?

1. **🔌 Pluggable** - Add/remove sports without touching core code
2. **⚙️ Sport-Specific Config** - Each sport defines its own polling strategy
3. **🧪 Testable** - Mock sport modules for unit testing
4. **📈 Scalable** - Sports run in parallel goroutines
5. **🎯 Domain Expertise** - Betting characteristics vary by sport:
   - **NBA**: Fast line movement (60s), props discovery every 6h
   - **NFL**: Very fast movement (30s), weekly schedule (12h discovery)
   - **MLB**: Slower lines (90s), daily games (24h discovery)

### Sport-Specific Polling Examples

**NBA Basketball** (High Volatility)
```
Featured: 60s → 40s (ramp) → 40s (live)
Props: 30min → 1min (near tipoff)
Discovery: Every 6 hours (48hr window)
```

**NFL Football** (Extreme Volatility - Future)
```
Featured: 30s → 20s (ramp) → 20s (live)
Props: 15min → 30s (near kickoff)
Discovery: Every 12 hours (weekly schedule)
```

**MLB Baseball** (Lower Volatility - Future)
```
Featured: 90s → 60s (ramp) → 60s (live)
Props: 20min → 2min (near first pitch)
Discovery: Every 24 hours (daily games)
```

## Quick Start

### Prerequisites
- Go 1.23+
- PostgreSQL 17+
- Redis 7+
- The Odds API key

### Environment Variables
```bash
# Database
ALEXANDRIA_DSN=postgres://user:pass@localhost:5432/alexandria

# Redis Cache
REDIS_URL=localhost:6379
REDIS_PASSWORD=your_redis_password

# External API
ODDS_API_KEY=your_api_key_here

# Performance Tuning
MERCURY_CACHE_TTL=2m  # Cache TTL for delta detection (must exceed poll interval)
                       # Supported formats: 5m, 300s, 10m, 1h
                       # Default: 5m (recommended for 60s poll interval)
```

### Run Locally
```bash
cd /Users/xavierbriggs/development/fortuna/mercury

# Run Setup
make setup

# Start Mercury
go run cmd/mercury/main.go
```

### Run with Docker
```bash
docker build -t mercury:latest -f Dockerfile .
docker run --env-file .env mercury:latest
```

## Configuration

### Plan A Cadence (Phase 3 Approved)

**Featured Markets (h2h, spreads, totals):**
- Pre-match: 60 seconds
- Ramp within 6hr: Linear ramp to 40s
- In-play: 40 seconds

**Player Props:**
- Discovery: Every 6 hours (48hr window)
- Ramping tiers:
  - `>24hr`: 30 min
  - `24hr-6hr`: 30 min
  - `6hr-1.5hr`: 10 min
  - `1.5hr-20min`: 2 min
  - `<20min`: 1 min
- Jitter: 5 seconds
- In-play: 60 seconds

## Latency Budget

Mercury component must complete in **<30ms** (per Phase 3 SLO):

| Component | Target | Actual (typical) |
|-----------|--------|------------------|
| Vendor API | N/A | 200-500ms |
| Delta Detection | <1ms | 0.3-0.8ms |
| Batch Queue | <5ms | 1-3ms |
| DB Write | <20ms | 10-18ms |
| Stream Publish | <3ms | 1-2ms |
| **Total** | **<30ms** | **13-24ms** ✅ |

## Data Flow

### 1. Polling
```go
scheduler.pollNBAFeatured()
  → adapter.FetchOdds(sport="basketball_nba", regions=["us","us2"], markets=["h2h","spreads","totals"])
  → Returns []RawOdds
```

### 2. Delta Detection
```go
deltaEngine.DetectChanges(newOdds)
  → Redis MGET odds:current:{event}:{market}:{book}:{outcome}
  → Compare price & point
  → Return []Delta (only changes)
```

### 3. Write Pipeline
```go
writer.Write(deltas)
  → Buffer until 100 rows OR 5 seconds
  → BEGIN TRANSACTION
  →   UPDATE odds_raw SET is_latest=false WHERE ...
  →   INSERT INTO odds_raw VALUES ... (with is_latest=true)
  → COMMIT
  → XADD odds.raw.basketball_nba
  → Redis SET odds:current:...
```

## Monitoring

### Key Metrics
- `mercury_fetch_duration_ms` - Time to fetch from vendor
- `mercury_delta_duration_ms` - Time to detect changes
- `mercury_write_duration_ms` - Time to write to Alexandria
- `mercury_deltas_count` - Number of changes detected
- `mercury_quota_remaining` - Vendor API quota headroom

### Health Check
```bash
curl localhost:8080/health
```

## Development

### Adding a New Sport
1. Create module: `sports/baseball_mlb/`
2. Implement config, markets, validation
3. Add to scheduler
4. Seed Alexandria with sport + markets

### Adding a New Vendor
1. Create adapter: `adapters/newvendor/`
2. Implement `VendorAdapter` interface (FR8)
3. Add conformance tests
4. Configure in scheduler

## Testing

```bash
# Unit tests
go test ./...

# Integration test (requires DB + Redis)
go test -tags=integration ./...

# Load test
go test -run=BenchmarkWriter -bench=. -benchtime=10s
```

## License

Proprietary - Fortuna v0

---

**Status:** ✅ Mercury v0 Complete  
**Last Updated:** 2025-11-06  
**Next:** Integrate with Fortuna normalizer service
