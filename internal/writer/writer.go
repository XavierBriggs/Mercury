package writer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/XavierBriggs/Mercury/internal/talos"
	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	streamKeyFormat      = "odds.raw.%s" // odds.raw.basketball_nba
)

// Writer batches Alexandria DB writes and publishes to Redis Streams
// Implements the write-through cache pattern
type Writer struct {
	db    *sql.DB
	redis *redis.Client
	talos *talos.Client // Optional Talos client for page warming

	batchSize     int
	flushInterval time.Duration

	buffer []models.RawOdds
	mu     sync.Mutex

	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Track seen events to only warm new ones
	seenEvents   map[string]bool
	seenEventsMu sync.RWMutex
}

// StreamMessage represents a message published to Redis Stream
type StreamMessage struct {
	EventID          string    `json:"event_id"`
	SportKey         string    `json:"sport_key"`
	MarketKey        string    `json:"market_key"`
	BookKey          string    `json:"book_key"`
	OutcomeName      string    `json:"outcome_name"`
	Price            int       `json:"price"`
	Point            *float64  `json:"point,omitempty"`
	VendorLastUpdate time.Time `json:"vendor_last_update"`
	ReceivedAt       time.Time `json:"received_at"`
	EventStatus      string    `json:"event_status"` // "upcoming" or "live"
	ChangeType       string    `json:"change_type,omitempty"`
}

// NewWriter creates a new batching writer
func NewWriter(db *sql.DB, redisClient *redis.Client) *Writer {
	return &Writer{
		db:            db,
		redis:         redisClient,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		buffer:        make([]models.RawOdds, 0, defaultBatchSize),
		stopChan:      make(chan struct{}),
		seenEvents:    make(map[string]bool),
	}
}

// SetTalosClient sets the Talos client for page warming
func (w *Writer) SetTalosClient(client *talos.Client) {
	w.talos = client
}

// Start begins the background flush ticker
func (w *Writer) Start(ctx context.Context) {
	w.flushTicker = time.NewTicker(w.flushInterval)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.flushTicker.C:
				if err := w.Flush(ctx); err != nil {
					// Log error but continue (would use proper logging in production)
					fmt.Printf("flush error: %v\n", err)
				}
			case <-w.stopChan:
				w.flushTicker.Stop()
				// Final flush on shutdown
				_ = w.Flush(ctx)
				return
			case <-ctx.Done():
				w.flushTicker.Stop()
				return
			}
		}
	}()
}

// Stop gracefully shuts down the writer
func (w *Writer) Stop() {
	close(w.stopChan)
	w.wg.Wait()
}

// Write adds odds to the buffer and flushes if batch size is reached
func (w *Writer) Write(ctx context.Context, odds []models.RawOdds) error {
	w.mu.Lock()
	w.buffer = append(w.buffer, odds...)
	shouldFlush := len(w.buffer) >= w.batchSize
	w.mu.Unlock()

	if shouldFlush {
		return w.Flush(ctx)
	}

	return nil
}

