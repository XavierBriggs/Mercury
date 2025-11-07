-- Alexandria Seed Data: Books
-- Sportsbook reference data with sharp/soft classification

INSERT INTO books (book_key, display_name, book_type, active, regions, supported_sports) VALUES
-- Sharp books (fast, efficient, low limits, used for fair price baseline)
('pinnacle', 'Pinnacle', 'sharp', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('circa', 'Circa Sports', 'sharp', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('bookmaker', 'Bookmaker', 'sharp', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),

-- Soft books (slower, higher vig, main edge opportunities)
('fanduel', 'FanDuel', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('draftkings', 'DraftKings', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('betmgm', 'BetMGM', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('caesars', 'Caesars Sportsbook', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('pointsbet', 'PointsBet', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('betrivers', 'BetRivers', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('wynnbet', 'WynnBET', 'soft', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('unibet', 'Unibet', 'soft', true, ARRAY['us', 'us2'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('bovada', 'Bovada', 'soft', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb']),
('mybookieag', 'MyBookie.ag', 'soft', true, ARRAY['us'], ARRAY['basketball_nba', 'american_football_nfl', 'baseball_mlb'])
ON CONFLICT (book_key) DO NOTHING;


