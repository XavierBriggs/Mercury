// +build integration

package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/XavierBriggs/Mercury/internal/delta"
	"github.com/XavierBriggs/Mercury/internal/writer"
	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/XavierBriggs/Mercury/pkg/testutil"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// TestEndToEnd_FetchDetectWrite tests the complete Mercury pipeline
func TestEndToEnd_FetchDetectWrite(t *testing.T) {
	ctx := context.Background()

	// Setup test database
	testDSN := getTestDSN()
	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	defer db.Close()

	// Setup test Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_URL", "localhost:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       1, // Use test DB
	})
	defer redisClient.Close()

	// Clear test Redis
	redisClient.FlushDB(ctx)

	// Initialize components
	// Create test event first (required by foreign key)
	_, err = db.ExecContext(ctx, `
		INSERT INTO events (event_id, sport_key, home_team, away_team, commence_time, event_status)
		VALUES ($1, $2, $3, $4, NOW() + INTERVAL '2 hours', $5)
		ON CONFLICT (event_id) DO NOTHING
	`, "integration_test_1", "basketball_nba", "Lakers", "Celtics", "upcoming")
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}

	deltaEngine := delta.NewEngine(redisClient, 30*time.Second)
	w := writer.NewWriter(db, redisClient)
	w.Start(ctx)
	defer w.Stop()

	// Step 1: Create initial odds
	odds1 := []models.RawOdds{
		testutil.NewTestOdd("integration_test_1", "h2h", "fanduel", "Lakers", -110, nil),
		testutil.NewTestOdd("integration_test_1", "h2h", "fanduel", "Celtics", -110, nil),
	}

	// Step 2: Detect changes (should be all new)
	deltas1, err := deltaEngine.DetectChanges(ctx, odds1)
	if err != nil {
		t.Fatalf("first DetectChanges failed: %v", err)
	}

	if len(deltas1) != 2 {
		t.Fatalf("expected 2 new deltas, got %d", len(deltas1))
	}

	// Step 3: Write to Alexandria
	deltaOdds1 := make([]models.RawOdds, len(deltas1))
	for i, d := range deltas1 {
		deltaOdds1[i] = d.Odd
	}

	if err := w.Write(ctx, deltaOdds1); err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	// Flush immediately for test
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Step 4: Update cache
	if err := deltaEngine.UpdateCache(ctx, deltaOdds1); err != nil {
		t.Fatalf("UpdateCache failed: %v", err)
	}

	// Step 5: Verify data was written to Alexandria
	var count int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM odds_raw 
		WHERE event_id = 'integration_test_1' AND is_latest = true
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query Alexandria failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 latest odds in Alexandria, got %d", count)
	}

	// Step 6: Change one price and detect
	odds2 := []models.RawOdds{
		testutil.NewTestOdd("integration_test_1", "h2h", "fanduel", "Lakers", -115, nil), // Changed
		testutil.NewTestOdd("integration_test_1", "h2h", "fanduel", "Celtics", -110, nil), // Unchanged
	}

	deltas2, err := deltaEngine.DetectChanges(ctx, odds2)
	if err != nil {
		t.Fatalf("second DetectChanges failed: %v", err)
	}

	if len(deltas2) != 1 {
		t.Fatalf("expected 1 delta for price change, got %d", len(deltas2))
	}

	if deltas2[0].ChangeType != delta.ChangeTypePriceOnly {
		t.Errorf("expected ChangeTypePriceOnly, got %s", deltas2[0].ChangeType)
	}

	// Step 7: Write second batch
	deltaOdds2 := []models.RawOdds{deltas2[0].Odd}
	if err := w.Write(ctx, deltaOdds2); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	if err := w.Flush(ctx); err != nil {
		t.Fatalf("second Flush failed: %v", err)
	}

	// Step 8: Verify old row was updated to is_latest=false
	var oldCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM odds_raw 
		WHERE event_id = 'integration_test_1' 
		  AND book_key = 'fanduel' 
		  AND outcome_name = 'Lakers'
		  AND is_latest = false
	`).Scan(&oldCount)
	if err != nil {
		t.Fatalf("query old odds failed: %v", err)
	}

	if oldCount != 1 {
		t.Errorf("expected 1 old (is_latest=false) row, got %d", oldCount)
	}

	// Step 9: Verify Redis Stream was published to
	streamKey := "odds.raw.basketball_nba"
	result, err := redisClient.XLen(ctx, streamKey).Result()
	if err != nil {
		t.Fatalf("query stream failed: %v", err)
	}

	if result < 2 { // At least 2 messages (initial + update)
		t.Errorf("expected at least 2 stream messages, got %d", result)
	}

	// Cleanup
	_, err = db.ExecContext(ctx, "DELETE FROM odds_raw WHERE event_id = 'integration_test_1'")
	if err != nil {
		t.Logf("cleanup failed: %v", err)
	}
}

// TestIntegration_LatencySLO tests that Mercury meets <30ms latency SLO
func TestIntegration_LatencySLO(t *testing.T) {
	ctx := context.Background()

	testDSN := getTestDSN()
	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_URL", "localhost:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       1,
	})
	defer redisClient.Close()

	redisClient.FlushDB(ctx)

	// Create test event first (required by foreign key)
	_, err = db.ExecContext(ctx, `
		INSERT INTO events (event_id, sport_key, home_team, away_team, commence_time, event_status)
		VALUES ($1, $2, $3, $4, NOW() + INTERVAL '2 hours', $5)
		ON CONFLICT (event_id) DO NOTHING
	`, "slo_test_event", "basketball_nba", "Lakers", "Celtics", "upcoming")
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}

	deltaEngine := delta.NewEngine(redisClient, 30*time.Second)
	w := writer.NewWriter(db, redisClient)
	w.Start(ctx)
	defer w.Stop()

	// Create 100 odds changes (using real book keys)
	realBooks := []string{"fanduel", "draftkings", "betmgm", "caesars", "pinnacle", "circa", "bookmaker", "pointsbet", "betrivers", "wynnbet"}
	odds := make([]models.RawOdds, 100)
	for i := 0; i < 100; i++ {
		bookKey := realBooks[i%len(realBooks)] // Cycle through real books
		odds[i] = testutil.NewTestOdd(
			"slo_test_event",
			"h2h",
			bookKey,
			fmt.Sprintf("Outcome_%d", i), // Different outcomes for each
			-110,
			nil,
		)
	}

	// Measure delta detection latency
	start := time.Now()
	deltas, err := deltaEngine.DetectChanges(ctx, odds)
	deltaDuration := time.Since(start)

	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	// Delta detection should be <1ms
	if deltaDuration > 1*time.Millisecond {
		t.Errorf("delta detection exceeded 1ms: %v", deltaDuration)
	}

	t.Logf("delta detection for 100 odds: %v", deltaDuration)

	// Measure write + cache update latency
	deltaOdds := make([]models.RawOdds, len(deltas))
	for i, d := range deltas {
		deltaOdds[i] = d.Odd
	}

	start = time.Now()
	if err := w.Write(ctx, deltaOdds); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if err := deltaEngine.UpdateCache(ctx, deltaOdds); err != nil {
		t.Fatalf("UpdateCache failed: %v", err)
	}
	writeDuration := time.Since(start)

	t.Logf("write + cache update for 100 odds: %v", writeDuration)

	// Total Mercury component latency should be <30ms (excluding vendor API)
	totalDuration := deltaDuration + writeDuration
	if totalDuration > 30*time.Millisecond {
		t.Errorf("total Mercury latency exceeded 30ms SLO: %v", totalDuration)
	}

	t.Logf("✓ Total Mercury latency: %v (SLO <30ms)", totalDuration)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM odds_raw WHERE event_id = 'slo_test_event'")
}

func getTestDSN() string {
	// Use environment variable or default test database
	if dsn := os.Getenv("ALEXANDRIA_TEST_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://fortuna:fortuna_dev_password@localhost:5432/alexandria_test?sslmode=disable"
}

