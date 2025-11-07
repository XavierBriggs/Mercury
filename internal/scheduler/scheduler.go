package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/XavierBriggs/Mercury/internal/delta"
	"github.com/XavierBriggs/Mercury/internal/writer"
	"github.com/XavierBriggs/Mercury/pkg/contracts"
	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/XavierBriggs/Mercury/sports/basketball_nba"
	"github.com/redis/go-redis/v9"
)

// Scheduler orchestrates polling for all active sports
type Scheduler struct {
	adapter      contracts.VendorAdapter
	deltaEngine  *delta.Engine
	writer       *writer.Writer
	nbaConfig    *basketball_nba.Config
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewScheduler creates a new polling scheduler
func NewScheduler(
	db *sql.DB,
	redisClient *redis.Client,
	adapter contracts.VendorAdapter,
	cacheTTL time.Duration,
) *Scheduler {
	return &Scheduler{
		adapter:     adapter,
		deltaEngine: delta.NewEngine(redisClient, cacheTTL),
		writer:      writer.NewWriter(db, redisClient),
		nbaConfig:   basketball_nba.DefaultConfig(),
		stopChan:    make(chan struct{}),
	}
}

// Start begins polling for all active sports
func (s *Scheduler) Start(ctx context.Context) error {
	// Start writer's background flush
	s.writer.Start(ctx)

	// Start NBA featured markets polling
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.pollNBAFeatured(ctx)
	}()

	// Start NBA props discovery sweep
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.discoverNBAProps(ctx)
	}()

	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	s.writer.Stop()
}

// pollNBAFeatured polls featured markets (h2h, spreads, totals) with Plan A cadence
func (s *Scheduler) pollNBAFeatured(ctx context.Context) {
	// Initial poll immediately
	if err := s.fetchAndProcess(ctx, &models.FetchOddsOptions{
		Sport:   s.nbaConfig.SportKey,
		Regions: s.nbaConfig.Regions,
		Markets: basketball_nba.FeaturedMarkets(),
	}); err != nil {
		fmt.Printf("initial featured poll error: %v\n", err)
	}

	// Dynamic ticker based on event times
	ticker := time.NewTicker(60 * time.Second) // Start with pre-match interval
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.fetchAndProcess(ctx, &models.FetchOddsOptions{
				Sport:   s.nbaConfig.SportKey,
				Regions: s.nbaConfig.Regions,
				Markets: basketball_nba.FeaturedMarkets(),
			}); err != nil {
				fmt.Printf("featured poll error: %v\n", err)
			}

			// TODO: Adjust ticker interval based on nearest event time
			// For v0, using fixed 60s interval (will enhance in I3)

		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// discoverNBAProps performs discovery sweep for props every 6 hours
func (s *Scheduler) discoverNBAProps(ctx context.Context) {
	ticker := time.NewTicker(s.nbaConfig.Props.DiscoverySweepInterval)
	defer ticker.Stop()

	// Initial discovery immediately
	if err := s.discoverProps(ctx); err != nil {
		fmt.Printf("initial props discovery error: %v\n", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := s.discoverProps(ctx); err != nil {
				fmt.Printf("props discovery error: %v\n", err)
			}

		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// discoverProps fetches upcoming events and schedules props polling
func (s *Scheduler) discoverProps(ctx context.Context) error {
	events, err := s.adapter.FetchEvents(ctx, s.nbaConfig.SportKey)
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}

	// Filter events within 48hr window
	now := time.Now()
	windowEnd := now.Add(time.Duration(s.nbaConfig.Props.DiscoveryWindowHours) * time.Hour)

	eventsInWindow := make([]models.Event, 0)
	for _, evt := range events {
		if evt.CommenceTime.After(now) && evt.CommenceTime.Before(windowEnd) {
			eventsInWindow = append(eventsInWindow, evt)
		}
	}

	fmt.Printf("discovered %d events in next 48hr window\n", len(eventsInWindow))

	// TODO: Store discovered events and schedule ramped polling
	// For v0, will implement full ramping in I3

	return nil
}

// fetchAndProcess executes the full pipeline: fetch → delta → write → cache update
func (s *Scheduler) fetchAndProcess(ctx context.Context, opts *models.FetchOddsOptions) error {
	start := time.Now()

	// Step 1: Fetch odds from vendor (includes events)
	result, err := s.adapter.FetchOdds(ctx, opts)
	if err != nil {
		return fmt.Errorf("fetch odds: %w", err)
	}

	if len(result.Odds) == 0 {
		return nil // No odds available
	}

	fetchDuration := time.Since(start)

	// Step 2: Detect deltas (Redis-first, <1ms)
	deltas, err := s.deltaEngine.DetectChanges(ctx, result.Odds)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}

	deltaDuration := time.Since(start) - fetchDuration

	if len(deltas) == 0 {
		// No changes, skip write
		return nil
	}

	// Step 3: Write deltas to Alexandria (batched, includes event upsert)
	deltaOdds := make([]models.RawOdds, len(deltas))
	for i, d := range deltas {
		deltaOdds[i] = d.Odd
	}

	if err := s.writer.WriteWithEvents(ctx, result.Events, deltaOdds); err != nil {
		return fmt.Errorf("write deltas: %w", err)
	}

	writeDuration := time.Since(start) - fetchDuration - deltaDuration

	// Step 4: Update Redis cache (write-through)
	if err := s.deltaEngine.UpdateCache(ctx, deltaOdds); err != nil {
		// Log but don't fail - cache will rebuild
		fmt.Printf("update cache error: %v\n", err)
	}

	cacheDuration := time.Since(start) - fetchDuration - deltaDuration - writeDuration

	// Metrics logging (would use proper metrics in production)
	totalDuration := time.Since(start)
	fmt.Printf("poll complete: %d events, %d odds, %d deltas, fetch=%v delta=%v write=%v cache=%v total=%v\n",
		len(result.Events), len(result.Odds), len(deltas), fetchDuration, deltaDuration, writeDuration, cacheDuration, totalDuration)

	// Check if we're meeting SLO (<30ms for Mercury component)
	if totalDuration > 30*time.Millisecond {
		fmt.Printf("WARNING: poll exceeded 30ms SLO: %v\n", totalDuration)
	}

	return nil
}

// addJitter adds random jitter to prevent synchronization
func addJitter(duration time.Duration, jitterSeconds int) time.Duration {
	if jitterSeconds == 0 {
		return duration
	}

	jitter := time.Duration(rand.Intn(jitterSeconds)) * time.Second
	return duration + jitter
}

