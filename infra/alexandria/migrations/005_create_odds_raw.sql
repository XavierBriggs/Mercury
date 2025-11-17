-- Alexandria DB Migration 005: Odds Raw table
-- Core table storing all raw vendor odds (source of truth)

CREATE TABLE IF NOT EXISTS odds_raw (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(100) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    sport_key VARCHAR(50) NOT NULL REFERENCES sports(sport_key),
    market_key VARCHAR(50) NOT NULL REFERENCES markets(market_key),
    book_key VARCHAR(50) NOT NULL REFERENCES books(book_key),
    outcome_name VARCHAR(200) NOT NULL,
    price INT NOT NULL,
    point DECIMAL(10,2),
    vendor_last_update TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_latest BOOLEAN NOT NULL DEFAULT true
);

-- Critical indexes for performance
CREATE INDEX idx_odds_raw_latest_odds ON odds_raw(event_id, market_key, book_key, is_latest);
CREATE INDEX idx_odds_raw_received ON odds_raw(received_at DESC);
CREATE INDEX idx_odds_raw_vendor_update ON odds_raw(vendor_last_update DESC);

-- Partial index for fast "current odds" queries (most common use case)
CREATE INDEX idx_odds_raw_current_odds ON odds_raw(event_id, market_key, book_key) 
WHERE is_latest = true;

-- Index for event-based queries
CREATE INDEX idx_odds_raw_event ON odds_raw(event_id, received_at DESC);

-- Comments
COMMENT ON TABLE odds_raw IS 'Raw vendor odds data - source of truth for all odds';
COMMENT ON COLUMN odds_raw.outcome_name IS 'LAL, Over 223.5, LeBron James Over 24.5 Pts, etc.';
COMMENT ON COLUMN odds_raw.price IS 'American odds (e.g., -110, +150)';
COMMENT ON COLUMN odds_raw.point IS 'Spread/total line (e.g., -3.5, 223.5) - NULL for h2h';
COMMENT ON COLUMN odds_raw.vendor_last_update IS 'Timestamp from vendor headers (for staleness tracking)';
COMMENT ON COLUMN odds_raw.is_latest IS 'TRUE for current odds, FALSE for historical. Enables fast queries without MAX(received_at) GROUP BY';





