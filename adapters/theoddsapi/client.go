package theoddsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/XavierBriggs/Mercury/pkg/contracts"
	"github.com/XavierBriggs/Mercury/pkg/models"
)

const (
	baseURL     = "https://api.the-odds-api.com"
	apiVersion  = "v4"
	userAgent   = "Mercury/1.0 (Fortuna Odds Aggregator)"
	timeout     = 10 * time.Second
	maxRetries  = 3
	retryDelay  = 2 * time.Second
)

// Client implements the VendorAdapter interface for The Odds API
type Client struct {
	apiKey     string
	httpClient *http.Client
	rateLimits *models.RateLimits
	mu         sync.RWMutex
}

// Ensure Client implements VendorAdapter
var _ contracts.VendorAdapter = (*Client)(nil)

// NewClient creates a new The Odds API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		rateLimits: &models.RateLimits{
			RequestsRemaining: 500, // Default quota
			RequestsUsed:      0,
		},
	}
}

// FetchOdds retrieves featured market odds (h2h, spreads, totals)
func (c *Client) FetchOdds(ctx context.Context, opts *models.FetchOddsOptions) (*models.FetchResult, error) {
	endpoint := fmt.Sprintf("%s/%s/sports/%s/odds", baseURL, apiVersion, opts.Sport)

	params := url.Values{}
	params.Set("apiKey", c.apiKey)
	params.Set("regions", strings.Join(opts.Regions, ","))
	params.Set("markets", strings.Join(opts.Markets, ","))
	params.Set("oddsFormat", "american")
	params.Set("dateFormat", "iso")

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	body, err := c.doRequestWithRetry(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("fetch odds failed: %w", err)
	}

	var apiResp []oddsResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse odds response: %w", err)
	}

	return c.parseOddsResponse(apiResp, time.Now()), nil
}

// FetchEventOdds retrieves event-specific odds (for props markets)
func (c *Client) FetchEventOdds(ctx context.Context, opts *models.FetchEventOddsOptions) (*models.FetchResult, error) {
	endpoint := fmt.Sprintf("%s/%s/sports/%s/events/%s/odds", baseURL, apiVersion, opts.Sport, opts.EventID)

	params := url.Values{}
	params.Set("apiKey", c.apiKey)
	params.Set("regions", strings.Join(opts.Regions, ","))
	params.Set("markets", strings.Join(opts.Markets, ","))
	params.Set("oddsFormat", "american")
	params.Set("dateFormat", "iso")

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	body, err := c.doRequestWithRetry(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("fetch event odds failed: %w", err)
	}

	// Single event response
	var apiResp oddsResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse event odds response: %w", err)
	}

	return c.parseOddsResponse([]oddsResponse{apiResp}, time.Now()), nil
}

// FetchEvents retrieves upcoming events without odds (for discovery)
func (c *Client) FetchEvents(ctx context.Context, sport string) ([]models.Event, error) {
	endpoint := fmt.Sprintf("%s/%s/sports/%s/events", baseURL, apiVersion, sport)

	params := url.Values{}
	params.Set("apiKey", c.apiKey)
	params.Set("dateFormat", "iso")

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	body, err := c.doRequestWithRetry(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("fetch events failed: %w", err)
	}

	var apiResp []eventResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse events response: %w", err)
	}

	return c.parseEventsResponse(apiResp), nil
}

// SupportsMarket checks if this adapter supports a given market
func (c *Client) SupportsMarket(market string) bool {
	supportedMarkets := map[string]bool{
		// Featured markets
		"h2h":     true,
		"spreads": true,
		"totals":  true,
		// Player props
		"player_points":                  true,
		"player_rebounds":                true,
		"player_assists":                 true,
		"player_threes":                  true,
		"player_points_rebounds_assists": true,
		"player_points_rebounds":         true,
		"player_points_assists":          true,
		"player_rebounds_assists":        true,
		"player_steals":                  true,
		"player_blocks":                  true,
		"player_turnovers":               true,
		"player_double_double":           true,
		"player_triple_double":           true,
	}
	return supportedMarkets[market]
}

// GetRateLimits returns current rate limit information
func (c *Client) GetRateLimits() *models.RateLimits {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimits
}

