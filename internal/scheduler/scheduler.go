package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/XavierBriggs/Mercury/internal/delta"
	"github.com/XavierBriggs/Mercury/internal/registry"
	"github.com/XavierBriggs/Mercury/internal/writer"
	"github.com/XavierBriggs/Mercury/pkg/contracts"
	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/redis/go-redis/v9"
)

// Scheduler orchestrates polling for all registered sports
type Scheduler struct {
	adapter      contracts.VendorAdapter
	deltaEngine  *delta.Engine
	writer       *writer.Writer
	sportRegistry *registry.SportRegistry
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewScheduler creates a new polling scheduler
func NewScheduler(
	db *sql.DB,
	redisClient *redis.Client,
	adapter contracts.VendorAdapter,
	cacheTTL time.Duration,
	sportRegistry *registry.SportRegistry,
) *Scheduler {
	return &Scheduler{
		adapter:       adapter,
		deltaEngine:   delta.NewEngine(redisClient, cacheTTL),
		writer:        writer.NewWriter(db, redisClient),
		sportRegistry: sportRegistry,
		stopChan:      make(chan struct{}),
	}
}

// Start begins polling for all registered sports
func (s *Scheduler) Start(ctx context.Context) error {
	// Start writer's background flush
	s.writer.Start(ctx)

	// Start polling for each registered sport
	sports := s.sportRegistry.GetAll()
	if len(sports) == 0 {
		return fmt.Errorf("no sports registered")
	}

	for _, sport := range sports {
		// Start featured markets polling for this sport
		s.wg.Add(1)
		go func(sport contracts.SportModule) {
			defer s.wg.Done()
			s.pollSportFeatured(ctx, sport)
		}(sport)

		// Start props discovery if enabled for this sport
		if sport.ShouldPollProps() {
			s.wg.Add(1)
			go func(sport contracts.SportModule) {
				defer s.wg.Done()
				s.discoverSportProps(ctx, sport)
			}(sport)
		}

		fmt.Printf("✓ Started polling for %s\n", sport.GetDisplayName())
	}

	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	s.writer.Stop()
}

// pollSportFeatured polls featured markets for a specific sport
func (s *Scheduler) pollSportFeatured(ctx context.Context, sport contracts.SportModule) {
	// Initial poll immediately
	if err := s.fetchAndProcess(ctx, &models.FetchOddsOptions{
		Sport:   sport.GetSportKey(),
		Regions: sport.GetRegions(),
		Markets: sport.GetFeaturedMarkets(),
	}); err != nil {
		fmt.Printf("[%s] initial featured poll error: %v\n", sport.GetDisplayName(), err)
	}

	// Dynamic ticker based on sport configuration
	ticker := time.NewTicker(sport.GetFeaturedPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.fetchAndProcess(ctx, &models.FetchOddsOptions{
				Sport:   sport.GetSportKey(),
				Regions: sport.GetRegions(),
				Markets: sport.GetFeaturedMarkets(),
			}); err != nil {
				fmt.Printf("[%s] featured poll error: %v\n", sport.GetDisplayName(), err)
			}

			// TODO: Adjust ticker interval based on nearest event time
			// For v0, using fixed intervals (will enhance in I3)

		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// discoverSportProps performs discovery sweep for props
func (s *Scheduler) discoverSportProps(ctx context.Context, sport contracts.SportModule) {
	ticker := time.NewTicker(sport.GetPropsDiscoveryInterval())
	defer ticker.Stop()

	// Initial discovery immediately
	if err := s.discoverProps(ctx, sport); err != nil {
		fmt.Printf("[%s] initial props discovery error: %v\n", sport.GetDisplayName(), err)
	}

	for {
		select {
		case <-ticker.C:
			if err := s.discoverProps(ctx, sport); err != nil {
				fmt.Printf("[%s] props discovery error: %v\n", sport.GetDisplayName(), err)
			}

		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// discoverProps fetches upcoming events and schedules props polling for a sport
func (s *Scheduler) discoverProps(ctx context.Context, sport contracts.SportModule) error {
	events, err := s.adapter.FetchEvents(ctx, sport.GetSportKey())
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}

	// Filter events within discovery window
	now := time.Now()
	windowEnd := now.Add(time.Duration(sport.GetPropsDiscoveryWindowHours()) * time.Hour)

	eventsInWindow := make([]models.Event, 0)
	for _, evt := range events {
		if evt.CommenceTime.After(now) && evt.CommenceTime.Before(windowEnd) {
			eventsInWindow = append(eventsInWindow, evt)
		}
	}

	fmt.Printf("[%s] discovered %d events in next %dhr window\n", 
		sport.GetDisplayName(), len(eventsInWindow), sport.GetPropsDiscoveryWindowHours())

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

