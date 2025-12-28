// Package talos provides an HTTP client for communicating with Talos Bot Manager
// to warm and close game pages based on event lifecycle.
package talos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Client handles HTTP communication with Talos Bot Manager for page warming
type Client struct {
	baseURL    string
	httpClient *http.Client
	enabled    bool
	books      []string // List of book keys to warm pages for
}

// Config holds configuration for the Talos client
type Config struct {
	BaseURL string   // e.g., "http://localhost:5008"
	Enabled bool     // Whether page warming is enabled
	Books   []string // List of books to warm, e.g., ["betmgm", "fanduel", "bovada"]
	Timeout time.Duration
}

// OpenGamePageRequest is the request format for warming a game page
type OpenGamePageRequest struct {
	Team1       string   `json:"team1"`
	Team2       string   `json:"team2"`
	Sport       string   `json:"sport"`
	League      string   `json:"league,omitempty"`
	BetPeriod   string   `json:"bet_period"`
	EventDate   string   `json:"event_date"` // YYYY-MM-DD format - always required
	TargetBooks []string `json:"target_books,omitempty"`
}

// CloseGamePageRequest is the request format for closing a game page
type CloseGamePageRequest struct {
	Book        string   `json:"book"`
	GameKey     string   `json:"game_key"`
	TargetBooks []string `json:"target_books,omitempty"`
}

// PageActionResponse is the response from open/close game page endpoints
type PageActionResponse struct {
	AllOK   bool                   `json:"all_ok"`
	AnyOK   bool                   `json:"any_ok"`
	Results map[string]interface{} `json:"results"`
}

// NewClient creates a new Talos client
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		enabled: cfg.Enabled,
		books:   cfg.Books,
	}
}

// IsEnabled returns whether page warming is enabled
func (c *Client) IsEnabled() bool {
	return c.enabled && c.baseURL != ""
}

// OpenGamePage warms a game page across all configured books
// Called when a new event is discovered with odds
func (c *Client) OpenGamePage(ctx context.Context, homeTeam, awayTeam, sport string, commenceTime time.Time) error {
	if !c.IsEnabled() {
		return nil
	}

	req := OpenGamePageRequest{
		Team1:       awayTeam, // Away team first (convention)
		Team2:       homeTeam, // Home team second
		Sport:       mapSportKey(sport),
		BetPeriod:   "game",
		EventDate:   commenceTime.Format("2006-01-02"),
		TargetBooks: c.books,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[Talos] Opening game page: %s @ %s (date: %s)", awayTeam, homeTeam, req.EventDate)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/open-game-page", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var pageResp PageActionResponse
	if err := json.Unmarshal(body, &pageResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !pageResp.AnyOK {
		log.Printf("[Talos] Warning: No bots warmed page for %s @ %s", awayTeam, homeTeam)
	} else {
		log.Printf("[Talos] Game page warmed: %s @ %s (all_ok=%v)", awayTeam, homeTeam, pageResp.AllOK)
	}

	return nil
}

// CloseGamePage closes a game page across all configured books
// Called when an event is marked as completed
func (c *Client) CloseGamePage(ctx context.Context, gameKey string) error {
	if !c.IsEnabled() {
		return nil
	}

	log.Printf("[Talos] Closing game pages for: %s", gameKey)

	// Close for each configured book
	for _, book := range c.books {
		req := CloseGamePageRequest{
			Book:    book,
			GameKey: gameKey,
		}

		jsonData, err := json.Marshal(req)
		if err != nil {
			log.Printf("[Talos] Warning: Failed to marshal close request for %s: %v", book, err)
			continue
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/close-game-page", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("[Talos] Warning: Failed to create close request for %s: %v", book, err)
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			log.Printf("[Talos] Warning: Failed to close page for %s: %v", book, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("[Talos] Warning: Close page failed for %s (status %d)", book, resp.StatusCode)
		}
	}

	return nil
}

// CloseGamePageForEvent closes game pages using event details
// Builds game key from event fields
func (c *Client) CloseGamePageForEvent(ctx context.Context, homeTeam, awayTeam, sport string, commenceTime time.Time) error {
	if !c.IsEnabled() {
		return nil
	}

	// Build game key for each book and close
	dateStr := commenceTime.Format("20060102")
	sportKey := mapSportKey(sport)

	// Normalize team names for key
	team1 := normalizeTeamName(awayTeam)
	team2 := normalizeTeamName(homeTeam)

	// Ensure consistent ordering (alphabetical)
	if team1 > team2 {
		team1, team2 = team2, team1
	}

	for _, book := range c.books {
		// Format: book:sport:league:date:team1:team2:period
		gameKey := fmt.Sprintf("%s:%s::%s:%s:%s:game", book, sportKey, dateStr, team1, team2)

		if err := c.CloseGamePage(ctx, gameKey); err != nil {
			log.Printf("[Talos] Warning: Failed to close page %s: %v", gameKey, err)
		}
	}

	return nil
}

// mapSportKey converts API sport keys to normalized format
func mapSportKey(sport string) string {
	switch sport {
	case "basketball_nba", "basketball/nba":
		return "nba"
	case "football_nfl", "football/nfl":
		return "nfl"
	case "baseball_mlb", "baseball/mlb":
		return "mlb"
	case "hockey_nhl", "hockey/nhl":
		return "nhl"
	default:
		return sport
	}
}

// normalizeTeamName converts team name to slug format for game key
func normalizeTeamName(name string) string {
	// Simple normalization - lowercase and replace spaces with underscores
	result := ""
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			result += string(c + 32) // lowercase
		} else if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			result += string(c)
		} else if c == ' ' {
			result += "_"
		}
	}
	return result
}
