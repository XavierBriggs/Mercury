package sports_test

import (
	"testing"
	"time"

	"github.com/XavierBriggs/Mercury/sports/basketball_nba"
)

func TestDefaultConfig(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	if config.SportKey != "basketball_nba" {
		t.Errorf("expected sport_key basketball_nba, got %s", config.SportKey)
	}

	if len(config.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(config.Regions))
	}

	if config.Featured.PreMatchInterval != 60*time.Second {
		t.Errorf("expected pre-match interval 60s, got %v", config.Featured.PreMatchInterval)
	}

	if config.Featured.RampTargetInterval != 40*time.Second {
		t.Errorf("expected ramp target 40s, got %v", config.Featured.RampTargetInterval)
	}

	if len(config.Props.RampTiers) != 5 {
		t.Errorf("expected 5 prop ramp tiers, got %d", len(config.Props.RampTiers))
	}
}

func TestGetFeaturedInterval_PreMatch(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	// Test far future (>6hr)
	interval := config.GetFeaturedInterval(12.0, false)
	if interval != 60*time.Second {
		t.Errorf("expected 60s for 12hr out, got %v", interval)
	}
}

func TestGetFeaturedInterval_Ramp(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	// Test within ramp window
	interval := config.GetFeaturedInterval(3.0, false) // 3hr until start
	// Should be ramping between 60s and 40s
	if interval < 40*time.Second || interval > 60*time.Second {
		t.Errorf("expected interval between 40s-60s for 3hr out, got %v", interval)
	}

	// Test near tipoff
	interval = config.GetFeaturedInterval(0.5, false) // 30min until start
	// Should be close to 40s target
	if interval < 40*time.Second || interval > 50*time.Second {
		t.Errorf("expected interval close to 40s for 30min out, got %v", interval)
	}
}

func TestGetFeaturedInterval_InPlay(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	// Test live game
	interval := config.GetFeaturedInterval(0, true)
	if interval != 40*time.Second {
		t.Errorf("expected 40s for in-play, got %v", interval)
	}
}

func TestGetPropsInterval(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	tests := []struct {
		name           string
		hoursUntilStart float64
		isLive          bool
		expectedMin     time.Duration
		expectedMax     time.Duration
	}{
		{
			name:           "far future",
			hoursUntilStart: 48,
			isLive:          false,
			expectedMin:     30 * time.Minute,
			expectedMax:     30 * time.Minute,
		},
		{
			name:           "24-6hr range",
			hoursUntilStart: 12,
			isLive:          false,
			expectedMin:     30 * time.Minute,
			expectedMax:     30 * time.Minute,
		},
		{
			name:           "6-1.5hr range",
			hoursUntilStart: 3,
			isLive:          false,
			expectedMin:     10 * time.Minute,
			expectedMax:     10 * time.Minute,
		},
		{
			name:           "1.5hr-20min range",
			hoursUntilStart: 1.0,
			isLive:          false,
			expectedMin:     2 * time.Minute,
			expectedMax:     2 * time.Minute,
		},
		{
			name:           "< 20min range",
			hoursUntilStart: 0.2,
			isLive:          false,
			expectedMin:     1 * time.Minute,
			expectedMax:     1 * time.Minute,
		},
		{
			name:           "in-play",
			hoursUntilStart: 0,
			isLive:          true,
			expectedMin:     60 * time.Second,
			expectedMax:     60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := config.GetPropsInterval(tt.hoursUntilStart, tt.isLive)
			if interval < tt.expectedMin || interval > tt.expectedMax {
				t.Errorf("interval %v not in expected range [%v, %v]",
					interval, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestRampTiersOrdering(t *testing.T) {
	config := basketball_nba.DefaultConfig()

	// Verify tiers are in descending order by FromHours
	for i := 0; i < len(config.Props.RampTiers)-1; i++ {
		curr := config.Props.RampTiers[i]
		next := config.Props.RampTiers[i+1]

		if curr.ToHours >= curr.FromHours {
			t.Errorf("tier %d: ToHours (%f) should be less than FromHours (%f)",
				i, curr.ToHours, curr.FromHours)
		}

		if curr.ToHours != next.FromHours {
			t.Errorf("tier %d and %d don't connect: tier %d ends at %f, tier %d starts at %f",
				i, i+1, i, curr.ToHours, i+1, next.FromHours)
		}
	}
}

func BenchmarkGetFeaturedInterval(b *testing.B) {
	config := basketball_nba.DefaultConfig()

	for i := 0; i < b.N; i++ {
		config.GetFeaturedInterval(3.5, false)
	}
}

func BenchmarkGetPropsInterval(b *testing.B) {
	config := basketball_nba.DefaultConfig()

	for i := 0; i < b.N; i++ {
		config.GetPropsInterval(3.5, false)
	}
}

