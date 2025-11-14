-- Alexandria DB Migration 003: Books table
-- Reference table for sportsbooks (sharp vs soft classification)

CREATE TABLE IF NOT EXISTS books (
    book_key VARCHAR(50) PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    book_type VARCHAR(20) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    regions VARCHAR(10)[] NOT NULL,
    supported_sports VARCHAR(50)[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_book_type CHECK (book_type IN ('sharp', 'soft'))
);

-- Index for active books
CREATE INDEX idx_books_active ON books(active) WHERE active = true;
CREATE INDEX idx_books_type ON books(book_type);

-- Comments
COMMENT ON TABLE books IS 'Sportsbook reference data with sharp/soft classification';
COMMENT ON COLUMN books.book_key IS 'Unique identifier matching vendor book key (fanduel, pinnacle, etc.)';
COMMENT ON COLUMN books.book_type IS 'sharp = fast, efficient, low limits; soft = slower, higher vig';
COMMENT ON COLUMN books.regions IS 'Array of regions where book operates (us, us2, uk, etc.)';
COMMENT ON COLUMN books.supported_sports IS 'Array of sport_keys this book covers';




