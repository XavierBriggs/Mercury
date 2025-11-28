-- Alexandria DB Migration 004: Markets table
-- Reference table for market types (h2h, spreads, totals, props)

CREATE TABLE IF NOT EXISTS markets (
    market_key VARCHAR(50) PRIMARY KEY,
    market_family VARCHAR(20) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    outcome_count INT NOT NULL,
    sport_key VARCHAR(50) NOT NULL REFERENCES sports(sport_key) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_market_family CHECK (market_family IN ('featured', 'props')),
    CONSTRAINT chk_outcome_count CHECK (outcome_count > 0)
);

-- Indexes
CREATE INDEX idx_markets_sport ON markets(sport_key);
CREATE INDEX idx_markets_family ON markets(market_family);

-- Comments
COMMENT ON TABLE markets IS 'Market type reference data (h2h, spreads, totals, player_points, etc.)';
COMMENT ON COLUMN markets.market_key IS 'Unique market identifier (h2h, spreads, totals, player_points, etc.)';
COMMENT ON COLUMN markets.market_family IS 'featured = mainline markets; props = player/game props';
COMMENT ON COLUMN markets.outcome_count IS 'Expected number of outcomes (2 for spreads, 3 for h2h, etc.)';











