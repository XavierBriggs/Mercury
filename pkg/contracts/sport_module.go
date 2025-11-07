package contracts

import (
	"time"

	"github.com/XavierBriggs/Mercury/pkg/models"
)

// SportModule defines the interface for sport-specific polling logic
// This enables Mercury to support multiple sports dynamically
type SportModule interface {
	// GetSportKey returns the unique identifier for this sport (e.g., "basketball_nba")
	GetSportKey() string

	// GetDisplayName returns the human-readable name (e.g., "NBA Basketball")
	GetDisplayName() string

	// GetFeaturedMarkets returns the markets to poll at high frequency
	GetFeaturedMarkets() []string

	// GetRegions returns the regions to poll (e.g., ["us", "us2"])
	GetRegions() []string

	// GetFeaturedPollInterval returns how often to poll featured markets
	GetFeaturedPollInterval() time.Duration

	// GetPropsPollInterval returns how often to poll player props
	GetPropsPollInterval() time.Duration

	// GetPropsDiscoveryInterval returns how often to discover new events
	GetPropsDiscoveryInterval() time.Duration

	// GetPropsDiscoveryWindow returns how many hours ahead to discover events
	GetPropsDiscoveryWindowHours() int

	// ShouldPollProps returns whether this sport supports props polling
	ShouldPollProps() bool

	// ValidateOdds performs sport-specific validation on raw odds
	ValidateOdds(odds models.RawOdds) error
}

