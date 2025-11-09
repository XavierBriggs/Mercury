package closer

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestCapturer_captureEventClosingLines(t *testing.T) {
	// Create mock DB
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	// Create Redis client (mock or use miniredis)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	capturer := NewCapturer(db, redisClient, 30*time.Second)
	ctx := context.Background()

	eventID := "test-event-123"

	// Expect transaction begin
	mock.ExpectBegin()

	// Expect insert query
	mock.ExpectExec(`INSERT INTO closing_lines`).
		WithArgs(eventID).
		WillReturnResult(sqlmock.NewResult(0, 5)) // 5 lines captured

	// Expect commit
	mock.ExpectCommit()

	// Execute capture
	err = capturer.captureEventClosingLines(ctx, eventID)
	assert.NoError(t, err)

	// Verify all expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestCapturer_captureClosingLines_NoEventsLive(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	capturer := NewCapturer(db, redisClient, 30*time.Second)
	ctx := context.Background()

	// Expect query that returns no rows
	mock.ExpectQuery(`SELECT DISTINCT e.event_id`).
		WillReturnRows(sqlmock.NewRows([]string{"event_id"}))

	err = capturer.captureClosingLines(ctx)
	assert.NoError(t, err)

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestCapturer_captureClosingLines_WithLiveEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	capturer := NewCapturer(db, redisClient, 30*time.Second)
	ctx := context.Background()

	// Mock finding live events
	eventRows := sqlmock.NewRows([]string{"event_id"}).
		AddRow("event-1").
		AddRow("event-2")

	mock.ExpectQuery(`SELECT DISTINCT e.event_id`).
		WillReturnRows(eventRows)

	// Mock capturing lines for event-1
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO closing_lines`).
		WithArgs("event-1").
		WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectCommit()

	// Mock capturing lines for event-2
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO closing_lines`).
		WithArgs("event-2").
		WillReturnResult(sqlmock.NewResult(0, 8))
	mock.ExpectCommit()

	err = capturer.captureClosingLines(ctx)
	assert.NoError(t, err)

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

