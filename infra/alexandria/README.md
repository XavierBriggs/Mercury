# Alexandria DB

**Source of Truth for Raw Vendor Odds Data**

Alexandria is Mercury's private database storing all raw odds data received from vendor APIs. It follows the "database per service" microservices pattern - Mercury is the **only** writer.

## Schema Overview

### Reference Tables (Seeded)
- `sports` - Supported sports with polling config
- `books` - Sportsbook reference (sharp vs soft)
- `markets` - Market types (h2h, spreads, props, etc.)

### Transactional Tables
- `events` - Discovered sporting events
- `odds_raw` - All raw odds with `is_latest` flag
- `closing_lines` - Closing prices for CLV

## Migrations

Migrations are numbered sequentially and must be applied in order:

```bash
001_create_sports.sql
002_create_events.sql
003_create_books.sql
004_create_markets.sql
005_create_odds_raw.sql
006_create_closing_lines.sql
```

## Seed Data

Seed data must be applied after migrations:

```bash
seed/001_sports.sql      # NBA active, NFL/MLB stubs
seed/002_books.sql       # Sharp (Pinnacle, Circa) + Soft (FanDuel, DK, etc.)
seed/003_markets_nba.sql # h2h, spreads, totals, player props
```

## Running Migrations

### Using Docker Compose
```bash
docker compose exec alexandria-db psql -U fortuna -d alexandria -f /migrations/001_create_sports.sql
# ... run all 6 migrations
docker compose exec alexandria-db psql -U fortuna -d alexandria -f /migrations/seed/001_sports.sql
# ... run all seed files
```

### Using psql directly
```bash
psql $ALEXANDRIA_DSN -f migrations/001_create_sports.sql
psql $ALEXANDRIA_DSN -f migrations/002_create_events.sql
# ... continue through all migrations and seeds
```

## Key Design Decisions

### `is_latest` Flag on `odds_raw`
- Avoids expensive `MAX(received_at) GROUP BY` queries
- Enables fast partial index: `WHERE is_latest = true`
- Mercury writer updates in batches: new rows get `true`, old rows get `false`

### 2NF/3NF Normalization
- No partial dependencies (all attributes depend on full primary key)
- No transitive dependencies (book details in separate table)
- Optimized for write performance (batch inserts) and read performance (indexed queries)

### Composite Keys
- `odds_raw` uses surrogate key (`id BIGSERIAL`) for write speed
- `closing_lines` uses composite natural key (event + market + book + outcome)

## Access Patterns

### Mercury (WRITE)
- Batch insert `odds_raw` rows (100 rows / 5s)
- Update `is_latest` flags in same transaction
- Upsert `events` during discovery sweeps
- Insert `closing_lines` on game start

### Fortuna Services (READ-ONLY)
- Query current odds: `WHERE is_latest = true` (uses partial index)
- Historical odds: `WHERE event_id = X ORDER BY received_at DESC`
- Event lists: `WHERE sport_key = 'basketball_nba' AND event_status = 'upcoming'`

### Primary Data Flow
**Mercury → Redis Streams** (real-time, event-driven)  
**Alexandria** is secondary for historical analysis and system recovery

## Monitoring

Track these metrics:
- `alexandria_write_latency_ms` (batch insert time)
- `alexandria_table_size_gb` (growth rate)
- `alexandria_is_latest_count` (should equal active event × market × book count)

## Future Optimizations (Post-v0)

- Partition `odds_raw` by `received_at` (monthly or weekly)
- Archive old data to S3
- Materialized views for common aggregations
- Read replicas if Fortuna services query too heavily

