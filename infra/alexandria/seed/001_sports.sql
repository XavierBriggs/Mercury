-- Alexandria Seed Data: Sports
-- Initial sports configuration for v0 (NBA active, others ready for v1)

INSERT INTO sports (sport_key, display_name, active, config) VALUES
('basketball_nba', 'NBA Basketball', true, '{
  "polling": {
    "featured": {
      "pre_match_interval_seconds": 60,
      "ramp_within_hours_to_start": 6,
      "ramp_target_seconds": 40,
      "in_play_interval_seconds": 40
    },
    "props": {
      "discovery_sweep_interval_seconds": 21600,
      "ramp": [
        {"from_hours": 9999, "to_hours": 24, "interval_seconds": 1800},
        {"from_hours": 24, "to_hours": 6, "interval_seconds": 1800},
        {"from_hours": 6, "to_hours": 1.5, "interval_seconds": 600},
        {"from_hours": 1.5, "to_hours": 0.333, "interval_seconds": 120},
        {"from_hours": 0.333, "to_hours": 0, "interval_seconds": 60}
      ],
      "in_play_interval_seconds": 60,
      "jitter_seconds": 5
    }
  }
}'::jsonb),
('american_football_nfl', 'NFL Football', false, null),
('baseball_mlb', 'MLB Baseball', false, null)
ON CONFLICT (sport_key) DO NOTHING;





