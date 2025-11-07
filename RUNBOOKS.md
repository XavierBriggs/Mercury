# Mercury Data Runbooks

Practical guides for querying, analyzing, and troubleshooting Mercury's odds data in Alexandria DB and Redis.

## Table of Contents

1. [Database Access](#database-access)
2. [Common Queries](#common-queries)
3. [Data Analysis](#data-analysis)
4. [Redis Operations](#redis-operations)
5. [Data Quality Checks](#data-quality-checks)
6. [Performance Monitoring](#performance-monitoring)
7. [Troubleshooting](#troubleshooting)
8. [Data Export](#data-export)

---

## Database Access

### Connect to Alexandria DB

**Docker (Development):**
```bash
# Interactive psql shell
docker exec -it fortuna-alexandria psql -U fortuna -d alexandria

# Single query
docker exec -it fortuna-alexandria psql -U fortuna -d alexandria -c "SELECT COUNT(*) FROM odds_raw;"

# Run SQL file
docker exec -i fortuna-alexandria psql -U fortuna -d alexandria < query.sql
```

**Local (Development):**
```bash
# Using psql
psql -h localhost -p 5435 -U fortuna -d alexandria

# Using connection string
psql "postgres://fortuna:fortuna_dev_password@localhost:5435/alexandria?sslmode=disable"
```

**Environment Variable:**
```bash
export ALEXANDRIA_DSN="postgres://fortuna:fortuna_dev_password@localhost:5435/alexandria?sslmode=disable"
psql $ALEXANDRIA_DSN
```

---

## Common Queries

### 1. Check Data Freshness

**How recent is our data?**
```sql
-- Most recent odds by sport
SELECT 
    sport_key,
    MAX(received_at) as last_update,
    NOW() - MAX(received_at) as age,
    COUNT(*) as total_odds
FROM odds_raw
WHERE is_latest = true
GROUP BY sport_key
ORDER BY last_update DESC;
```

**Expected Output:**
```
 sport_key      | last_update              | age         | total_odds
----------------+--------------------------+-------------+-----------
 basketball_nba | 2025-11-07 21:05:00+00   | 00:02:15    | 1247
```

### 2. Get Current Odds for a Game

**View all current odds for a specific event:**
```sql
SELECT 
    e.home_team,
    e.away_team,
    e.commence_time,
    o.market_key,
    o.book_key,
    o.outcome_name,
    o.price,
    o.point,
    o.vendor_last_update,
    NOW() - o.received_at as data_age
FROM odds_raw o
JOIN events e ON o.event_id = e.event_id
WHERE o.event_id = 'YOUR_EVENT_ID'
  AND o.is_latest = true
ORDER BY o.market_key, o.book_key, o.outcome_name;
```

**Find event by team:**
```sql
SELECT 
    event_id,
    home_team,
    away_team,
    commence_time,
    event_status
FROM events
WHERE home_team ILIKE '%Lakers%' 
   OR away_team ILIKE '%Lakers%'
ORDER BY commence_time DESC
LIMIT 5;
```

### 3. Compare Odds Across Books

**Find best odds for a specific outcome:**
```sql
WITH latest_odds AS (
    SELECT 
        o.event_id,
        o.market_key,
        o.outcome_name,
        o.book_key,
        o.price,
        o.point,
        b.book_type,
        o.vendor_last_update
    FROM odds_raw o
    JOIN books b ON o.book_key = b.book_key
    WHERE o.is_latest = true
      AND o.event_id = 'YOUR_EVENT_ID'
      AND o.market_key = 'h2h'
      AND o.outcome_name = 'Los Angeles Lakers'
)
SELECT 
    book_key,
    book_type,
    price,
    vendor_last_update,
    -- Convert American to implied probability
    CASE 
        WHEN price > 0 THEN ROUND(100.0 / (price + 100), 4)
        ELSE ROUND(ABS(price) / (ABS(price) + 100), 4)
    END as implied_prob
FROM latest_odds
ORDER BY price DESC;  -- Best odds at top (most positive or least negative)
```

### 4. Line Movement History

**Track how odds changed over time:**
```sql
SELECT 
    book_key,
    outcome_name,
    price,
    point,
    received_at,
    vendor_last_update,
    LAG(price) OVER (PARTITION BY book_key, outcome_name ORDER BY received_at) as prev_price,
    price - LAG(price) OVER (PARTITION BY book_key, outcome_name ORDER BY received_at) as price_change
FROM odds_raw
WHERE event_id = 'YOUR_EVENT_ID'
  AND market_key = 'spreads'
ORDER BY outcome_name, book_key, received_at;
```

### 5. Sharp vs Soft Book Comparison

**Compare sharp (Pinnacle, Circa) vs soft books:**
```sql
SELECT 
    b.book_type,
    b.book_key,
    o.outcome_name,
    o.price,
    CASE 
        WHEN o.price > 0 THEN ROUND(100.0 / (o.price + 100), 4)
        ELSE ROUND(ABS(o.price) / (ABS(o.price) + 100), 4)
    END as implied_prob
FROM odds_raw o
JOIN books b ON o.book_key = b.book_key
WHERE o.event_id = 'YOUR_EVENT_ID'
  AND o.market_key = 'h2h'
  AND o.is_latest = true
ORDER BY b.book_type, o.outcome_name;
```

### 6. Market Overview

**Get all markets available for an event:**
```sql
SELECT 
    m.market_key,
    m.market_name,
    m.market_type,
    COUNT(DISTINCT o.book_key) as num_books,
    COUNT(DISTINCT o.outcome_name) as num_outcomes,
    MAX(o.received_at) as last_update
FROM odds_raw o
JOIN markets m ON o.market_key = m.market_key
WHERE o.event_id = 'YOUR_EVENT_ID'
  AND o.is_latest = true
GROUP BY m.market_key, m.market_name, m.market_type
ORDER BY 
    CASE m.market_type
        WHEN 'featured' THEN 1
        WHEN 'props' THEN 2
        ELSE 3
    END,
    m.market_key;
```

---

## Data Analysis

### 1. Book Coverage Analysis

**Which books provide the most markets?**
```sql
SELECT 
    b.book_key,
    b.display_name,
    b.book_type,
    COUNT(DISTINCT o.market_key) as markets_offered,
    COUNT(DISTINCT o.event_id) as events_covered,
    COUNT(*) as total_odds,
    MAX(o.received_at) as last_update
FROM odds_raw o
JOIN books b ON o.book_key = b.book_key
WHERE o.sport_key = 'basketball_nba'
  AND o.is_latest = true
GROUP BY b.book_key, b.display_name, b.book_type
ORDER BY markets_offered DESC;
```

### 2. Vig Analysis

**Calculate implied probability total (vig) per market:**
```sql
WITH market_odds AS (
    SELECT 
        o.event_id,
        o.market_key,
        o.book_key,
        SUM(
            CASE 
                WHEN o.price > 0 THEN 100.0 / (o.price + 100)
                ELSE ABS(o.price) / (ABS(o.price) + 100)
            END
        ) as total_implied_prob,
        COUNT(*) as num_outcomes
    FROM odds_raw o
    WHERE o.is_latest = true
      AND o.market_key IN ('h2h', 'spreads', 'totals')
      AND o.sport_key = 'basketball_nba'
    GROUP BY o.event_id, o.market_key, o.book_key
    HAVING COUNT(*) = 2  -- Two-way markets only
)
SELECT 
    book_key,
    market_key,
    COUNT(*) as num_markets,
    ROUND(AVG(total_implied_prob), 4) as avg_total_prob,
    ROUND(MIN(total_implied_prob), 4) as best_line,
    ROUND(MAX(total_implied_prob), 4) as worst_line,
    ROUND(AVG(total_implied_prob - 1.0) * 100, 2) as avg_vig_pct
FROM market_odds
GROUP BY book_key, market_key
ORDER BY book_key, market_key;
```

**Interpretation:**
- `total_implied_prob` = 1.00: No vig (theoretical)
- `total_implied_prob` = 1.05: 5% vig (typical)
- `total_implied_prob` = 1.10: 10% vig (high)

### 3. Props Market Analysis

**Most common player props:**
```sql
SELECT 
    market_key,
    market_name,
    COUNT(DISTINCT event_id) as num_events,
    COUNT(DISTINCT outcome_name) as num_players,
    COUNT(DISTINCT book_key) as num_books,
    AVG(CASE WHEN point IS NOT NULL THEN point ELSE 0 END) as avg_line
FROM odds_raw o
JOIN markets m ON o.market_key = m.market_key
WHERE m.market_type = 'props'
  AND o.is_latest = true
  AND o.sport_key = 'basketball_nba'
GROUP BY market_key, market_name
ORDER BY num_events DESC, num_players DESC
LIMIT 20;
```

### 4. Data Latency Analysis

**How fast is Mercury polling?**
```sql
SELECT 
    sport_key,
    DATE_TRUNC('hour', received_at) as hour,
    COUNT(*) as polls,
    COUNT(DISTINCT event_id) as events,
    AVG(EXTRACT(EPOCH FROM (received_at - vendor_last_update))) as avg_latency_seconds
FROM odds_raw
WHERE received_at > NOW() - INTERVAL '24 hours'
GROUP BY sport_key, DATE_TRUNC('hour', received_at)
ORDER BY hour DESC;
```

### 5. Line Movement Velocity

**Identify fast-moving lines:**
```sql
WITH line_changes AS (
    SELECT 
        event_id,
        market_key,
        book_key,
        outcome_name,
        received_at,
        price,
        LAG(price) OVER (PARTITION BY event_id, market_key, book_key, outcome_name ORDER BY received_at) as prev_price,
        received_at - LAG(received_at) OVER (PARTITION BY event_id, market_key, book_key, outcome_name ORDER BY received_at) as time_diff
    FROM odds_raw
    WHERE received_at > NOW() - INTERVAL '24 hours'
)
SELECT 
    e.home_team || ' vs ' || e.away_team as matchup,
    lc.market_key,
    lc.book_key,
    lc.outcome_name,
    lc.prev_price,
    lc.price as current_price,
    lc.price - lc.prev_price as price_change,
    EXTRACT(EPOCH FROM lc.time_diff) / 60 as minutes_elapsed
FROM line_changes lc
JOIN events e ON lc.event_id = e.event_id
WHERE lc.prev_price IS NOT NULL
  AND ABS(lc.price - lc.prev_price) >= 10  -- 10+ point move
ORDER BY ABS(lc.price - lc.prev_price) DESC
LIMIT 20;
```

---

## Redis Operations

### Connect to Redis

```bash
# Docker
docker exec -it fortuna-redis redis-cli -a reddis_pw

# Local
redis-cli -h localhost -p 6380 -a reddis_pw

# Disable auth warnings
redis-cli -h localhost -p 6380 -a reddis_pw --no-auth-warning
```

### Inspect Streams

**List all streams:**
```bash
redis-cli -a reddis_pw --no-auth-warning KEYS "odds.*"
```

**Get stream info:**
```bash
# Stream length
redis-cli -a reddis_pw --no-auth-warning XLEN odds.raw.basketball_nba

# Stream info
redis-cli -a reddis_pw --no-auth-warning XINFO STREAM odds.raw.basketball_nba

# Consumer groups
redis-cli -a reddis_pw --no-auth-warning XINFO GROUPS odds.raw.basketball_nba

# Consumer group status
redis-cli -a reddis_pw --no-auth-warning XINFO CONSUMERS odds.raw.basketball_nba normalizer
```

**Read recent messages:**
```bash
# Last 10 messages
redis-cli -a reddis_pw --no-auth-warning XREVRANGE odds.raw.basketball_nba + - COUNT 10

# Messages in last hour (approximate)
redis-cli -a reddis_pw --no-auth-warning XREAD COUNT 100 STREAMS odds.raw.basketball_nba 0
```

**Inspect specific message:**
```bash
# Read by message ID
redis-cli -a reddis_pw --no-auth-warning XRANGE odds.raw.basketball_nba MESSAGE_ID MESSAGE_ID
```

### Monitor Real-Time

**Watch all commands:**
```bash
redis-cli -a reddis_pw --no-auth-warning MONITOR
```

**Filter for stream operations:**
```bash
redis-cli -a reddis_pw --no-auth-warning MONITOR | grep XADD
```

### Delta Cache Inspection

**Check cached odds:**
```bash
# List all cache keys
redis-cli -a reddis_pw --no-auth-warning KEYS "odds:*"

# Get specific cached odds
redis-cli -a reddis_pw --no-auth-warning GET "odds:event123:h2h:fanduel:Lakers"

# Check TTL
redis-cli -a reddis_pw --no-auth-warning TTL "odds:event123:h2h:fanduel:Lakers"

# Count cached entries
redis-cli -a reddis_pw --no-auth-warning DBSIZE
```

### Clean Up Streams

**Trim old messages (keep last 10,000):**
```bash
redis-cli -a reddis_pw --no-auth-warning XTRIM odds.raw.basketball_nba MAXLEN ~ 10000
redis-cli -a reddis_pw --no-auth-warning XTRIM odds.normalized.basketball_nba MAXLEN ~ 10000
```

**Delete consumer group:**
```bash
redis-cli -a reddis_pw --no-auth-warning XGROUP DESTROY odds.raw.basketball_nba normalizer
```

---

## Data Quality Checks

### 1. Check for Stale Data

```sql
-- Events with no recent odds updates
SELECT 
    e.event_id,
    e.home_team || ' vs ' || e.away_team as matchup,
    e.commence_time,
    MAX(o.received_at) as last_odds_update,
    NOW() - MAX(o.received_at) as staleness,
    COUNT(DISTINCT o.book_key) as active_books
FROM events e
LEFT JOIN odds_raw o ON e.event_id = o.event_id AND o.is_latest = true
WHERE e.event_status = 'upcoming'
  AND e.commence_time > NOW()
  AND e.commence_time < NOW() + INTERVAL '7 days'
GROUP BY e.event_id, e.home_team, e.away_team, e.commence_time
HAVING MAX(o.received_at) < NOW() - INTERVAL '5 minutes'
   OR MAX(o.received_at) IS NULL
ORDER BY e.commence_time;
```

### 2. Detect Missing Markets

```sql
-- Events missing expected markets
SELECT 
    e.event_id,
    e.home_team || ' vs ' || e.away_team as matchup,
    e.commence_time,
    ARRAY_AGG(DISTINCT o.market_key ORDER BY o.market_key) as available_markets,
    CASE 
        WHEN 'h2h' = ANY(ARRAY_AGG(DISTINCT o.market_key)) THEN '✓' 
        ELSE '✗' 
    END as has_moneyline,
    CASE 
        WHEN 'spreads' = ANY(ARRAY_AGG(DISTINCT o.market_key)) THEN '✓' 
        ELSE '✗' 
    END as has_spreads,
    CASE 
        WHEN 'totals' = ANY(ARRAY_AGG(DISTINCT o.market_key)) THEN '✓' 
        ELSE '✗' 
    END as has_totals
FROM events e
LEFT JOIN odds_raw o ON e.event_id = o.event_id AND o.is_latest = true
WHERE e.event_status = 'upcoming'
  AND e.commence_time > NOW()
  AND e.commence_time < NOW() + INTERVAL '3 days'
GROUP BY e.event_id, e.home_team, e.away_team, e.commence_time
HAVING COUNT(DISTINCT o.market_key) < 3  -- Expected: h2h, spreads, totals
ORDER BY e.commence_time;
```

### 3. Identify Outlier Odds

```sql
-- Find odds that deviate significantly from average
WITH market_averages AS (
    SELECT 
        event_id,
        market_key,
        outcome_name,
        AVG(price) as avg_price,
        STDDEV(price) as std_dev,
        COUNT(*) as num_books
    FROM odds_raw
    WHERE is_latest = true
      AND market_key IN ('h2h', 'spreads', 'totals')
    GROUP BY event_id, market_key, outcome_name
    HAVING COUNT(*) >= 3  -- At least 3 books for comparison
)
SELECT 
    e.home_team || ' vs ' || e.away_team as matchup,
    o.market_key,
    o.book_key,
    o.outcome_name,
    o.price as actual_price,
    ROUND(ma.avg_price) as avg_price,
    ROUND(o.price - ma.avg_price) as deviation,
    ROUND(ABS(o.price - ma.avg_price) / NULLIF(ma.std_dev, 0), 2) as std_devs_from_mean
FROM odds_raw o
JOIN market_averages ma ON o.event_id = ma.event_id 
    AND o.market_key = ma.market_key 
    AND o.outcome_name = ma.outcome_name
JOIN events e ON o.event_id = e.event_id
WHERE o.is_latest = true
  AND ABS(o.price - ma.avg_price) > 20  -- 20+ point deviation
ORDER BY ABS(o.price - ma.avg_price) DESC
LIMIT 50;
```

### 4. Verify Data Completeness

```sql
-- Check data coverage by hour
SELECT 
    DATE_TRUNC('hour', received_at) as hour,
    COUNT(*) as total_odds,
    COUNT(DISTINCT event_id) as unique_events,
    COUNT(DISTINCT book_key) as unique_books,
    COUNT(DISTINCT market_key) as unique_markets
FROM odds_raw
WHERE received_at > NOW() - INTERVAL '48 hours'
GROUP BY DATE_TRUNC('hour', received_at)
ORDER BY hour DESC;
```

---

## Performance Monitoring

### 1. Database Size

```sql
-- Total database size
SELECT pg_size_pretty(pg_database_size('alexandria'));

-- Table sizes
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as total_size,
    pg_size_pretty(pg_relation_size(schemaname||'.'||tablename)) as table_size,
    pg_size_pretty(pg_indexes_size(schemaname||'.'||tablename)) as indexes_size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

### 2. Table Statistics

```sql
-- Row counts and dead tuples
SELECT 
    schemaname,
    relname,
    n_live_tup as live_rows,
    n_dead_tup as dead_rows,
    last_vacuum,
    last_autovacuum,
    last_analyze,
    last_autoanalyze
FROM pg_stat_user_tables
WHERE schemaname = 'public'
ORDER BY n_live_tup DESC;
```

### 3. Index Usage

```sql
-- Index usage statistics
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_scan as index_scans,
    idx_tup_read as tuples_read,
    idx_tup_fetch as tuples_fetched,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
ORDER BY idx_scan ASC;  -- Least used indexes at top
```

### 4. Query Performance

```sql
-- Slow queries (requires pg_stat_statements extension)
SELECT 
    calls,
    ROUND(total_exec_time::numeric, 2) as total_time_ms,
    ROUND(mean_exec_time::numeric, 2) as avg_time_ms,
    ROUND(max_exec_time::numeric, 2) as max_time_ms,
    LEFT(query, 100) as query_preview
FROM pg_stat_statements
WHERE query NOT LIKE '%pg_stat%'
ORDER BY mean_exec_time DESC
LIMIT 20;
```

---

## Troubleshooting

### Problem: No New Odds Data

**Check 1: Is Mercury running?**
```bash
docker ps | grep mercury
docker logs fortuna-mercury --tail=50
```

**Check 2: Check last poll time**
```sql
SELECT 
    sport_key,
    MAX(received_at) as last_poll,
    NOW() - MAX(received_at) as time_since_last,
    COUNT(*) as odds_count
FROM odds_raw
GROUP BY sport_key;
```

**Check 3: Verify API key**
```bash
docker exec fortuna-mercury env | grep ODDS_API_KEY
```

**Check 4: Check for errors in logs**
```bash
docker logs fortuna-mercury 2>&1 | grep -i error
```

### Problem: Stale Odds

**Check Delta Cache TTL:**
```bash
# Should be > poll interval (60s)
docker exec fortuna-mercury env | grep MERCURY_CACHE_TTL
```

**Clear Delta Cache:**
```bash
redis-cli -a reddis_pw --no-auth-warning FLUSHDB
```

**Verify is_latest flag:**
```sql
-- Should have only one is_latest=true per unique odds combo
SELECT 
    event_id,
    market_key,
    book_key,
    outcome_name,
    COUNT(*) as count,
    SUM(CASE WHEN is_latest THEN 1 ELSE 0 END) as latest_count
FROM odds_raw
GROUP BY event_id, market_key, book_key, outcome_name
HAVING SUM(CASE WHEN is_latest THEN 1 ELSE 0 END) > 1;
```

### Problem: Missing Books

**Check which books are available:**
```sql
SELECT 
    b.book_key,
    b.display_name,
    CASE 
        WHEN EXISTS (
            SELECT 1 FROM odds_raw o 
            WHERE o.book_key = b.book_key 
              AND o.received_at > NOW() - INTERVAL '1 hour'
        ) THEN '✓ Active'
        ELSE '✗ Inactive'
    END as status
FROM books b
ORDER BY b.book_key;
```

**Check TheOddsAPI response:**
```bash
# Make test API call
curl "https://api.the-odds-api.com/v4/sports/basketball_nba/odds?apiKey=YOUR_KEY&regions=us&markets=h2h"
```

---

## Data Export

### 1. Export to CSV

**Export current odds:**
```sql
\copy (SELECT e.home_team, e.away_team, e.commence_time, o.market_key, o.book_key, o.outcome_name, o.price, o.point FROM odds_raw o JOIN events e ON o.event_id = e.event_id WHERE o.is_latest = true AND o.sport_key = 'basketball_nba') TO '/tmp/current_odds.csv' WITH CSV HEADER;
```

**Export line movement history:**
```sql
\copy (SELECT e.home_team || ' vs ' || e.away_team as matchup, o.market_key, o.book_key, o.outcome_name, o.price, o.received_at FROM odds_raw o JOIN events e ON o.event_id = e.event_id WHERE o.event_id = 'YOUR_EVENT_ID' ORDER BY o.received_at) TO '/tmp/line_history.csv' WITH CSV HEADER;
```

### 2. Export to JSON

```bash
# Using psql
psql $ALEXANDRIA_DSN -c "SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM odds_raw WHERE is_latest = true LIMIT 100) t;" -t > odds.json

# Using jq for pretty printing
psql $ALEXANDRIA_DSN -t -c "SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM odds_raw WHERE is_latest = true LIMIT 10) t;" | jq '.' > odds_pretty.json
```

### 3. Backup Database

```bash
# Full backup
docker exec fortuna-alexandria pg_dump -U fortuna alexandria > alexandria_backup.sql

# Schema only
docker exec fortuna-alexandria pg_dump -U fortuna -s alexandria > alexandria_schema.sql

# Data only
docker exec fortuna-alexandria pg_dump -U fortuna -a alexandria > alexandria_data.sql

# Specific table
docker exec fortuna-alexandria pg_dump -U fortuna -t odds_raw alexandria > odds_raw_backup.sql
```

### 4. Export for Analysis (Python/Pandas)

```python
import psycopg2
import pandas as pd

# Connect
conn = psycopg2.connect(
    host="localhost",
    port=5435,
    database="alexandria",
    user="fortuna",
    password="fortuna_dev_password"
)

# Query current odds
df = pd.read_sql("""
    SELECT 
        e.home_team,
        e.away_team,
        e.commence_time,
        o.market_key,
        o.book_key,
        o.outcome_name,
        o.price,
        o.point
    FROM odds_raw o
    JOIN events e ON o.event_id = e.event_id
    WHERE o.is_latest = true
      AND o.sport_key = 'basketball_nba'
""", conn)

# Save to parquet for efficient storage
df.to_parquet('current_odds.parquet', compression='gzip')

# Or CSV
df.to_csv('current_odds.csv', index=False)

conn.close()
```

---

## Quick Reference

### Most Useful Queries

```sql
-- 1. Current odds for next game
SELECT * FROM odds_raw o 
JOIN events e ON o.event_id = e.event_id 
WHERE o.is_latest = true 
  AND e.commence_time > NOW() 
ORDER BY e.commence_time 
LIMIT 100;

-- 2. Best available odds
SELECT outcome_name, book_key, MAX(price) 
FROM odds_raw 
WHERE is_latest = true AND event_id = 'EVENT_ID' 
GROUP BY outcome_name, book_key;

-- 3. Data freshness
SELECT sport_key, MAX(received_at), NOW() - MAX(received_at) 
FROM odds_raw 
WHERE is_latest = true 
GROUP BY sport_key;

-- 4. Book coverage
SELECT book_key, COUNT(*) 
FROM odds_raw 
WHERE is_latest = true 
GROUP BY book_key 
ORDER BY COUNT(*) DESC;

-- 5. Line movement
SELECT book_key, price, received_at 
FROM odds_raw 
WHERE event_id = 'EVENT_ID' 
  AND market_key = 'h2h' 
  AND outcome_name = 'Team Name' 
ORDER BY received_at;
```

### Useful psql Commands

```sql
\dt              -- List tables
\d odds_raw      -- Describe table
\di              -- List indexes
\l               -- List databases
\dn              -- List schemas
\df              -- List functions
\x               -- Toggle expanded display
\timing          -- Show query execution time
\q               -- Quit
```

---

## Advanced Topics

### Create Materialized Views

**Pre-compute expensive queries:**
```sql
-- Current odds summary
CREATE MATERIALIZED VIEW current_odds_summary AS
SELECT 
    e.event_id,
    e.home_team,
    e.away_team,
    e.commence_time,
    o.market_key,
    COUNT(DISTINCT o.book_key) as num_books,
    MAX(o.received_at) as last_update
FROM events e
JOIN odds_raw o ON e.event_id = o.event_id
WHERE o.is_latest = true
  AND e.event_status = 'upcoming'
GROUP BY e.event_id, e.home_team, e.away_team, e.commence_time, o.market_key;

-- Refresh periodically
REFRESH MATERIALIZED VIEW current_odds_summary;
```

### Custom Functions

**Calculate implied probability:**
```sql
CREATE OR REPLACE FUNCTION implied_probability(american_odds INT)
RETURNS DECIMAL(10,4) AS $$
BEGIN
    IF american_odds > 0 THEN
        RETURN ROUND(100.0 / (american_odds + 100), 4);
    ELSE
        RETURN ROUND(ABS(american_odds) / (ABS(american_odds) + 100), 4);
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Usage
SELECT outcome_name, price, implied_probability(price) 
FROM odds_raw 
WHERE is_latest = true 
LIMIT 10;
```

---

## See Also

- [Mercury README](./README.md) - Service documentation
- [Alexandria DB Schema](./infra/alexandria/README.md) - Database design
- [Testing Guide](./TESTING.md) - Test documentation
- [Deployment Guide](../deploy/README.md) - Docker Compose setup

---

**Pro Tips:**
- Use `\timing` in psql to measure query performance
- Create indexes on frequently queried columns
- Use `EXPLAIN ANALYZE` to understand query plans
- Set up views for commonly used queries
- Export data regularly for offline analysis
- Monitor database size and vacuum regularly

Happy querying! 📊

