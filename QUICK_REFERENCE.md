# Mercury Quick Reference Card

Essential commands and queries for daily Mercury operations.

## 🔌 Connect

```bash
# PostgreSQL
docker exec -it fortuna-alexandria psql -U fortuna -d alexandria

# Redis
docker exec -it fortuna-redis redis-cli -a reddis_pw --no-auth-warning

# Mercury Logs
docker logs -f fortuna-mercury
```

## 📊 Top 10 Queries

### 1. Data Freshness
```sql
SELECT sport_key, MAX(received_at) as last_update, 
       NOW() - MAX(received_at) as age
FROM odds_raw WHERE is_latest = true 
GROUP BY sport_key;
```

### 2. Find Event
```sql
SELECT event_id, home_team, away_team, commence_time
FROM events 
WHERE home_team ILIKE '%Lakers%' 
ORDER BY commence_time DESC LIMIT 5;
```

### 3. Current Odds
```sql
SELECT book_key, outcome_name, price, point
FROM odds_raw 
WHERE event_id = 'YOUR_EVENT_ID' 
  AND market_key = 'h2h' 
  AND is_latest = true;
```

### 4. Best Odds
```sql
SELECT book_key, MAX(price) as best_price
FROM odds_raw 
WHERE event_id = 'EVENT_ID' 
  AND outcome_name = 'Team Name' 
  AND is_latest = true
GROUP BY book_key 
ORDER BY best_price DESC;
```

### 5. Line Movement
```sql
SELECT received_at, book_key, price
FROM odds_raw 
WHERE event_id = 'EVENT_ID' 
  AND market_key = 'h2h' 
  AND outcome_name = 'Team' 
ORDER BY received_at DESC 
LIMIT 20;
```

### 6. Sharp vs Soft
```sql
SELECT b.book_type, o.book_key, o.price
FROM odds_raw o 
JOIN books b ON o.book_key = b.book_key
WHERE o.event_id = 'EVENT_ID' 
  AND o.market_key = 'h2h' 
  AND o.is_latest = true
ORDER BY b.book_type, o.price DESC;
```

### 7. Market Coverage
```sql
SELECT market_key, COUNT(DISTINCT book_key) as books
FROM odds_raw 
WHERE event_id = 'EVENT_ID' 
  AND is_latest = true
GROUP BY market_key;
```

### 8. Book Activity
```sql
SELECT book_key, COUNT(*) as odds_count, 
       MAX(received_at) as last_seen
FROM odds_raw 
WHERE is_latest = true 
GROUP BY book_key 
ORDER BY odds_count DESC;
```

### 9. Stale Events
```sql
SELECT e.event_id, e.home_team || ' vs ' || e.away_team,
       MAX(o.received_at), NOW() - MAX(o.received_at) as stale
FROM events e 
LEFT JOIN odds_raw o ON e.event_id = o.event_id
WHERE e.commence_time > NOW() 
GROUP BY e.event_id, e.home_team, e.away_team
HAVING NOW() - MAX(o.received_at) > INTERVAL '10 minutes';
```

### 10. Database Size
```sql
SELECT pg_size_pretty(pg_database_size('alexandria'));
```

## 🔍 Redis Commands

```bash
# Stream length
XLEN odds.raw.basketball_nba

# Last 10 messages
XREVRANGE odds.raw.basketball_nba + - COUNT 10

# Consumer groups
XINFO GROUPS odds.raw.basketball_nba

# Cache keys
KEYS odds:*

# Database stats
INFO stats
INFO memory
DBSIZE
```

## 🛠️ Common Tasks

### Check Mercury Status
```bash
# Is it running?
docker ps | grep mercury

# Recent logs
docker logs fortuna-mercury --tail=100 --follow

# Check for errors
docker logs fortuna-mercury 2>&1 | grep -i error

# Restart
docker restart fortuna-mercury
```

### Export Data
```bash
# Current odds to CSV
docker exec fortuna-alexandria psql -U fortuna -d alexandria -c "\copy (SELECT * FROM odds_raw WHERE is_latest = true) TO STDOUT WITH CSV HEADER" > odds.csv

# Specific event
docker exec fortuna-alexandria psql -U fortuna -d alexandria -c "SELECT * FROM odds_raw WHERE event_id = 'EVENT_ID'" > event_odds.txt
```

### Clear Old Data
```sql
-- Delete odds older than 7 days (keep is_latest)
DELETE FROM odds_raw 
WHERE received_at < NOW() - INTERVAL '7 days' 
  AND is_latest = false;

-- Vacuum to reclaim space
VACUUM ANALYZE odds_raw;
```

### Reset Everything
```bash
# Stop services
docker-compose down

# Remove volumes (deletes all data!)
docker-compose down -v

# Fresh start
docker-compose up -d
```

## 📈 Performance

```sql
-- Slow queries
SELECT calls, mean_exec_time, query 
FROM pg_stat_statements 
ORDER BY mean_exec_time DESC 
LIMIT 10;

-- Table sizes
SELECT tablename, pg_size_pretty(pg_total_relation_size(tablename::text))
FROM pg_tables 
WHERE schemaname = 'public';

-- Index usage
SELECT indexrelname, idx_scan 
FROM pg_stat_user_indexes 
ORDER BY idx_scan;
```

## 🚨 Troubleshooting

```bash
# Problem: No new data
# 1. Check Mercury is running
docker ps | grep mercury

# 2. Check API key
docker exec fortuna-mercury env | grep ODDS_API_KEY

# 3. Check last poll
docker exec fortuna-alexandria psql -U fortuna -d alexandria -c "SELECT MAX(received_at) FROM odds_raw"

# 4. Check logs for errors
docker logs fortuna-mercury --tail=50

# Problem: Stale cache
# Clear Redis delta cache
docker exec fortuna-redis redis-cli -a reddis_pw FLUSHDB

# Problem: Database full
# Check size
docker exec fortuna-alexandria psql -U fortuna -d alexandria -c "SELECT pg_size_pretty(pg_database_size('alexandria'))"

# Delete old data
docker exec fortuna-alexandria psql -U fortuna -d alexandria -c "DELETE FROM odds_raw WHERE received_at < NOW() - INTERVAL '30 days' AND is_latest = false"
```

## 🔗 Useful Links

- Full Runbooks: [RUNBOOKS.md](./RUNBOOKS.md)
- Mercury README: [README.md](./README.md)
- Alexandria Schema: [infra/alexandria/README.md](./infra/alexandria/README.md)
- Docker Setup: [../deploy/README.md](../deploy/README.md)

## 💡 Pro Tips

1. **Use `\timing` in psql** to measure query speed
2. **Create views** for frequently used queries
3. **Monitor `is_latest` flag** - only one should be true per unique odds combo
4. **Check Redis streams** with `XLEN` to ensure processing isn't backed up
5. **Set up alerts** for stale data (>10 min without updates)
6. **Export data regularly** for offline analysis
7. **Use `EXPLAIN ANALYZE`** to optimize slow queries
8. **Vacuum regularly** to maintain performance

## 📝 Custom Queries Template

```sql
-- Save your frequent queries here

-- My Query 1:


-- My Query 2:


-- My Query 3:

```

---

**Need more help?** See [RUNBOOKS.md](./RUNBOOKS.md) for detailed guides.