// WriteWithEvents writes events and odds together (for immediate upsert)
func (w *Writer) WriteWithEvents(ctx context.Context, events []models.Event, odds []models.RawOdds) error {
	if len(events) == 0 && len(odds) == 0 {
		return nil
	}

	// Filter odds: Only accept Pinnacle from EU region books
	// All US/US2 books are accepted automatically
	odds = filterEUBooks(odds)

	// Identify new events (not seen before) for page warming
	newEvents := w.identifyNewEvents(events)

	// Execute write in transaction immediately (bypass buffer)
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 0: Upsert events
	if len(events) > 0 {
		if err := w.upsertEventsFromList(ctx, tx, events); err != nil {
			return fmt.Errorf("upsert events: %w", err)
		}
	}

	// Step 0.5: Upsert books (extract from odds)
	if len(odds) > 0 {
		if err := w.upsertBooksFromOdds(ctx, tx, odds); err != nil {
			return fmt.Errorf("upsert books: %w", err)
		}
	}

	// Step 1: Update previous rows (set is_latest = false)
	if len(odds) > 0 {
		if err := w.updatePreviousOdds(ctx, tx, odds); err != nil {
			return fmt.Errorf("update previous odds: %w", err)
		}

		// Step 2: Insert new rows (with is_latest = true)
		if err := w.insertNewOdds(ctx, tx, odds); err != nil {
			return fmt.Errorf("insert new odds: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Step 3: Publish to Redis Streams (after successful DB write)
	if len(odds) > 0 {
		if err := w.publishToStream(ctx, odds, events); err != nil {
			// Log but don't fail - DB is source of truth
			fmt.Printf("publish to stream error: %v\n", err)
		}
	}

	// Step 4: Warm game pages for new events (after successful DB write)
	if len(newEvents) > 0 {
		w.warmGamePages(ctx, newEvents)
	}

	return nil
}

// Flush writes buffered odds to Alexandria and publishes to Redis Stream
func (w *Writer) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}

	// Swap buffer
	odds := w.buffer
	w.buffer = make([]models.RawOdds, 0, w.batchSize)
	w.mu.Unlock()

	// Execute write in transaction
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Update previous rows (set is_latest = false)
	if err := w.updatePreviousOdds(ctx, tx, odds); err != nil {
		return fmt.Errorf("update previous odds: %w", err)
	}

	// Step 2: Insert new rows (with is_latest = true)
	if err := w.insertNewOdds(ctx, tx, odds); err != nil {
		return fmt.Errorf("insert new odds: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Step 3: Publish to Redis Streams (after successful DB write)
	// Note: events are not available in Flush context, pass nil
	if err := w.publishToStream(ctx, odds, nil); err != nil {
		// Log but don't fail - DB is source of truth
		fmt.Printf("publish to stream error: %v\n", err)
	}

	return nil
}

// updatePreviousOdds sets is_latest = false for existing odds
func (w *Writer) updatePreviousOdds(ctx context.Context, tx *sql.Tx, odds []models.RawOdds) error {
	if len(odds) == 0 {
		return nil
	}

	// Build UPDATE statement for batch
	// UPDATE odds_raw SET is_latest = false
	// WHERE is_latest = true AND (event_id, market_key, book_key, outcome_name) IN (...)

	query := `
		UPDATE odds_raw 
		SET is_latest = false 
		WHERE is_latest = true 
		  AND (event_id, market_key, book_key, outcome_name) IN (
			SELECT UNNEST($1::text[]), UNNEST($2::text[]), UNNEST($3::text[]), UNNEST($4::text[])
		  )
	`

	eventIDs := make([]string, len(odds))
	marketKeys := make([]string, len(odds))
	bookKeys := make([]string, len(odds))
	outcomeNames := make([]string, len(odds))

	for i, odd := range odds {
		eventIDs[i] = odd.EventID
		marketKeys[i] = odd.MarketKey
		bookKeys[i] = odd.BookKey
		outcomeNames[i] = odd.OutcomeName
	}

	_, err := tx.ExecContext(ctx, query, pq.Array(eventIDs), pq.Array(marketKeys), pq.Array(bookKeys), pq.Array(outcomeNames))
	return err
}

// insertNewOdds inserts new odds rows with is_latest = true
func (w *Writer) insertNewOdds(ctx context.Context, tx *sql.Tx, odds []models.RawOdds) error {
	if len(odds) == 0 {
		return nil
	}

	// Build INSERT statement with UNNEST for batch insert
	query := `
		INSERT INTO odds_raw (
			event_id, sport_key, market_key, book_key, outcome_name,
			price, point, vendor_last_update, received_at, is_latest
		)
		SELECT * FROM UNNEST(
			$1::text[], $2::text[], $3::text[], $4::text[], $5::text[],
			$6::int[], $7::decimal[], $8::timestamptz[], $9::timestamptz[], $10::boolean[]
		)
	`

	eventIDs := make([]string, len(odds))
	sportKeys := make([]string, len(odds))
	marketKeys := make([]string, len(odds))
	bookKeys := make([]string, len(odds))
	outcomeNames := make([]string, len(odds))
	prices := make([]int, len(odds))
	points := make([]*float64, len(odds))
	vendorUpdates := make([]time.Time, len(odds))
	receivedAts := make([]time.Time, len(odds))
	isLatests := make([]bool, len(odds))

	for i, odd := range odds {
		eventIDs[i] = odd.EventID
		sportKeys[i] = odd.SportKey
		marketKeys[i] = odd.MarketKey
		bookKeys[i] = odd.BookKey
		outcomeNames[i] = odd.OutcomeName
		prices[i] = odd.Price
		points[i] = odd.Point
		vendorUpdates[i] = odd.VendorLastUpdate
		receivedAts[i] = odd.ReceivedAt
		isLatests[i] = true
	}

	_, err := tx.ExecContext(ctx, query,
		pq.Array(eventIDs), pq.Array(sportKeys), pq.Array(marketKeys), pq.Array(bookKeys), pq.Array(outcomeNames),
		pq.Array(prices), pq.Array(points), pq.Array(vendorUpdates), pq.Array(receivedAts), pq.Array(isLatests),
	)

	return err
}

// publishToStream publishes odds deltas to Redis Stream
func (w *Writer) publishToStream(ctx context.Context, odds []models.RawOdds, events []models.Event) error {
	if len(odds) == 0 {
		return nil
	}

	// Build event status lookup map
	eventStatusMap := make(map[string]string)
	for _, event := range events {
		eventStatusMap[event.EventID] = event.EventStatus
	}

	// Group by sport for separate streams
	bySport := make(map[string][]models.RawOdds)
	for _, odd := range odds {
		bySport[odd.SportKey] = append(bySport[odd.SportKey], odd)
	}

	// Publish to each sport's stream
	for sportKey, sportOdds := range bySport {
		streamKey := fmt.Sprintf(streamKeyFormat, sportKey)

		pipe := w.redis.Pipeline()

		for _, odd := range sportOdds {
			// Get event status from map, default to "upcoming" if not found
			eventStatus := eventStatusMap[odd.EventID]
			if eventStatus == "" {
				eventStatus = "upcoming"
			}

			msg := StreamMessage{
				EventID:          odd.EventID,
				SportKey:         odd.SportKey,
				MarketKey:        odd.MarketKey,
				BookKey:          odd.BookKey,
				OutcomeName:      odd.OutcomeName,
				Price:            odd.Price,
				Point:            odd.Point,
				VendorLastUpdate: odd.VendorLastUpdate,
				ReceivedAt:       odd.ReceivedAt,
				EventStatus:      eventStatus,
			}

			msgJSON, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("marshal stream message: %w", err)
			}

			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: streamKey,
				Values: map[string]interface{}{
					"data": msgJSON,
				},
			})
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("redis pipeline exec for stream: %w", err)
		}
	}

	return nil
}

