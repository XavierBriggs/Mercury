package basketball_nba

import (
	"fmt"
	"time"

	"github.com/XavierBriggs/Mercury/pkg/models"
)

// Module implements the SportModule interface for NBA Basketball
type Module struct {
	config *Config
}

// NewModule creates a new NBA sport module
func NewModule() *Module {
	return &Module{
		config: DefaultConfig(),
	}
}

// GetSportKey returns the sport identifier
func (m *Module) GetSportKey() string {
	return m.config.SportKey
}

// GetDisplayName returns the human-readable name
func (m *Module) GetDisplayName() string {
	return m.config.DisplayName
}

// GetFeaturedMarkets returns the featured markets to poll
func (m *Module) GetFeaturedMarkets() []string {
	return FeaturedMarkets()
}

// GetRegions returns the regions to poll
func (m *Module) GetRegions() []string {
	return m.config.Regions
}

// GetFeaturedPollInterval returns the poll interval for featured markets
func (m *Module) GetFeaturedPollInterval() time.Duration {
	return m.config.Featured.PollInterval
}

// GetPropsPollInterval returns the poll interval for props
func (m *Module) GetPropsPollInterval() time.Duration {
	return m.config.Props.PollInterval
}

// GetPropsDiscoveryInterval returns how often to discover new events
func (m *Module) GetPropsDiscoveryInterval() time.Duration {
	return m.config.Props.DiscoverySweepInterval
}

// GetPropsDiscoveryWindowHours returns the discovery window in hours
func (m *Module) GetPropsDiscoveryWindowHours() int {
	return m.config.Props.DiscoveryWindowHours
}

// ShouldPollProps returns whether props polling is enabled
func (m *Module) ShouldPollProps() bool {
	return m.config.Props.Enabled
}

// ValidateOdds performs NBA-specific validation
func (m *Module) ValidateOdds(odds models.RawOdds) error {
	// Validate sport key
	if odds.SportKey != m.config.SportKey {
		return fmt.Errorf("invalid sport_key: expected %s, got %s", m.config.SportKey, odds.SportKey)
	}

	// Validate market key
	validMarkets := make(map[string]bool)
	for _, market := range FeaturedMarkets() {
		validMarkets[market] = true
	}
	for _, market := range PropsMarkets() {
		validMarkets[market] = true
	}

	if !validMarkets[odds.MarketKey] {
		return fmt.Errorf("invalid market_key for NBA: %s", odds.MarketKey)
	}

	// Validate American odds format (should be integer)
	if odds.Price == 0 {
		return fmt.Errorf("invalid price: cannot be 0")
	}

	// Validate spreads/totals have point values
	if (odds.MarketKey == "spreads" || odds.MarketKey == "totals") && odds.Point == nil {
		return fmt.Errorf("market %s requires point value", odds.MarketKey)
	}

	return nil
}











