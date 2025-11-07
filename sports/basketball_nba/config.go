package basketball_nba

import (
	"time"
)

// Config contains NBA-specific polling configuration (Plan A from Phase 3)
type Config struct {
	// Sport identification
	SportKey    string
	DisplayName string

	// Regions to poll
	Regions []string

	// Featured markets configuration (h2h, spreads, totals)
	Featured FeaturedConfig

	// Props markets configuration
	Props PropsConfig
}

// FeaturedConfig defines polling for mainline markets
type FeaturedConfig struct {
	// Default polling interval (used by scheduler)
	PollInterval time.Duration

	// Pre-match polling interval (>6hr from start)
	PreMatchInterval time.Duration

	// How many hours before start to begin ramping
	RampWithinHours float64

	// Target interval near tipoff
	RampTargetInterval time.Duration

	// In-play polling interval
	InPlayInterval time.Duration
}

// PropsConfig defines polling for player props
type PropsConfig struct {
	// Enable props polling
	Enabled bool

	// Default polling interval (used by scheduler)
	PollInterval time.Duration

	// Discovery sweep configuration
	DiscoverySweepInterval time.Duration
	DiscoveryWindowHours   int

	// Time-based ramping tiers
	RampTiers []RampTier

	// In-play interval
	InPlayInterval time.Duration

	// Jitter to prevent synchronization
	JitterSeconds int

	// Capture final snapshot after game ends
	PostGameFinalSnapshot bool
}

// RampTier defines a polling interval based on time to event start
type RampTier struct {
	FromHours float64       // Hours until start (inclusive)
	ToHours   float64       // Hours until start (exclusive)
	Interval  time.Duration // Polling interval
}

// DefaultConfig returns the Plan A configuration from Phase 3
func DefaultConfig() *Config {
	return &Config{
		SportKey:    "basketball_nba",
		DisplayName: "NBA Basketball",
		Regions:     []string{"us", "us2"},

		Featured: FeaturedConfig{
			PollInterval:       60 * time.Second, // Default pre-match interval
			PreMatchInterval:   60 * time.Second,
			RampWithinHours:    6.0,
			RampTargetInterval: 40 * time.Second,
			InPlayInterval:     40 * time.Second,
		},

		Props: PropsConfig{
			Enabled:                true,
			PollInterval:           30 * time.Minute, // Default props interval
			DiscoverySweepInterval: 6 * time.Hour,
			DiscoveryWindowHours:   48,

			RampTiers: []RampTier{
				{FromHours: 9999, ToHours: 24, Interval: 30 * time.Minute},
				{FromHours: 24, ToHours: 6, Interval: 30 * time.Minute},
				{FromHours: 6, ToHours: 1.5, Interval: 10 * time.Minute},
				{FromHours: 1.5, ToHours: 0.333, Interval: 2 * time.Minute}, // 20 min
				{FromHours: 0.333, ToHours: 0, Interval: 1 * time.Minute},
			},

			InPlayInterval:        60 * time.Second,
			JitterSeconds:         5,
			PostGameFinalSnapshot: true,
		},
	}
}

// GetFeaturedInterval returns the appropriate polling interval for featured markets
// based on hours until event start
func (c *Config) GetFeaturedInterval(hoursUntilStart float64, isLive bool) time.Duration {
	if isLive {
		return c.Featured.InPlayInterval
	}

	if hoursUntilStart > c.Featured.RampWithinHours {
		return c.Featured.PreMatchInterval
	}

	// Linear ramp from PreMatchInterval to RampTargetInterval
	rampFactor := hoursUntilStart / c.Featured.RampWithinHours
	diff := c.Featured.PreMatchInterval - c.Featured.RampTargetInterval
	return c.Featured.RampTargetInterval + time.Duration(float64(diff)*rampFactor)
}

// GetPropsInterval returns the appropriate polling interval for props
// based on hours until event start
func (c *Config) GetPropsInterval(hoursUntilStart float64, isLive bool) time.Duration {
	if isLive {
		return c.Props.InPlayInterval
	}

	// Find matching tier
	for _, tier := range c.Props.RampTiers {
		if hoursUntilStart >= tier.ToHours && hoursUntilStart < tier.FromHours {
			return tier.Interval
		}
	}

	// Default to fastest tier if somehow outside range
	return c.Props.RampTiers[len(c.Props.RampTiers)-1].Interval
}

