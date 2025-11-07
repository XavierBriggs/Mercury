package delta

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/XavierBriggs/Mercury/pkg/models"
	"github.com/redis/go-redis/v9"
)

// Engine detects changes in odds by comparing against Redis cache
// This is the Redis-first approach for <1ms delta detection
type Engine struct {
	redis *redis.Client
	ttl   time.Duration
}

// CachedOdd represents the minimal data stored in Redis for comparison
type CachedOdd struct {
	Price            int       `json:"price"`
	Point            *float64  `json:"point,omitempty"`
	VendorLastUpdate time.Time `json:"vendor_last_update"`
}

// ChangeType indicates the type of change detected
type ChangeType string

const (
	ChangeTypeNew       ChangeType = "new"
	ChangeTypePriceOnly ChangeType = "price"
	ChangeTypePointOnly ChangeType = "point"
	ChangeTypeBoth      ChangeType = "price_and_point"
	ChangeTypeNone      ChangeType = "none"
)

// Delta represents a detected change
type Delta struct {
	Odd        models.RawOdds
	ChangeType ChangeType
	OldPrice   *int
	OldPoint   *float64
}

// NewEngine creates a new delta detection engine
func NewEngine(redisClient *redis.Client, cacheTTL time.Duration) *Engine {
	return &Engine{
		redis: redisClient,
		ttl:   cacheTTL,
	}
}

// DetectChanges compares new odds against Redis cache and returns only deltas
// This is the hot path - must be <1ms per call
func (e *Engine) DetectChanges(ctx context.Context, newOdds []models.RawOdds) ([]Delta, error) {
	if len(newOdds) == 0 {
		return nil, nil
	}

	// Build Redis keys for batch lookup
	keys := make([]string, len(newOdds))
	for i, odd := range newOdds {
		keys[i] = e.buildKey(odd)
	}

	// Batch GET from Redis (<1ms for 100s of keys)
	cachedValues, err := e.redis.MGet(ctx, keys...).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis mget: %w", err)
	}

	// Compare and detect changes
	deltas := make([]Delta, 0, len(newOdds))

	for i, odd := range newOdds {
		cachedValue := cachedValues[i]

		changeType, oldPrice, oldPoint := e.compareOdd(odd, cachedValue)

		if changeType != ChangeTypeNone {
			deltas = append(deltas, Delta{
				Odd:        odd,
				ChangeType: changeType,
				OldPrice:   oldPrice,
				OldPoint:   oldPoint,
			})
		}
	}

	return deltas, nil
}

// UpdateCache updates Redis cache with new odds (write-through pattern)
// This should be called after successfully writing to Alexandria
func (e *Engine) UpdateCache(ctx context.Context, odds []models.RawOdds) error {
	if len(odds) == 0 {
		return nil
	}

	// Build SET commands for pipeline
	pipe := e.redis.Pipeline()

	for _, odd := range odds {
		key := e.buildKey(odd)
		cached := CachedOdd{
			Price:            odd.Price,
			Point:            odd.Point,
			VendorLastUpdate: odd.VendorLastUpdate,
		}

		data, err := json.Marshal(cached)
		if err != nil {
			return fmt.Errorf("marshal cached odd: %w", err)
		}

		pipe.Set(ctx, key, data, e.ttl)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis pipeline exec: %w", err)
	}

	return nil
}

// RebuildCache rebuilds Redis cache from Alexandria DB
// Called on startup or after Redis restart
func (e *Engine) RebuildCache(ctx context.Context, currentOdds []models.RawOdds) error {
	return e.UpdateCache(ctx, currentOdds)
}

// buildKey creates a Redis key for an odd
// Format: odds:current:{event_id}:{market_key}:{book_key}:{outcome_name}
func (e *Engine) buildKey(odd models.RawOdds) string {
	return fmt.Sprintf("odds:current:%s:%s:%s:%s",
		odd.EventID,
		odd.MarketKey,
		odd.BookKey,
		odd.OutcomeName,
	)
}

// compareOdd compares a new odd against its cached value
func (e *Engine) compareOdd(newOdd models.RawOdds, cachedValue interface{}) (ChangeType, *int, *float64) {
	// If no cache entry, this is a new outcome
	if cachedValue == nil {
		return ChangeTypeNew, nil, nil
	}

	// Parse cached value
	cachedStr, ok := cachedValue.(string)
	if !ok {
		// Cache corruption, treat as new
		return ChangeTypeNew, nil, nil
	}

	var cached CachedOdd
	if err := json.Unmarshal([]byte(cachedStr), &cached); err != nil {
		// Cache corruption, treat as new
		return ChangeTypeNew, nil, nil
	}

	// Compare price and point
	priceChanged := newOdd.Price != cached.Price
	pointChanged := e.pointChanged(newOdd.Point, cached.Point)

	if !priceChanged && !pointChanged {
		return ChangeTypeNone, nil, nil
	}

	oldPrice := &cached.Price
	var oldPoint *float64
	if cached.Point != nil {
		val := *cached.Point
		oldPoint = &val
	}

	if priceChanged && pointChanged {
		return ChangeTypeBoth, oldPrice, oldPoint
	}

	if priceChanged {
		return ChangeTypePriceOnly, oldPrice, oldPoint
	}

	return ChangeTypePointOnly, oldPrice, oldPoint
}

// pointChanged checks if point values are different
func (e *Engine) pointChanged(newPoint, oldPoint *float64) bool {
	if newPoint == nil && oldPoint == nil {
		return false
	}

	if newPoint == nil || oldPoint == nil {
		return true
	}

	// Compare with small epsilon for float precision
	const epsilon = 0.001
	diff := *newPoint - *oldPoint
	if diff < 0 {
		diff = -diff
	}

	return diff > epsilon
}

