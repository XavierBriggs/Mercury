package closer

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatusUpdater updates event status based on commence_time
type StatusUpdater struct {
	db           *sql.DB
	pollInterval time.Duration
	stopChan     chan struct{}
}

// NewStatusUpdater creates a new event status updater
func NewStatusUpdater(db *sql.DB, pollInterval time.Duration) *StatusUpdater {
	return &StatusUpdater{
		db:           db,
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
	}
}

// Start begins monitoring and updating event statuses
func (s *StatusUpdater) Start(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	fmt.Println("✓ Event status updater started")

	// Initial update immediately
	if err := s.updateStatuses(ctx); err != nil {
		fmt.Printf("[StatusUpdater] initial update error: %v\n", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := s.updateStatuses(ctx); err != nil {
				fmt.Printf("[StatusUpdater] update error: %v\n", err)
			}
		case <-s.stopChan:
			fmt.Println("✓ Event status updater stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop gracefully stops the updater
func (s *StatusUpdater) Stop() {
	close(s.stopChan)
}

// updateStatuses updates event statuses based on current time
func (s *StatusUpdater) updateStatuses(ctx context.Context) error {
	// Update upcoming -> live (games that started in last 5 minutes)
	liveQuery := `
		UPDATE events
		SET event_status = 'live'
		WHERE event_status = 'upcoming'
		  AND commence_time <= NOW()
		  AND commence_time > NOW() - INTERVAL '5 minutes'
	`

	liveResult, err := s.db.ExecContext(ctx, liveQuery)
	if err != nil {
		return fmt.Errorf("update to live: %w", err)
	}

	liveCount, _ := liveResult.RowsAffected()
	if liveCount > 0 {
		fmt.Printf("[StatusUpdater] marked %d event(s) as LIVE\n", liveCount)
	}

	// Update live -> completed (games that started >3 hours ago)
	// NBA games typically last 2-2.5 hours, so 3 hours is a safe buffer
	completedQuery := `
		UPDATE events
		SET event_status = 'completed'
		WHERE event_status = 'live'
		  AND commence_time < NOW() - INTERVAL '3 hours'
	`

	completedResult, err := s.db.ExecContext(ctx, completedQuery)
	if err != nil {
		return fmt.Errorf("update to completed: %w", err)
	}

	completedCount, _ := completedResult.RowsAffected()
	if completedCount > 0 {
		fmt.Printf("[StatusUpdater] marked %d event(s) as COMPLETED\n", completedCount)
	}

	// Cleanup old completed events (optional - remove events >7 days old)
	// cleanupQuery := `
	// 	DELETE FROM events
	// 	WHERE event_status = 'completed'
	// 	  AND commence_time < NOW() - INTERVAL '7 days'
	// `
	//
	// cleanupResult, err := s.db.ExecContext(ctx, cleanupQuery)
	// if err != nil {
	// 	fmt.Printf("[StatusUpdater] cleanup warning: %v\n", err)
	// }
	//
	// cleanupCount, _ := cleanupResult.RowsAffected()
	// if cleanupCount > 0 {
	// 	fmt.Printf("[StatusUpdater] cleaned up %d old event(s)\n", cleanupCount)
	// }

	return nil
}
