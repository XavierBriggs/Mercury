-- Alexandria DB Migration 001: Sports table
-- Creates the sports reference table for multi-sport support

CREATE TABLE IF NOT EXISTS sports (
    sport_key VARCHAR(50) PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT false,
    config JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for active sports queries
CREATE INDEX idx_sports_active ON sports(active) WHERE active = true;

-- Comments for documentation
COMMENT ON TABLE sports IS 'Reference table for supported sports (basketball_nba, american_football_nfl, etc.)';
COMMENT ON COLUMN sports.sport_key IS 'Unique identifier matching vendor sport key';
COMMENT ON COLUMN sports.config IS 'Sport-specific polling configuration (Plan A cadence, etc.)';





