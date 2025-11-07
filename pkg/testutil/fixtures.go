package testutil

import (
	"time"

	"github.com/XavierBriggs/Mercury/pkg/models"
)

// NewTestEvent creates a test event
func NewTestEvent(eventID, homeTeam, awayTeam string, hoursUntilStart float64) models.Event {
	return models.Event{
		EventID:      eventID,
		SportKey:     "basketball_nba",
		HomeTeam:     homeTeam,
		AwayTeam:     awayTeam,
		CommenceTime: time.Now().Add(time.Duration(hoursUntilStart * float64(time.Hour))),
		EventStatus:  "upcoming",
	}
}

// NewTestOdd creates a test odd
func NewTestOdd(eventID, marketKey, bookKey, outcomeName string, price int, point *float64) models.RawOdds {
	now := time.Now()
	return models.RawOdds{
		EventID:          eventID,
		SportKey:         "basketball_nba",
		MarketKey:        marketKey,
		BookKey:          bookKey,
		OutcomeName:      outcomeName,
		Price:            price,
		Point:            point,
		VendorLastUpdate: now,
		ReceivedAt:       now,
	}
}

// GoldenFixtures returns a set of known odds for testing normalization
type GoldenFixture struct {
	Name             string
	Odds             []models.RawOdds
	ExpectedNoVig    map[string]float64 // bookKey -> expected no-vig probability
	ExpectedFairOdds int                // Expected fair American odds
	ExpectedEdge     map[string]float64 // bookKey -> expected edge %
}

// GetGoldenFixtures returns test fixtures with expected outputs
func GetGoldenFixtures() []GoldenFixture {
	return []GoldenFixture{
		{
			Name: "Even Money Spread",
			Odds: []models.RawOdds{
				NewTestOdd("game1", "spreads", "fanduel", "Lakers -3.5", -110, ptrFloat64(-3.5)),
				NewTestOdd("game1", "spreads", "fanduel", "Celtics +3.5", -110, ptrFloat64(3.5)),
			},
			ExpectedNoVig: map[string]float64{
				"fanduel": 0.50, // After removing vig
			},
			ExpectedFairOdds: -100, // True even money
			ExpectedEdge: map[string]float64{
				"fanduel": -4.76, // Negative edge due to vig
			},
		},
		{
			Name: "Favorite Underdog Spread",
			Odds: []models.RawOdds{
				NewTestOdd("game2", "spreads", "draftkings", "Lakers -7.5", -105, ptrFloat64(-7.5)),
				NewTestOdd("game2", "spreads", "draftkings", "Celtics +7.5", -115, ptrFloat64(7.5)),
			},
			ExpectedNoVig: map[string]float64{
				"draftkings": 0.523, // Approximation
			},
			ExpectedFairOdds: -110,
			ExpectedEdge:     map[string]float64{},
		},
		{
			Name: "Totals Market",
			Odds: []models.RawOdds{
				NewTestOdd("game3", "totals", "betmgm", "Over 223.5", -110, ptrFloat64(223.5)),
				NewTestOdd("game3", "totals", "betmgm", "Under 223.5", -110, ptrFloat64(223.5)),
			},
			ExpectedNoVig: map[string]float64{
				"betmgm": 0.50,
			},
			ExpectedFairOdds: -100,
			ExpectedEdge:     map[string]float64{},
		},
		{
			Name: "Sharp vs Soft Edge",
			Odds: []models.RawOdds{
				// Sharp book (Pinnacle) - true market
				NewTestOdd("game4", "h2h", "pinnacle", "Lakers", -105, nil),
				NewTestOdd("game4", "h2h", "pinnacle", "Celtics", -105, nil),
				// Soft book with worse line
				NewTestOdd("game4", "h2h", "fanduel", "Lakers", -115, nil),
				NewTestOdd("game4", "h2h", "fanduel", "Celtics", -105, nil),
			},
			ExpectedFairOdds: -105, // Pinnacle's line is fair
			ExpectedEdge: map[string]float64{
				"fanduel": -4.35, // Lakers side has negative edge vs Pinnacle
			},
		},
	}
}

// ptrFloat64 creates a pointer to float64
func ptrFloat64(val float64) *float64 {
	return &val
}

// MockVendorAdapter is a test adapter that returns predetermined odds
type MockVendorAdapter struct {
	FetchOddsFunc       func() ([]models.RawOdds, error)
	FetchEventOddsFunc  func() ([]models.RawOdds, error)
	FetchEventsFunc     func() ([]models.Event, error)
	SupportsMarketFunc  func(market string) bool
	GetRateLimitsFunc   func() *models.RateLimits
}

func (m *MockVendorAdapter) FetchOdds(ctx interface{}, opts interface{}) ([]models.RawOdds, error) {
	if m.FetchOddsFunc != nil {
		return m.FetchOddsFunc()
	}
	return []models.RawOdds{}, nil
}

func (m *MockVendorAdapter) FetchEventOdds(ctx interface{}, opts interface{}) ([]models.RawOdds, error) {
	if m.FetchEventOddsFunc != nil {
		return m.FetchEventOddsFunc()
	}
	return []models.RawOdds{}, nil
}

func (m *MockVendorAdapter) FetchEvents(ctx interface{}, sport string) ([]models.Event, error) {
	if m.FetchEventsFunc != nil {
		return m.FetchEventsFunc()
	}
	return []models.Event{}, nil
}

func (m *MockVendorAdapter) SupportsMarket(market string) bool {
	if m.SupportsMarketFunc != nil {
		return m.SupportsMarketFunc(market)
	}
	return true
}

func (m *MockVendorAdapter) GetRateLimits() *models.RateLimits {
	if m.GetRateLimitsFunc != nil {
		return m.GetRateLimitsFunc()
	}
	return &models.RateLimits{
		RequestsRemaining: 500,
		RequestsUsed:      0,
	}
}

