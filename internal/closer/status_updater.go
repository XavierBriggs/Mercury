package closer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/XavierBriggs/Mercury/internal/talos"
)

// completedEvent holds the details needed to close game pages
type completedEvent struct {
	EventID      string
	SportKey     string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
}

// StatusUpdater updates event status based on commence_time
type StatusUpdater struct {
	db           *sql.DB
	talos        *talos.Client // Optional Talos client for page closing
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

// SetTalosClient sets the Talos client for page closing
func (s *StatusUpdater) SetTalosClient(client *talos.Client) {
	s.talos = client
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

	// First, fetch events that are about to be marked as completed
	// We need their details for closing game pages
	eventsToComplete, err := s.fetchEventsToComplete(ctx)
	if err != nil {
		fmt.Printf("[StatusUpdater] fetch events to complete warning: %v\n", err)
		// Continue with update even if fetch fails
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

		// Close game pages for completed events
		s.closeGamePages(ctx, eventsToComplete)
	}

	return nil
}

// fetchEventsToComplete fetches event details for events about to be marked completed
func (s *StatusUpdater) fetchEventsToComplete(ctx context.Context) ([]completedEvent, error) {
	query := `
		SELECT event_id, sport_key, home_team, away_team, commence_time
		FROM events
		WHERE event_status = 'live'
		  AND commence_time < NOW() - INTERVAL '3 hours'
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query events to complete: %w", err)
	}
	defer rows.Close()

	var events []completedEvent
	for rows.Next() {
		var evt completedEvent
		if err := rows.Scan(&evt.EventID, &evt.SportKey, &evt.HomeTeam, &evt.AwayTeam, &evt.CommenceTime); err != nil {
			fmt.Printf("[StatusUpdater] scan warning: %v\n", err)
			continue
		}
		events = append(events, evt)
	}

	return events, nil
}

// closeGamePages sends CloseGamePage requests to Talos for completed events
func (s *StatusUpdater) closeGamePages(ctx context.Context, events []completedEvent) {
	if s.talos == nil || !s.talos.IsEnabled() {
		return
	}

	for _, evt := range events {
		// Send close request (async - don't block)
		go func(e completedEvent) {
			closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := s.talos.CloseGamePageForEvent(closeCtx, e.HomeTeam, e.AwayTeam, e.SportKey, e.CommenceTime); err != nil {
				fmt.Printf("[StatusUpdater] Page close failed for %s @ %s: %v\n", e.AwayTeam, e.HomeTeam, err)
			} else {
				fmt.Printf("[StatusUpdater] Closed pages for %s @ %s\n", e.AwayTeam, e.HomeTeam)
			}
		}(evt)
	}
}
