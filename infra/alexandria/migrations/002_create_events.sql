-- Alexandria DB Migration 002: Events table
-- Stores discovered sporting events from vendors

CREATE TABLE IF NOT EXISTS events (
    event_id VARCHAR(100) PRIMARY KEY,
    sport_key VARCHAR(50) NOT NULL REFERENCES sports(sport_key) ON DELETE CASCADE,
    home_team VARCHAR(100) NOT NULL,
    away_team VARCHAR(100) NOT NULL,
    commence_time TIMESTAMPTZ NOT NULL,
    event_status VARCHAR(20) NOT NULL DEFAULT 'upcoming',
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_event_status CHECK (event_status IN ('upcoming', 'live', 'completed', 'cancelled'))
);

-- Indexes for common query patterns
CREATE INDEX idx_events_sport_commence ON events(sport_key, commence_time);
CREATE INDEX idx_events_status_commence ON events(event_status, commence_time);
CREATE INDEX idx_events_last_seen ON events(last_seen_at DESC);

-- Comments
COMMENT ON TABLE events IS 'Sporting events discovered from vendor APIs';
COMMENT ON COLUMN events.event_id IS 'Vendor-provided unique event identifier';
COMMENT ON COLUMN events.event_status IS 'Current status: upcoming, live, completed, cancelled';
COMMENT ON COLUMN events.last_seen_at IS 'Last time this event was seen in vendor responses (for cleanup)';




