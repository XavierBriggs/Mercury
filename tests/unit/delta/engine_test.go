// +build integration

package delta_test

import (
	"context"
	"testing"
	"time"

	"github.com/XavierBriggs/Mercury/internal/delta"
	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/redis/go-redis/v9"
)

func TestDetectChanges_NewOutcome(t *testing.T) {
	// Setup test Redis (requires Redis running)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx := context.Background()
	engine := delta.NewEngine(redisClient, 30*time.Second)

	// Clear test keys
	redisClient.FlushDB(ctx)

	// Create new odds
	now := time.Now()
	odds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "h2h",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers",
			Price:            -110,
			VendorLastUpdate: now,
			ReceivedAt:       now,
		},
	}

	// Detect changes (should be NEW since cache is empty)
	deltas, err := engine.DetectChanges(ctx, odds)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(deltas))
	}

	if deltas[0].ChangeType != delta.ChangeTypeNew {
		t.Errorf("expected ChangeTypeNew, got %s", deltas[0].ChangeType)
	}
}

func TestDetectChanges_PriceChange(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx := context.Background()
	engine := delta.NewEngine(redisClient, 30*time.Second)

	redisClient.FlushDB(ctx)

	now := time.Now()

	// First odds
	initialOdds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "h2h",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers",
			Price:            -110,
			VendorLastUpdate: now,
			ReceivedAt:       now,
		},
	}

	// Update cache with initial odds
	engine.UpdateCache(ctx, initialOdds)

	// Changed odds (price changed)
	changedOdds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "h2h",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers",
			Price:            -115, // Changed from -110
			VendorLastUpdate: now.Add(1 * time.Minute),
			ReceivedAt:       now.Add(1 * time.Minute),
		},
	}

	deltas, err := engine.DetectChanges(ctx, changedOdds)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(deltas))
	}

	if deltas[0].ChangeType != delta.ChangeTypePriceOnly {
		t.Errorf("expected ChangeTypePriceOnly, got %s", deltas[0].ChangeType)
	}

	if deltas[0].OldPrice == nil || *deltas[0].OldPrice != -110 {
		t.Errorf("expected old price -110, got %v", deltas[0].OldPrice)
	}
}

func TestDetectChanges_PointChange(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx := context.Background()
	engine := delta.NewEngine(redisClient, 30*time.Second)

	redisClient.FlushDB(ctx)

	now := time.Now()
	point1 := 3.5
	point2 := 4.5

	// Initial spread
	initialOdds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "spreads",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers -3.5",
			Price:            -110,
			Point:            &point1,
			VendorLastUpdate: now,
			ReceivedAt:       now,
		},
	}

	engine.UpdateCache(ctx, initialOdds)

	// Changed spread line
	changedOdds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "spreads",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers -3.5",
			Price:            -110, // Same price
			Point:            &point2, // Changed from 3.5 to 4.5
			VendorLastUpdate: now.Add(1 * time.Minute),
			ReceivedAt:       now.Add(1 * time.Minute),
		},
	}

	deltas, err := engine.DetectChanges(ctx, changedOdds)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(deltas))
	}

	if deltas[0].ChangeType != delta.ChangeTypePointOnly {
		t.Errorf("expected ChangeTypePointOnly, got %s", deltas[0].ChangeType)
	}

	if deltas[0].OldPoint == nil || *deltas[0].OldPoint != 3.5 {
		t.Errorf("expected old point 3.5, got %v", deltas[0].OldPoint)
	}
}

func TestDetectChanges_NoChange(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx := context.Background()
	engine := delta.NewEngine(redisClient, 30*time.Second)

	redisClient.FlushDB(ctx)

	now := time.Now()

	odds := []models.RawOdds{
		{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "h2h",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers",
			Price:            -110,
			VendorLastUpdate: now,
			ReceivedAt:       now,
		},
	}

	// Update cache
	engine.UpdateCache(ctx, odds)

	// Same odds again
	deltas, err := engine.DetectChanges(ctx, odds)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(deltas) != 0 {
		t.Errorf("expected 0 deltas for unchanged odds, got %d", len(deltas))
	}
}

func BenchmarkDetectChanges(b *testing.B) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx := context.Background()
	engine := delta.NewEngine(redisClient, 30*time.Second)

	redisClient.FlushDB(ctx)

	// Create 100 odds
	now := time.Now()
	odds := make([]models.RawOdds, 100)
	for i := 0; i < 100; i++ {
		odds[i] = models.RawOdds{
			EventID:          "test_event_1",
			SportKey:         "basketball_nba",
			MarketKey:        "h2h",
			BookKey:          "fanduel",
			OutcomeName:      "Lakers",
			Price:            -110 + i,
			VendorLastUpdate: now,
			ReceivedAt:       now,
		}
	}

	engine.UpdateCache(ctx, odds)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.DetectChanges(ctx, odds)
		if err != nil {
			b.Fatalf("DetectChanges failed: %v", err)
		}
	}
}

