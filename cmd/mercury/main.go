package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XavierBriggs/Mercury/adapters/theoddsapi"
	"github.com/XavierBriggs/Mercury/internal/registry"
	"github.com/XavierBriggs/Mercury/internal/scheduler"
	"github.com/XavierBriggs/Mercury/sports/basketball_nba"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// Load configuration from environment
	config := loadConfig()

	// Initialize Alexandria DB connection
	db, err := sql.Open("postgres", config.AlexandriaDSN)
	if err != nil {
		fmt.Printf("failed to connect to Alexandria DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Test DB connection
	if err := db.PingContext(ctx); err != nil {
		fmt.Printf("failed to ping Alexandria DB: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Connected to Alexandria DB")

	// Initialize Redis connection
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisURL,
		Password: config.RedisPassword,
	})
	defer redisClient.Close()

	// Test Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		fmt.Printf("failed to connect to Redis: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Connected to Redis")

	// Initialize The Odds API adapter
	adapter := theoddsapi.NewClient(config.OddsAPIKey)

	fmt.Println("✓ Initialized The Odds API adapter")

	// Initialize sport registry and register active sports
	sportRegistry := registry.NewSportRegistry()
	
	// Register NBA
	nbaModule := basketball_nba.NewModule()
	if err := sportRegistry.Register(nbaModule); err != nil {
		fmt.Printf("failed to register NBA module: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("✓ Registered %d sport(s)\n", sportRegistry.Count())

	// Initialize scheduler
	sched := scheduler.NewScheduler(db, redisClient, adapter, config.CacheTTL, sportRegistry)

	// Start scheduler
	if err := sched.Start(ctx); err != nil {
		fmt.Printf("failed to start scheduler: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Mercury started - polling odds")
	fmt.Printf("  Cache TTL: %v\n", config.CacheTTL)
	fmt.Println()
	
	// Show registered sports
	for _, sport := range sportRegistry.GetAll() {
		fmt.Printf("  [%s]\n", sport.GetDisplayName())
		fmt.Printf("    Regions: %v\n", sport.GetRegions())
		fmt.Printf("    Markets: %v\n", sport.GetFeaturedMarkets())
		fmt.Printf("    Poll Interval: %v\n", sport.GetFeaturedPollInterval())
		if sport.ShouldPollProps() {
			fmt.Printf("    Props Discovery: every %v\n", sport.GetPropsDiscoveryInterval())
		}
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\n✓ Shutting down gracefully...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sched.Stop()

	select {
	case <-shutdownCtx.Done():
		fmt.Println("✗ Shutdown timeout exceeded")
		os.Exit(1)
	default:
		fmt.Println("✓ Mercury stopped")
	}
}

// Config holds Mercury configuration
type Config struct {
	AlexandriaDSN string
	RedisURL      string
	RedisPassword string
	OddsAPIKey    string
	CacheTTL      time.Duration
}

// loadConfig loads configuration from environment variables
func loadConfig() Config {
	// Parse cache TTL (default 5 minutes)
	cacheTTL := 5 * time.Minute
	if ttlStr := os.Getenv("MERCURY_CACHE_TTL"); ttlStr != "" {
		if parsed, err := time.ParseDuration(ttlStr); err == nil {
			cacheTTL = parsed
		} else {
			fmt.Printf("⚠ Invalid MERCURY_CACHE_TTL '%s', using default 5m\n", ttlStr)
		}
	}

	config := Config{
		AlexandriaDSN: getEnv("ALEXANDRIA_DSN", "postgres://fortuna:fortuna@localhost:5432/alexandria?sslmode=disable"),
		RedisURL:      getEnv("REDIS_URL", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		OddsAPIKey:    getEnv("ODDS_API_KEY", ""),
		CacheTTL:      cacheTTL,
	}

	if config.OddsAPIKey == "" {
		fmt.Println("✗ ODDS_API_KEY environment variable is required")
		os.Exit(1)
	}

	return config
}

// getEnv gets an environment variable with a default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

