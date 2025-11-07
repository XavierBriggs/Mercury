package adapters_test

import (
	"testing"

	"github.com/XavierBriggs/Mercury/adapters/theoddsapi"
)

func TestSupportsMarket(t *testing.T) {
	client := theoddsapi.NewClient("test_key")

	tests := []struct {
		market   string
		expected bool
	}{
		{"h2h", true},
		{"spreads", true},
		{"totals", true},
		{"player_points", true},
		{"player_rebounds", true},
		{"invalid_market", false},
		{"futures", false},
	}

	for _, tt := range tests {
		t.Run(tt.market, func(t *testing.T) {
			result := client.SupportsMarket(tt.market)
			if result != tt.expected {
				t.Errorf("SupportsMarket(%s) = %v, want %v", tt.market, result, tt.expected)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	client := theoddsapi.NewClient("test_api_key")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestGetRateLimits(t *testing.T) {
	client := theoddsapi.NewClient("test_key")
	limits := client.GetRateLimits()

	if limits == nil {
		t.Fatal("GetRateLimits returned nil")
	}

	// Initial state should have 500 requests remaining
	if limits.RequestsRemaining != 500 {
		t.Errorf("expected 500 initial requests, got %d", limits.RequestsRemaining)
	}
}

// TODO: Add HTTP mocking tests for FetchOdds and FetchEvents
// These require either:
// 1. Exposing httpClient or baseURL in Client for testing
// 2. Using dependency injection for HTTP client
// 3. Creating a testable constructor that accepts custom base URL
//
// For now, these methods are tested via integration tests