// doRequestWithRetry performs HTTP request with retry logic
func (c *Client) doRequestWithRetry(ctx context.Context, fullURL string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := retryDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		body, err := c.doRequest(ctx, fullURL)
		if err == nil {
			return body, nil
		}

		lastErr = err

		// Don't retry on client errors (4xx except 429)
		if httpErr, ok := err.(*httpError); ok {
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != 429 {
				return nil, err
			}
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doRequest performs a single HTTP request
func (c *Client) doRequest(ctx context.Context, fullURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Update rate limits from headers
	c.updateRateLimits(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	return body, nil
}

// updateRateLimits extracts rate limit info from response headers
func (c *Client) updateRateLimits(headers http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if remaining := headers.Get("x-requests-remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			c.rateLimits.RequestsRemaining = val
		}
	}

	if used := headers.Get("x-requests-used"); used != "" {
		if val, err := strconv.Atoi(used); err == nil {
			c.rateLimits.RequestsUsed = val
		}
	}
}

// parseOddsResponse converts API response to internal FetchResult with events and odds
func (c *Client) parseOddsResponse(apiResp []oddsResponse, receivedAt time.Time) *models.FetchResult {
	var allOdds []models.RawOdds
	var allEvents []models.Event
	seenEvents := make(map[string]bool)

	for _, event := range apiResp {
		// Parse event commence time once per event
		commenceTime, err := time.Parse(time.RFC3339, event.CommenceTime)
		if err != nil {
			commenceTime = receivedAt // Fallback
		}

		// Extract event (deduplicate by ID)
		if !seenEvents[event.ID] {
			// Determine if game is live based on commence_time
			eventStatus := "upcoming"
			if time.Now().After(commenceTime) {
				eventStatus = "live"
			}
			
			allEvents = append(allEvents, models.Event{
				EventID:      event.ID,
				SportKey:     event.SportKey,
				HomeTeam:     event.HomeTeam,
				AwayTeam:     event.AwayTeam,
				CommenceTime: commenceTime,
				EventStatus:  eventStatus,
			})
			seenEvents[event.ID] = true
		}

		// Extract odds
		for _, bookmaker := range event.Bookmakers {
			vendorUpdate, err := time.Parse(time.RFC3339, bookmaker.LastUpdate)
			if err != nil {
				vendorUpdate = receivedAt
			}

			for _, market := range bookmaker.Markets {
				for _, outcome := range market.Outcomes {
					odd := models.RawOdds{
						EventID:          event.ID,
						SportKey:         event.SportKey,
						MarketKey:        market.Key,
						BookKey:          bookmaker.Key,
						OutcomeName:      outcome.Name,
						Price:            outcome.Price,
						VendorLastUpdate: vendorUpdate,
						ReceivedAt:       receivedAt,
					}

					// Add point for spreads/totals
					if outcome.Point != nil {
						point := *outcome.Point
						odd.Point = &point
					}

					allOdds = append(allOdds, odd)
				}
			}
		}
	}

	return &models.FetchResult{
		Events: allEvents,
		Odds:   allOdds,
	}
}

// parseEventsResponse converts API response to internal Event format
func (c *Client) parseEventsResponse(apiResp []eventResponse) []models.Event {
	events := make([]models.Event, 0, len(apiResp))

	for _, evt := range apiResp {
		commenceTime, err := time.Parse(time.RFC3339, evt.CommenceTime)
		if err != nil {
			continue // Skip invalid events
		}

		// Determine if game is live based on commence_time
		eventStatus := "upcoming"
		if time.Now().After(commenceTime) {
			eventStatus = "live"
		}

		events = append(events, models.Event{
			EventID:      evt.ID,
			SportKey:     evt.SportKey,
			HomeTeam:     evt.HomeTeam,
			AwayTeam:     evt.AwayTeam,
			CommenceTime: commenceTime,
			EventStatus:  eventStatus,
		})
	}

	return events
}

// httpError represents an HTTP error with status code
type httpError struct {
	StatusCode int
	Message    string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// API response structures matching The Odds API JSON format

type oddsResponse struct {
	ID           string       `json:"id"`
	SportKey     string       `json:"sport_key"`
	SportTitle   string       `json:"sport_title"`
	CommenceTime string       `json:"commence_time"`
	HomeTeam     string       `json:"home_team"`
	AwayTeam     string       `json:"away_team"`
	Bookmakers   []bookmaker  `json:"bookmakers"`
}

type bookmaker struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	LastUpdate string   `json:"last_update"`
	Markets    []market `json:"markets"`
}

type market struct {
	Key        string    `json:"key"`
	LastUpdate string    `json:"last_update"`
	Outcomes   []outcome `json:"outcomes"`
}

type outcome struct {
	Name  string   `json:"name"`
	Price int      `json:"price"`
	Point *float64 `json:"point,omitempty"`
}

type eventResponse struct {
	ID           string `json:"id"`
	SportKey     string `json:"sport_key"`
	SportTitle   string `json:"sport_title"`
	CommenceTime string `json:"commence_time"`
	HomeTeam     string `json:"home_team"`
	AwayTeam     string `json:"away_team"`
}

