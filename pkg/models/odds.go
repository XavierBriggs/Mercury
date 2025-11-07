package models

import "time"

// RawOdds represents raw odds data from a vendor before normalization
type RawOdds struct {
	EventID           string
	SportKey          string
	MarketKey         string
	BookKey           string
	OutcomeName       string
	Price             int       // American odds
	Point             *float64  // For spreads/totals
	VendorLastUpdate  time.Time
	ReceivedAt        time.Time
}

// Event represents a sporting event
type Event struct {
	EventID      string
	SportKey     string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
	EventStatus  string // upcoming, live, completed, cancelled
}

// FetchOddsOptions contains parameters for fetching odds
type FetchOddsOptions struct {
	Sport   string
	Regions []string
	Markets []string
}

// FetchResult contains both events and odds from a fetch operation
type FetchResult struct {
	Events []Event
	Odds   []RawOdds
}

// FetchEventOddsOptions contains parameters for fetching event-specific odds (props)
type FetchEventOddsOptions struct {
	Sport   string
	EventID string
	Regions []string
	Markets []string
}

// RateLimits contains rate limiting information
type RateLimits struct {
	RequestsRemaining int
	RequestsUsed      int
	ResetTime         time.Time
}

