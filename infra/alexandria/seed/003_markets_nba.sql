-- Alexandria Seed Data: NBA Markets
-- Market types for basketball_nba sport (matching The Odds API keys)

INSERT INTO markets (market_key, market_family, display_name, outcome_count, sport_key) VALUES
-- Featured markets (mainlines) - The Odds API uses these exact keys
('h2h', 'featured', 'Moneyline (Head to Head)', 2, 'basketball_nba'),
('spreads', 'featured', 'Point Spread', 2, 'basketball_nba'),
('totals', 'featured', 'Total Points (Over/Under)', 2, 'basketball_nba'),

-- Player props
('player_points', 'props', 'Player Points', 2, 'basketball_nba'),
('player_rebounds', 'props', 'Player Rebounds', 2, 'basketball_nba'),
('player_assists', 'props', 'Player Assists', 2, 'basketball_nba'),
('player_threes', 'props', 'Player 3-Pointers Made', 2, 'basketball_nba'),
('player_points_rebounds_assists', 'props', 'Player Points + Rebounds + Assists', 2, 'basketball_nba'),
('player_points_rebounds', 'props', 'Player Points + Rebounds', 2, 'basketball_nba'),
('player_points_assists', 'props', 'Player Points + Assists', 2, 'basketball_nba'),
('player_rebounds_assists', 'props', 'Player Rebounds + Assists', 2, 'basketball_nba'),
('player_steals', 'props', 'Player Steals', 2, 'basketball_nba'),
('player_blocks', 'props', 'Player Blocks', 2, 'basketball_nba'),
('player_turnovers', 'props', 'Player Turnovers', 2, 'basketball_nba'),
('player_double_double', 'props', 'Player Double Double', 2, 'basketball_nba'),
('player_triple_double', 'props', 'Player Triple Double', 2, 'basketball_nba')
ON CONFLICT (market_key) DO NOTHING;

