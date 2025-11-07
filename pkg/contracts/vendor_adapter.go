package contracts

import (
	"context"

	"github.com/XavierBriggs/Mercury/pkg/models"
)

// VendorAdapter defines the interface for fetching odds from external vendors
// This is FR8 requirement: stable interface for future in-house odds aggregators
type VendorAdapter interface {
	// FetchOdds retrieves odds for featured markets (h2h, spreads, totals)
	// Returns both events and odds to enable proper event upsertion
	FetchOdds(ctx context.Context, opts *models.FetchOddsOptions) (*models.FetchResult, error)

	// FetchEventOdds retrieves odds for a specific event (for props markets)
	// Returns both event and odds to enable proper event upsertion
	FetchEventOdds(ctx context.Context, opts *models.FetchEventOddsOptions) (*models.FetchResult, error)

	// FetchEvents retrieves upcoming events without odds (for discovery)
	FetchEvents(ctx context.Context, sport string) ([]models.Event, error)

	// SupportsMarket checks if this adapter supports a given market
	SupportsMarket(market string) bool

	// GetRateLimits returns current rate limit information
	GetRateLimits() *models.RateLimits
}

