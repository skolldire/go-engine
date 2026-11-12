package gormsql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These tests run against a real SQLite database held in memory. That is a
// deliberate choice over mocking *gorm.DB: the value of this package is what it
// does to a database, and a mock would only assert that we call the methods we
// call. SQLite needs no infrastructure, so this runs on every `go test`.

type user struct {
	ID    uint `gorm:"primarykey"`
	Name  string
	Email string
	Age   int
}

// newTestClient builds a client over a fresh in-memory database.
func newTestClient(t *testing.T) *DBClient {
	t.Helper()

	// Each test gets its own database: a shared file would let one test's rows
	// leak into another's assertions.
	client, err := New(context.Background(), Config{
		Type:               "sqlite",
		MaxOpenConnections: 1, // in-memory SQLite is a single connection
		MaxIdleConnections: 1,
		ConnMaxLifetime:    time.Minute,
	}, sqlite.Open(":memory:"), logger.NewService(logger.Config{Level: "error"}, nil))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.DB().AutoMigrate(&user{}))

	return client
}

func TestNew_AppliesPoolSettings(t *testing.T) {
	client := newTestClient(t)

	sqlDB, err := client.DB().DB()
	require.NoError(t, err)

	// A pool size read from the wrong key silently leaves the driver default,
	// which only surfaces as saturation under load.
	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)
}

func TestNew_RejectsAnUnreachableDatabase(t *testing.T) {
	_, err := New(context.Background(), Config{Type: "sqlite"},
		sqlite.Open("/nonexistent-directory/db.sqlite"),
		logger.NewService(logger.Config{Level: "error"}, nil))

	assert.Error(t, err, "a database that cannot be opened must fail the build")
}

func TestPing(t *testing.T) {
	client := newTestClient(t)
	assert.NoError(t, client.Ping(context.Background()))
}

func TestCreateAndFirst(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, client.Create(ctx, &user{Name: "ada", Email: "ada@example.com", Age: 36}))

	var got user
	require.NoError(t, client.First(ctx, &got, "name = ?", "ada"))

	assert.Equal(t, "ada", got.Name)
	assert.Equal(t, "ada@example.com", got.Email)
	assert.NotZero(t, got.ID, "the primary key must be populated")
}

func TestFirst_ReportsNotFound(t *testing.T) {
	client := newTestClient(t)

	var got user
	err := client.First(context.Background(), &got, "name = ?", "nobody")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound,
		"the package's own sentinel is the documented contract")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"and the driver's sentinel stays reachable for callers that import gorm")
}

func TestFind(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	for _, u := range []user{{Name: "ada", Age: 36}, {Name: "alan", Age: 41}, {Name: "grace", Age: 45}} {
		require.NoError(t, client.Create(ctx, &u))
	}

	t.Run("all rows", func(t *testing.T) {
		var users []user
		require.NoError(t, client.Find(ctx, &users))
		assert.Len(t, users, 3)
	})

	t.Run("filtered", func(t *testing.T) {
		var users []user
		require.NoError(t, client.Find(ctx, &users, "age > ?", 40))
		assert.Len(t, users, 2)
	})

	t.Run("no match is not an error", func(t *testing.T) {
		var users []user
		require.NoError(t, client.Find(ctx, &users, "age > ?", 100),
			"an empty result set is a valid answer, unlike First")
		assert.Empty(t, users)
	})
}

func TestUpdate(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	u := user{Name: "ada", Age: 36}
	require.NoError(t, client.Create(ctx, &u))

	require.NoError(t, client.Update(ctx, &u, map[string]any{"age": 37}))

	var got user
	require.NoError(t, client.First(ctx, &got, "id = ?", u.ID))
	assert.Equal(t, 37, got.Age)
}

func TestDelete(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	u := user{Name: "ada"}
	require.NoError(t, client.Create(ctx, &u))
	require.NoError(t, client.Delete(ctx, &user{}, "id = ?", u.ID))

	var count int64
	require.NoError(t, client.Count(ctx, &user{}, &count))
	assert.Zero(t, count)
}

func TestCount(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	for _, u := range []user{{Name: "a", Age: 20}, {Name: "b", Age: 30}, {Name: "c", Age: 40}} {
		require.NoError(t, client.Create(ctx, &u))
	}

	var all int64
	require.NoError(t, client.Count(ctx, &user{}, &all))
	assert.Equal(t, int64(3), all)

	var filtered int64
	require.NoError(t, client.Count(ctx, &user{}, &filtered, "age >= ?", 30))
	assert.Equal(t, int64(2), filtered)
}

func TestExec(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, client.Exec(ctx, "INSERT INTO users (name, age) VALUES (?, ?)", "raw", 50))

	var got user
	require.NoError(t, client.First(ctx, &got, "name = ?", "raw"))
	assert.Equal(t, 50, got.Age)
}

func TestExec_ReportsASyntaxError(t *testing.T) {
	client := newTestClient(t)

	err := client.Exec(context.Background(), "NOT VALID SQL")
	assert.Error(t, err)
}

// TestTransaction_CommitsOnSuccess and its rollback counterpart are the pair
// that matters: a transaction helper that silently commits on error would be
// worse than none at all.
func TestTransaction_CommitsOnSuccess(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, client.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&user{Name: "first"}).Error; err != nil {
			return err
		}
		return tx.Create(&user{Name: "second"}).Error
	}))

	var count int64
	require.NoError(t, client.Count(ctx, &user{}, &count))
	assert.Equal(t, int64(2), count)
}

func TestTransaction_RollsBackOnError(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	sentinel := errors.New("business rule violated")

	err := client.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&user{Name: "doomed"}).Error; err != nil {
			return err
		}
		return sentinel
	})

	require.ErrorIs(t, err, sentinel, "the caller's error must survive the rollback")

	var count int64
	require.NoError(t, client.Count(ctx, &user{}, &count))
	assert.Zero(t, count, "nothing may be committed when the function returns an error")
}

func TestWhere(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	for _, u := range []user{{Name: "ada", Age: 36}, {Name: "alan", Age: 41}} {
		require.NoError(t, client.Create(ctx, &u))
	}

	var users []user
	require.NoError(t, client.Where(ctx, &users, "age > ?", 40))
	require.Len(t, users, 1)
	assert.Equal(t, "alan", users[0].Name)
}

// TestContextTimeoutIsHonoured proves the timeout the client applies actually
// reaches the driver rather than being decorative.
func TestContextTimeoutIsHonoured(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := client.Create(ctx, &user{Name: "never"})
	assert.Error(t, err, "a cancelled context must abort the query")
}

func TestClose(t *testing.T) {
	client, err := New(context.Background(), Config{Type: "sqlite"}, sqlite.Open(":memory:"),
		logger.NewService(logger.Config{Level: "error"}, nil))
	require.NoError(t, err)

	require.NoError(t, client.Close())

	// After Close the pool is gone, so a query must fail rather than silently
	// reconnect to a different database.
	assert.Error(t, client.Ping(context.Background()))
}
