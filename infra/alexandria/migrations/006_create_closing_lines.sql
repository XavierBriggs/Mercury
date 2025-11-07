-- Alexandria DB Migration 006: Closing Lines table
-- Captures closing line prices at game start (for CLV calculation)

CREATE TABLE IF NOT EXISTS closing_lines (
    event_id VARCHAR(100) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    market_key VARCHAR(50) NOT NULL REFERENCES markets(market_key),
    book_key VARCHAR(50) NOT NULL REFERENCES books(book_key),
    outcome_name VARCHAR(200) NOT NULL,
    closing_price INT NOT NULL,
    point DECIMAL(10,2),
    closed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (event_id, market_key, book_key, outcome_name)
);

-- Index for efficient CLV queries
CREATE INDEX idx_closing_lines_event ON closing_lines(event_id);
CREATE INDEX idx_closing_lines_closed_at ON closing_lines(closed_at DESC);

-- Comments
COMMENT ON TABLE closing_lines IS 'Closing line values captured at game start for CLV analysis';
COMMENT ON COLUMN closing_lines.closing_price IS 'American odds at the moment event went live';
COMMENT ON COLUMN closing_lines.closed_at IS 'Timestamp when line was captured (typically event commence_time)';