// upsertEventsFromList inserts or updates events in the events table
func (w *Writer) upsertEventsFromList(ctx context.Context, tx *sql.Tx, events []models.Event) error {
	if len(events) == 0 {
		return nil
	}

	// Build UPSERT statement using UNNEST for batch insert
	query := `
		INSERT INTO events (
			event_id, sport_key, home_team, away_team, commence_time, event_status
		)
		SELECT UNNEST($1::text[]), UNNEST($2::text[]), UNNEST($3::text[]), 
		       UNNEST($4::text[]), UNNEST($5::timestamptz[]), UNNEST($6::text[])
		ON CONFLICT (event_id) 
		DO UPDATE SET 
			home_team = EXCLUDED.home_team,
			away_team = EXCLUDED.away_team,
			commence_time = EXCLUDED.commence_time,
			event_status = EXCLUDED.event_status
	`

	eventIDs := make([]string, len(events))
	sportKeys := make([]string, len(events))
	homeTeams := make([]string, len(events))
	awayTeams := make([]string, len(events))
	commenceTimes := make([]time.Time, len(events))
	statuses := make([]string, len(events))

	for i, evt := range events {
		eventIDs[i] = evt.EventID
		sportKeys[i] = evt.SportKey
		homeTeams[i] = evt.HomeTeam
		awayTeams[i] = evt.AwayTeam
		commenceTimes[i] = evt.CommenceTime
		statuses[i] = evt.EventStatus
	}

	_, err := tx.ExecContext(ctx, query,
		pq.Array(eventIDs), pq.Array(sportKeys), pq.Array(homeTeams),
		pq.Array(awayTeams), pq.Array(commenceTimes), pq.Array(statuses),
	)

	return err
}

