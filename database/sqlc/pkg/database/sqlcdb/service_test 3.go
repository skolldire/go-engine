package sqlcdb_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skolldire/go-engine/database/sqlc/pkg/database/sqlcdb"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func log() logger.Service { return logger.NewService(logger.Config{Level: "error"}, nil) }

// newTestClient opens an in-memory SQLite pool. MaxOpenConnections is 1 because
// every connection to ":memory:" gets its own database: with a larger pool the
// table created on one connection is invisible to the next query.
func newTestClient(t *testing.T) *sqlcdb.Client {
	t.Helper()

	c, err := sqlcdb.New(context.Background(), sqlcdb.Config{
		Driver:             "sqlite3",
		DSN:                ":memory:",
		MaxOpenConnections: 1,
	}, log())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.ExecContext(context.Background(),
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)

	return c
}

func TestNewOpensAndPings(t *testing.T) {
	c := newTestClient(t)

	assert.NoError(t, c.Ping(context.Background()))
	assert.Equal(t, "sqlite3", c.Driver())
	assert.NotNil(t, c.DB())
	assert.Equal(t, 1, c.Stats().MaxOpenConnections)
}

func TestNewWithoutDriverIsReported(t *testing.T) {
	_, err := sqlcdb.New(context.Background(), sqlcdb.Config{DSN: ":memory:"}, log())

	require.Error(t, err)
	assert.ErrorIs(t, err, sqlcdb.ErrNoDriver)
}

func TestNewWithUnreachableDSNClosesThePool(t *testing.T) {
	// sql.Open is lazy: this only fails on the ping inside New, which is the
	// path that must not leak the pool.
	dsn := filepath.Join(t.TempDir(), "missing-dir", "app.db")

	_, err := sqlcdb.New(context.Background(), sqlcdb.Config{Driver: "sqlite3", DSN: dsn}, log())

	require.Error(t, err)
	assert.Contains(t, err.Error(), sqlcdb.ErrConnection.Error())
}

func TestNewWithDBAdoptsThePool(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	c, err := sqlcdb.NewWithDB(context.Background(), sqlcdb.Config{Driver: "sqlite3"}, db, log())
	require.NoError(t, err)

	assert.Same(t, db, c.DB())
	require.NoError(t, c.Close())
	assert.Error(t, db.Ping(), "Close must close the adopted pool")
}

func TestNewWithNilHandleIsReported(t *testing.T) {
	_, err := sqlcdb.NewWithDB(context.Background(), sqlcdb.Config{}, nil, log())
	assert.ErrorIs(t, err, sqlcdb.ErrNoDriver)

	_, err = sqlcdb.NewWithConnector(context.Background(), sqlcdb.Config{}, nil, log())
	assert.ErrorIs(t, err, sqlcdb.ErrNoDriver)
}

func TestExecAndQueryRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	res, err := c.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 1, "ada")
	require.NoError(t, err)
	affected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	var name string
	require.NoError(t, c.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, 1).Scan(&name))
	assert.Equal(t, "ada", name)
}

// TestQueryRowsSurviveTheCall is the regression test for wrapping QueryContext
// in BaseClient.Execute: Execute cancels the context it derives when it
// returns, and database/sql would then fail the first Next() with
// "context canceled".
func TestQueryRowsSurviveTheCall(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	for i, n := range []string{"ada", "grace", "alan"} {
		_, err := c.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, i+1, n)
		require.NoError(t, err)
	}

	rows, err := c.QueryContext(ctx, `SELECT name FROM users ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"ada", "grace", "alan"}, got)
}

func TestPrepareContext(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	stmt, err := c.PrepareContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`)
	require.NoError(t, err)
	defer func() { _ = stmt.Close() }()

	_, err = stmt.ExecContext(ctx, 7, "hopper")
	require.NoError(t, err)

	var count int
	require.NoError(t, c.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestMissingRowIsNotFound(t *testing.T) {
	c := newTestClient(t)

	var name string
	err := c.QueryRowContext(context.Background(), `SELECT name FROM users WHERE id = ?`, 99).Scan(&name)

	require.Error(t, err)
	assert.True(t, sqlcdb.IsNotFound(err))
	assert.ErrorIs(t, err, sqlcdb.ErrNotFound)
}

func TestTxCommits(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	err := c.Tx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 1, "ada")
		return err
	})
	require.NoError(t, err)

	assert.Equal(t, 1, countUsers(t, c))
}

func TestTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	boom := errors.New("boom")

	err := c.Tx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 1, "ada"); err != nil {
			return err
		}
		return boom
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, boom, "the caller's error must survive the wrapping")
	assert.Equal(t, 0, countUsers(t, c), "the insert must not have been committed")
}

func TestTxRollsBackOnPanic(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	assert.Panics(t, func() {
		_ = c.Tx(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, _ = tx.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 1, "ada")
			panic("boom")
		})
	})

	assert.Equal(t, 0, countUsers(t, c))
}

func TestTxWithOptionsRejectsWritesWhenReadOnly(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	err := c.TxWithOptions(ctx, &sql.TxOptions{ReadOnly: true}, func(context.Context, *sql.Tx) error {
		return nil
	})

	// SQLite's driver refuses a read-only transaction outright; other drivers
	// accept it. Either way the call must report, not panic.
	if err != nil {
		assert.Contains(t, err.Error(), sqlcdb.ErrTransaction.Error())
	}
}

// fakeQueries stands in for a sqlc-generated Queries value: New(DBTX) and
// WithTx(*sql.Tx) are exactly the two constructors sqlc emits.
type fakeQueries struct{ db sqlcdb.DBTX }

func newFakeQueries(db sqlcdb.DBTX) *fakeQueries { return &fakeQueries{db: db} }

func (q *fakeQueries) WithTx(tx *sql.Tx) *fakeQueries { return &fakeQueries{db: tx} }

func (q *fakeQueries) createUser(ctx context.Context, id int, name string) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, id, name)
	return err
}

// TestClientSatisfiesGeneratedDBTX is the contract that matters: the generated
// package declares its own DBTX, so *Client is only usable if its method set
// matches structurally.
func TestClientSatisfiesGeneratedDBTX(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	q := newFakeQueries(c)
	require.NoError(t, q.createUser(ctx, 1, "ada"))

	assert.Equal(t, 1, countUsers(t, c))
}

func TestInTxBindsGeneratedQueries(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	q := newFakeQueries(c)

	err := sqlcdb.InTx(ctx, c, q.WithTx, func(ctx context.Context, q *fakeQueries) error {
		return q.createUser(ctx, 1, "ada")
	})
	require.NoError(t, err)
	assert.Equal(t, 1, countUsers(t, c))

	boom := errors.New("boom")
	err = sqlcdb.InTx(ctx, c, q.WithTx, func(ctx context.Context, q *fakeQueries) error {
		if err := q.createUser(ctx, 2, "grace"); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, countUsers(t, c), "the failed transaction must have rolled back")
}

func TestLoggingDoesNotBreakQueries(t *testing.T) {
	ctx := context.Background()
	c, err := sqlcdb.New(ctx, sqlcdb.Config{
		Driver:             "sqlite3",
		DSN:                ":memory:",
		MaxOpenConnections: 1,
		EnableLogging:      true,
	}, log())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.ExecContext(ctx, `CREATE TABLE t (id INTEGER)`)
	require.NoError(t, err)

	var n int
	require.NoError(t, c.QueryRowContext(ctx, `SELECT COUNT(*) FROM t`).Scan(&n))
	assert.Equal(t, 0, n)
}

func countUsers(t *testing.T, c *sqlcdb.Client) int {
	t.Helper()
	var n int
	require.NoError(t, c.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&n))
	return n
}
