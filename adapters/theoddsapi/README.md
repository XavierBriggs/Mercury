# The Odds API Adapter

Official adapter for [The Odds API](https://the-odds-api.com/) - a comprehensive sports odds API covering NBA and other sports.

## API Overview

### Base URL
```
https://api.the-odds-api.com
```

### Authentication
- API Key required (passed as query parameter `apiKey`)
- Get your key at: https://the-odds-api.com/

### Rate Limits
- **Free Tier:** 500 requests/month
- **Paid Tiers:** Higher limits based on plan
- Monitor usage via response headers: `x-requests-remaining`, `x-requests-used`

## NBA Endpoints

### 1. Get Sports List
```
GET /v4/sports
```
Returns list of available sports including `basketball_nba`.

### 2. Get NBA Odds (Featured Markets)
```
GET /v4/sports/basketball_nba/odds
```

**Query Parameters:**
- `apiKey` (required): Your API key
- `regions`: Comma-separated list (e.g., `us,us2,uk`)
- `markets`: Comma-separated list (e.g., `h2h,spreads,totals`)
- `oddsFormat`: `american` (default), `decimal`, or `fractional`
- `dateFormat`: `iso` (default) or `unix`

**Response Structure:**
```json
[
  {
    "id": "event_id_abc123",
    "sport_key": "basketball_nba",
    "sport_title": "NBA",
    "commence_time": "2025-11-07T01:10:00Z",
    "home_team": "Los Angeles Lakers",
    "away_team": "Boston Celtics",
    "bookmakers": [
      {
        "key": "fanduel",
        "title": "FanDuel",
        "last_update": "2025-11-07T00:05:12Z",
        "markets": [
          {
            "key": "h2h",
            "last_update": "2025-11-07T00:05:12Z",
            "outcomes": [
              {
                "name": "Los Angeles Lakers",
                "price": -110
              },
              {
                "name": "Boston Celtics",
                "price": -110
              }
            ]
          },
          {
            "key": "spreads",
            "last_update": "2025-11-07T00:05:12Z",
            "outcomes": [
              {
                "name": "Los Angeles Lakers",
                "price": -110,
                "point": -3.5
              },
              {
                "name": "Boston Celtics",
                "price": -110,
                "point": 3.5
              }
            ]
          },
          {
            "key": "totals",
            "last_update": "2025-11-07T00:05:12Z",
            "outcomes": [
              {
                "name": "Over",
                "price": -110,
                "point": 223.5
              },
              {
                "name": "Under",
                "price": -110,
                "point": 223.5
              }
            ]
          }
        ]
      }
    ]
  }
]
```

### 3. Get NBA Events (Discovery)
```
GET /v4/sports/basketball_nba/events
```
Returns upcoming NBA games without odds (faster, uses fewer quota).

### 4. Get Event Odds (Props)
```
GET /v4/sports/basketball_nba/events/{event_id}/odds
```

**Query Parameters:**
- `apiKey` (required)
- `regions`: e.g., `us,us2`
- `markets`: Player prop markets (see below)

**Player Prop Markets** (based on The Odds API structure):
- `player_points`
- `player_rebounds`
- `player_assists`
- `player_threes` (3-pointers made)
- `player_points_rebounds_assists`
- `player_points_rebounds`
- `player_points_assists`
- `player_rebounds_assists`
- `player_steals`
- `player_blocks`
- `player_turnovers`
- `player_double_double`
- `player_triple_double`

## Market Keys Mapping

### Featured Markets (Always Available)
| API Key | Alexandria market_key | Display Name |
|---------|----------------------|--------------|
| `h2h` | `h2h` | Moneyline |
| `spreads` | `spreads` | Point Spread |
| `totals` | `totals` | Total Points |

### Player Props (Event-Specific)
| API Key | Alexandria market_key | Display Name |
|---------|----------------------|--------------|
| `player_points` | `player_points` | Player Points |
| `player_rebounds` | `player_rebounds` | Player Rebounds |
| `player_assists` | `player_assists` | Player Assists |
| `player_threes` | `player_threes` | Player 3-Pointers Made |

## Regions

The Odds API uses region codes for sportsbook filtering:
- `us`: United States sportsbooks (region 1)
- `us2`: United States sportsbooks (region 2)
- `uk`: United Kingdom sportsbooks
- `au`: Australian sportsbooks
- `eu`: European sportsbooks

**For Fortuna v0:** We use `us` and `us2` as specified in Phase 3.

## Bookmaker Keys

The Odds API uses these keys for major US sportsbooks:
- `fanduel` → FanDuel
- `draftkings` → DraftKings
- `betmgm` → BetMGM
- `caesars` → Caesars
- `pointsbet` → PointsBet
- `betrivers` → BetRivers
- `wynnbet` → WynnBET
- `unibet_us` → Unibet
- `bovada` → Bovada
- `mybookieag` → MyBookie.ag

**Sharp Books (if available in API):**
- `pinnacle` → Pinnacle
- `circa` → Circa Sports
- `bookmaker` → Bookmaker

## Polling Strategy (Plan A from Phase 3)

### Featured Markets (`h2h`, `spreads`, `totals`)
- **Pre-match (>6hr):** 60 seconds
- **Ramping (6hr-0hr):** 60s → 40s linear ramp
- **In-play:** 40 seconds

### Player Props
- **Discovery:** Every 6 hours, call `/events` to find games within 48hr
- **Ramped polling per event:**
  - `>24hr to start`: 30 min
  - `24hr-6hr`: 30 min
  - `6hr-1.5hr`: 10 min
  - `1.5hr-20min`: 2 min
  - `<20min`: 1 min
  - Add 5s jitter to prevent synchronization

## Error Handling

### HTTP Status Codes
- `200`: Success
- `401`: Invalid API key
- `422`: Invalid parameters
- `429`: Rate limit exceeded
- `500`: Server error

### Response Headers
- `x-requests-remaining`: Remaining quota
- `x-requests-used`: Used quota this month

### Retry Strategy
- **429 (Rate Limit):** Implement token bucket, shed far-future events first
- **5xx Errors:** Exponential backoff (1s, 2s, 4s, 8s, stop)
- **Network Errors:** Retry up to 3 times with 2s delay

## Usage in Mercury

```go
adapter := theoddsapi.NewClient(apiKey)

// Featured markets
opts := &FetchOddsOptions{
    Sport:   "basketball_nba",
    Regions: []string{"us", "us2"},
    Markets: []string{"h2h", "spreads", "totals"},
}
odds, err := adapter.FetchOdds(ctx, opts)

// Player props for specific event
propsOpts := &FetchEventOddsOptions{
    Sport:   "basketball_nba",
    EventID: "abc123",
    Regions: []string{"us", "us2"},
    Markets: []string{"player_points", "player_rebounds", "player_assists"},
}
props, err := adapter.FetchEventOdds(ctx, propsOpts)
```

## Data Freshness

- The Odds API updates odds at varying frequencies based on bookmaker data
- `last_update` timestamp in response indicates vendor's last update time
- Mercury tracks this as `vendor_last_update` in Alexandria for staleness calculation
- **Critical:** Always prefer `bookmaker.last_update` over `market.last_update` for accuracy

## Cost Optimization

1. **Use `/events` for discovery** (no odds, cheaper)
2. **Request only needed markets** (don't fetch all markets)
3. **Batch regional queries** (us,us2 in one call vs two separate)
4. **Monitor `x-requests-remaining`** header
5. **Implement quota headroom alerts** (warn at <20%)

## Historical Data

The Odds API provides historical odds:
- **Featured markets:** Available from mid-2020
- **Player props:** Available from May 2023
- Access via separate historical endpoints (not used in v0)

