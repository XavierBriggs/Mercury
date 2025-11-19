package closer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Capturer monitors events and captures closing lines when they go live
type Capturer struct {
	db          *sql.DB
	redisClient *redis.Client
	pollInterval time.Duration
	stopChan    chan struct{}
}

// NewCapturer creates a new closing line capturer
func NewCapturer(db *sql.DB, redisClient *redis.Client, pollInterval time.Duration) *Capturer {
	return &Capturer{
		db:           db,
		redisClient:  redisClient,
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
	}
}

// Start begins monitoring for events going live
func (c *Capturer) Start(ctx context.Context) {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	fmt.Println("✓ Closing line capturer started")

	// Initial check immediately
	if err := c.captureClosingLines(ctx); err != nil {
		fmt.Printf("[Closer] initial capture error: %v\n", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := c.captureClosingLines(ctx); err != nil {
				fmt.Printf("[Closer] capture error: %v\n", err)
			}
		case <-c.stopChan:
			fmt.Println("✓ Closing line capturer stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop gracefully stops the capturer
func (c *Capturer) Stop() {
	close(c.stopChan)
}

// captureClosingLines finds events that just went live and captures their closing lines
func (c *Capturer) captureClosingLines(ctx context.Context) error {
	// Find events that are now live but don't have closing lines yet
	query := `
		SELECT DISTINCT e.event_id 
		FROM events e
		WHERE e.event_status = 'live'
		  AND e.event_id NOT IN (SELECT DISTINCT event_id FROM closing_lines)
		  AND e.commence_time BETWEEN NOW() - INTERVAL '5 minutes' AND NOW() + INTERVAL '5 minutes'
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query live events: %w", err)
	}
	defer rows.Close()

	var liveEvents []string
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return fmt.Errorf("scan event: %w", err)
		}
		liveEvents = append(liveEvents, eventID)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	// Capture closing lines for each event
	for _, eventID := range liveEvents {
		if err := c.captureEventClosingLines(ctx, eventID); err != nil {
			fmt.Printf("[Closer] error capturing lines for event %s: %v\n", eventID, err)
			continue
		}
		fmt.Printf("[Closer] captured closing lines for event: %s\n", eventID)
	}

	return nil
}

// captureEventClosingLines captures all current odds for an event as closing lines
func (c *Capturer) captureEventClosingLines(ctx context.Context, eventID string) error {
	// Begin transaction
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert closing lines from current odds
	// Convert NULL points to 0 for h2h markets (primary key compatibility)
	insertQuery := `
		INSERT INTO closing_lines (event_id, sport_key, market_key, book_key, outcome_name, closing_price, point, closed_at)
		SELECT event_id, sport_key, market_key, book_key, outcome_name, price, COALESCE(point, 0), NOW()
		FROM odds_raw
		WHERE event_id = $1 AND is_latest = true
		ON CONFLICT (event_id, market_key, book_key, outcome_name, point) DO NOTHING
	`

	result, err := tx.ExecContext(ctx, insertQuery, eventID)
	if err != nil {
		return fmt.Errorf("insert closing lines: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Publish to Redis stream
	if err := c.publishClosingLineEvent(ctx, eventID); err != nil {
		// Log but don't fail - closing lines are captured
		fmt.Printf("[Closer] warning: failed to publish stream event: %v\n", err)
	}

	fmt.Printf("[Closer] captured %d closing lines for event %s\n", rowsAffected, eventID)

	return nil
}

// publishClosingLineEvent publishes a message to Redis stream
func (c *Capturer) publishClosingLineEvent(ctx context.Context, eventID string) error {
	streamName := "closing_lines.captured"
	
	values := map[string]interface{}{
		"event_id":    eventID,
		"captured_at": time.Now().UTC().Format(time.RFC3339),
	}

	_, err := c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: values,
	}).Result()

	if err != nil {
		return fmt.Errorf("xadd to stream: %w", err)
	}

	return nil
}

