package basketball_nba

import (
	"fmt"
	"strings"
	"time"

	"github.com/XavierBriggs/Mercury/pkg/models"
)

// ValidateEvent checks if an NBA event is valid
func ValidateEvent(event *models.Event) error {
	if event.SportKey != "basketball_nba" {
		return fmt.Errorf("invalid sport key: expected basketball_nba, got %s", event.SportKey)
	}

	if event.HomeTeam == "" {
		return fmt.Errorf("home team cannot be empty")
	}

	if event.AwayTeam == "" {
		return fmt.Errorf("away team cannot be empty")
	}

	if event.HomeTeam == event.AwayTeam {
		return fmt.Errorf("home and away teams cannot be the same")
	}

	if event.CommenceTime.Before(time.Now().Add(-24 * time.Hour)) {
		return fmt.Errorf("event commence time is too far in the past")
	}

	return nil
}

// NormalizeTeamName standardizes team names from vendor
// Handles variations like "LA Lakers" vs "Los Angeles Lakers"
func NormalizeTeamName(name string) string {
	name = strings.TrimSpace(name)

	// Common normalizations
	replacements := map[string]string{
		"LA Lakers":       "Los Angeles Lakers",
		"LA Clippers":     "Los Angeles Clippers",
		"NY Knicks":       "New York Knicks",
		"GS Warriors":     "Golden State Warriors",
		"SA Spurs":        "San Antonio Spurs",
		"OKC Thunder":     "Oklahoma City Thunder",
		"NO Pelicans":     "New Orleans Pelicans",
		"Washington Wizards": "Washington Wizards",
	}

	if normalized, ok := replacements[name]; ok {
		return normalized
	}

	return name
}

// IsRegularSeason determines if a date falls within NBA regular season
// This is a simplified version - real impl would query a calendar
func IsRegularSeason(t time.Time) bool {
	month := t.Month()
	// NBA regular season roughly Oct-Apr
	return month >= time.October || month <= time.April
}