// upsertBooksFromOdds extracts unique books from odds and inserts them if they don't exist
func (w *Writer) upsertBooksFromOdds(ctx context.Context, tx *sql.Tx, odds []models.RawOdds) error {
	if len(odds) == 0 {
		return nil
	}

	// Extract unique books
	bookMap := make(map[string]string) // book_key -> sport_key
	for _, odd := range odds {
		bookMap[odd.BookKey] = odd.SportKey
	}

	if len(bookMap) == 0 {
		return nil
	}

	// Build UPSERT statement for books
	// We'll insert with minimal info and let manual seed data provide full details
	query := `
		INSERT INTO books (book_key, display_name, book_type, active, regions, supported_sports)
		SELECT UNNEST($1::text[]), UNNEST($2::text[]), 'soft', true, ARRAY['us'], ARRAY[UNNEST($3::text[])]
		ON CONFLICT (book_key) DO NOTHING
	`

	bookKeys := make([]string, 0, len(bookMap))
	displayNames := make([]string, 0, len(bookMap))
	sportKeys := make([]string, 0, len(bookMap))

	for bookKey, sportKey := range bookMap {
		bookKeys = append(bookKeys, bookKey)
		// Capitalize first letter for display name
		displayNames = append(displayNames, capitalizeFirst(bookKey))
		sportKeys = append(sportKeys, sportKey)
	}

	_, err := tx.ExecContext(ctx, query,
		pq.Array(bookKeys), pq.Array(displayNames), pq.Array(sportKeys),
	)

	return err
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// filterEUBooks only accepts Pinnacle from EU region books
// This prevents foreign key errors from unknown EU bookmakers
func filterEUBooks(odds []models.RawOdds) []models.RawOdds {
	// EU books we want to accept (currently only Pinnacle)
	allowedEUBooks := map[string]bool{
		"pinnacle": true,
	}

	filtered := make([]models.RawOdds, 0, len(odds))
	for _, odd := range odds {
		bookKey := strings.ToLower(odd.BookKey)

		// Check if this is a known EU-only book
		// If it's an allowed EU book OR any other book (US/US2), accept it
		// This filters out unknown EU books while keeping Pinnacle
		if isEUOnlyBook(bookKey) {
			// Only accept if in allowed list
			if allowedEUBooks[bookKey] {
				filtered = append(filtered, odd)
			}
			// Otherwise skip this book
		} else {
			// Not an EU-only book, accept it (US/US2 books)
			filtered = append(filtered, odd)
		}
	}

	return filtered
}

// isEUOnlyBook checks if a book is EU-exclusive
// US books that also appear in EU are NOT considered EU-only
func isEUOnlyBook(bookKey string) bool {
	euOnlyBooks := map[string]bool{
		"pinnacle":        true,
		"betfair_ex_eu":   true,
		"matchbook":       true,
		"marathonbet":     true,
		"betsson":         true,
		"coolbet":         true,
		"nordicbet":       true,
		"unibet_se":       true,
		"unibet_fr":       true,
		"unibet_it":       true,
		"unibet_nl":       true,
		"leovegas_se":     true,
		"tipico_de":       true,
		"winamax_fr":      true,
		"winamax_de":      true,
		"betclic_fr":      true,
		"parionssport_fr": true,
		"suprabets":       true,
		"onexbet":         true,
	}

	return euOnlyBooks[bookKey]
}

// identifyNewEvents returns events that haven't been seen before
// This is used to trigger page warming only for genuinely new events
func (w *Writer) identifyNewEvents(events []models.Event) []models.Event {
	if len(events) == 0 {
		return nil
	}

	w.seenEventsMu.Lock()
	defer w.seenEventsMu.Unlock()

	newEvents := make([]models.Event, 0)
	for _, evt := range events {
		if !w.seenEvents[evt.EventID] {
			w.seenEvents[evt.EventID] = true
			newEvents = append(newEvents, evt)
		}
	}

	return newEvents
}

// warmGamePages sends OpenGamePage requests to Talos for new events
// Only warms events within 72 hours (most sportsbooks only list 1-3 days ahead)
// Rate limited to 1 second between requests to avoid overwhelming Talos
func (w *Writer) warmGamePages(ctx context.Context, events []models.Event) {
	if w.talos == nil || !w.talos.IsEnabled() {
		return
	}

	// Filter events that should be warmed
	now := time.Now()
	warmWindow := 72 * time.Hour // Match sportsbook availability window

	var toWarm []models.Event
	var skippedFuture int

	for _, evt := range events {
		// Only warm pages for upcoming events (not live or completed)
		if evt.EventStatus != "" && evt.EventStatus != "upcoming" {
			continue
		}

		// Skip if commence time is in the past
		if evt.CommenceTime.Before(now) {
			continue
		}

		// Skip if event is too far in the future (sportsbook won't have it listed)
		if evt.CommenceTime.After(now.Add(warmWindow)) {
			skippedFuture++
			continue
		}

		toWarm = append(toWarm, evt)
	}

	if len(toWarm) == 0 {
		if skippedFuture > 0 {
			fmt.Printf("[Writer] Skipped %d events beyond 72h window\n", skippedFuture)
		}
		return
	}

	if skippedFuture > 0 {
		fmt.Printf("[Writer] Warming %d events (skipped %d beyond 72h window)\n", len(toWarm), skippedFuture)
	} else {
		fmt.Printf("[Writer] Warming %d new events...\n", len(toWarm))
	}

	// Send page warm requests with rate limiting
	// Use a goroutine to avoid blocking the writer, but rate limit internally
	go func() {
		for i, e := range toWarm {
			warmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			if err := w.talos.OpenGamePage(warmCtx, e.HomeTeam, e.AwayTeam, e.SportKey, e.CommenceTime); err != nil {
				fmt.Printf("[Writer] Page warm failed for %s @ %s: %v\n", e.AwayTeam, e.HomeTeam, err)
			}

			cancel()

			// Rate limit: 1 second between requests, except after last
			if i < len(toWarm)-1 {
				time.Sleep(1 * time.Second)
			}
		}
	}()
}

// ClearSeenEvents clears the seen events cache (useful for testing or restarts)
func (w *Writer) ClearSeenEvents() {
	w.seenEventsMu.Lock()
	defer w.seenEventsMu.Unlock()
	w.seenEvents = make(map[string]bool)
}

// LoadSeenEventsFromDB loads existing event IDs from the database
// Call this on startup to prevent re-warming events that are already in DB
func (w *Writer) LoadSeenEventsFromDB(ctx context.Context) error {
	query := `
		SELECT event_id FROM events
		WHERE event_status IN ('upcoming', 'live')
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query seen events: %w", err)
	}
	defer rows.Close()

	w.seenEventsMu.Lock()
	defer w.seenEventsMu.Unlock()

	count := 0
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			continue
		}
		w.seenEvents[eventID] = true
		count++
	}

	fmt.Printf("[Writer] Loaded %d existing events into seenEvents cache\n", count)
	return nil
}

// WarmUpcomingEvents sends OpenGamePage requests for ALL upcoming events on startup
// This ensures game pages are warmed even if Talos was down when Mercury discovered the events.
//
// Key behavior:
// - Warms ALL events within 72 hours (sportsbook availability window)
// - Does NOT check seenEvents - that's only for preventing duplicates during polling
// - Marks events as seen AFTER queuing warm request (prevents re-warming during polling)
// - Talos has deduplication at the bot level, so duplicate requests are safe
func (w *Writer) WarmUpcomingEvents(ctx context.Context) error {
	if w.talos == nil || !w.talos.IsEnabled() {
		fmt.Println("[Writer] Talos client not enabled, skipping warm-up")
		return nil
	}

	// Only warm events in the near future that sportsbooks will have listed
	// Most sportsbooks show games 1-3 days ahead, use 72 hours as safe window
	query := `
		SELECT event_id, sport_key, home_team, away_team, commence_time
		FROM events
		WHERE event_status = 'upcoming'
		  AND commence_time > NOW()
		  AND commence_time < NOW() + INTERVAL '72 hours'
		ORDER BY commence_time ASC
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query upcoming events: %w", err)
	}
	defer rows.Close()

	// Collect ALL events within 72h window - don't check seenEvents
	// seenEvents is for polling deduplication, not startup
	var eventsToWarm []models.Event

	for rows.Next() {
		var evt models.Event
		if err := rows.Scan(&evt.EventID, &evt.SportKey, &evt.HomeTeam, &evt.AwayTeam, &evt.CommenceTime); err != nil {
			fmt.Printf("[Writer] Scan warning: %v\n", err)
			continue
		}
		evt.EventStatus = "upcoming"
		eventsToWarm = append(eventsToWarm, evt)
	}

	if len(eventsToWarm) == 0 {
		fmt.Println("[Writer] No upcoming events within 72h window to warm")
		return nil
	}

	fmt.Printf("[Writer] Startup warm-up: sending %d events to Talos (Talos will deduplicate)...\n", len(eventsToWarm))

	// Warm pages for all events
	for _, evt := range eventsToWarm {
		// Mark as seen so polling doesn't re-warm these
		w.seenEventsMu.Lock()
		w.seenEvents[evt.EventID] = true
		w.seenEventsMu.Unlock()

		// Send warm request (async)
		go func(e models.Event) {
			warmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := w.talos.OpenGamePage(warmCtx, e.HomeTeam, e.AwayTeam, e.SportKey, e.CommenceTime); err != nil {
				fmt.Printf("[Writer] Warm-up failed for %s @ %s: %v\n", e.AwayTeam, e.HomeTeam, err)
			}
		}(evt)

		// Rate limit: 1 second between requests to avoid overwhelming Talos
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("[Writer] Warm-up requests sent for %d events\n", len(eventsToWarm))
	return nil
}
