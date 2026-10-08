# go-engine/database/sqlc

`database/sql` client for [sqlc](https://sqlc.dev)-generated code: a pool with
the framework's logging, timeouts, resilience and health, and no ORM under it.

```bash
go get github.com/skolldire/go-engine/database/sqlc
```

It is the light half of the SQL story. `database/sql` wraps GORM, which is the
right tool when the schema is large and relations are mapped; this module exists
for the projects where that machinery costs more than it returns — the queries
are known, they are written in SQL, and `sqlc` compiles them into typed Go.

The two are separate modules on purpose: every external module this one
resolves is one the core already resolves — there is no driver, no ORM and no
SQL library underneath, only the standard library. `make lint-deps` prints the
figure. A service that chose sqlc to avoid an ORM would otherwise link one
anyway, through the module graph.

---

## Choosing it

The two SQL providers read different configuration sections and register under
different component names, so the application picks one when it assembles the
engine — or registers both and keeps GORM for the tables it already maps.

| | `database/sql` (GORM) | `database/sqlc` |
|---|---|---|
| Config section | `sql_clients` | `sqlc_clients` |
| Component name | `sql:<instance>` | `sqlc:<instance>` |
| Driver | a `gorm.Dialector` passed in Go | a driver name in the YAML + a blank import |
| Runtime dependencies | GORM + one dialect | none beyond the core |

---

## Wiring

```go
import (
    _ "github.com/jackc/pgx/v5/stdlib"   // registers "pgx"

    sqlcprovider "github.com/skolldire/go-engine/database/sqlc/provider/sqlc"
    "github.com/skolldire/go-engine/pkg/engine"

    "myservice/internal/db"              // the sqlc-generated package
)

eng, err := engine.New(ctx,
    engine.WithRouter(),
    engine.WithHealth(),
    engine.WithProvider(sqlcprovider.New("main")),
)
if err != nil {
    return err
}

client, err := sqlcprovider.From(eng, "main")
if err != nil {
    return err
}

queries := db.New(client)   // *sqlcdb.Client satisfies the generated DBTX
```

The blank import is the whole driver story: a `database/sql` driver registers
itself by name, so the choice travels in the YAML and nothing is linked that the
application did not import. Forgetting it is reported by name rather than as an
"unknown driver" from deep inside the standard library.

The engine owns the pool: it is closed in LIFO order on shutdown and contributes
a health check, so a database outage shows on `/ready` without the application
wiring anything.

---

## Configuration

```yaml
sqlc_clients:
  - main:
      driver: "pgx"
      dsn: "${DATABASE_URL}"
      max_idle_connections: 10
      max_open_connections: 100
      conn_max_lifetime: 5m
      conn_max_idle_time: 1m
      timeout: 30s
      enable_logging: false
      with_resilience: false
```

| Key | Default | Meaning |
|---|---|---|
| `driver` | — | registered driver name (`pgx`, `mysql`, `sqlite3`, …) |
| `dsn` | — | connection string; `${VAR}` is resolved from the environment |
| `max_idle_connections` | 10 | `SetMaxIdleConns` |
| `max_open_connections` | 100 | `SetMaxOpenConns` |
| `conn_max_lifetime` | 5m | `SetConnMaxLifetime` |
| `conn_max_idle_time` | unset | `SetConnMaxIdleTime` |
| `timeout` | 30s | per-operation timeout, applied when the caller's context has no deadline |
| `enable_logging` | false | logs every statement with its duration |
| `with_resilience` | false | retry + circuit breaker; see the warning below |

Durations are strings (`5m`, `30s`). A bare number is rejected at startup.

When the credentials cannot live in a connection string — an IAM token, a secret
rotated at runtime — pass a `driver.Connector` instead and leave `dsn` out:

```go
sqlcprovider.New("main", sqlcprovider.WithConnector(connector))
```

---

## Transactions

`InTx` binds the generated `Queries` to a transaction and commits, rolls back on
error, and rolls back on panic. `queries.WithTx` is already the shape it wants:

```go
err := sqlcdb.InTx(ctx, client, queries.WithTx,
    func(ctx context.Context, q *db.Queries) error {
        if err := q.Debit(ctx, from); err != nil {
            return err
        }
        return q.Credit(ctx, to)
    })
```

For SQL the generated code does not cover, `Tx` hands over the raw handle:

```go
err := client.Tx(ctx, func(ctx context.Context, tx *sql.Tx) error {
    _, err := tx.ExecContext(ctx, `REFRESH MATERIALIZED VIEW report`)
    return err
})
```

`TxWithOptions` takes a `*sql.TxOptions` for an isolation level or a read-only
transaction.

---

## API

| Method | Description |
|---|---|
| `ExecContext(ctx, query, args...)` | statement; goes through the resilience layer |
| `QueryContext(ctx, query, args...)` | rows |
| `QueryRowContext(ctx, query, args...)` | at most one row |
| `PrepareContext(ctx, query)` | prepared statement |
| `Tx(ctx, fn)` / `TxWithOptions(ctx, opts, fn)` | transaction with rollback on error or panic |
| `InTx(ctx, client, bind, fn)` | the same, bound to a generated `Queries` |
| `Ping(ctx)` | connectivity check; what the health check calls |
| `DB()` | the underlying `*sql.DB` |
| `Stats()` | pool counters, for metrics or a saturation alert |
| `Driver()` | the driver name the pool was opened with |
| `Close()` | closes the pool |

`sqlcdb.IsNotFound(err)` reports the empty-result case; `sqlcdb.ErrNotFound` is
`sql.ErrNoRows` itself, so `errors.Is` works directly against what the generated
queries return.

### Two details worth knowing

**Queries are observed, not wrapped.** `ExecContext` and `PrepareContext` run
through `BaseClient.Execute`, so they get the timeout and the resilience layer.
`QueryContext` and `QueryRowContext` do not: `Execute` cancels the context it
derives as soon as it returns, and `database/sql` keeps `*sql.Rows` bound to the
query's context until they are closed — wrapping them would fail the caller's
first `Next()` with "context canceled". They run on the caller's context and are
logged with their duration instead.

**`with_resilience` retries.** A retried `INSERT` cannot know the first one
landed, and a retried transaction runs `fn` again from the top. Enable it only
where the statements are idempotent.

---

## Generating the code

This module does not run `sqlc`; it is the runtime half. The usual layout:

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/query.sql"
    schema: "internal/db/schema.sql"
    gen:
      go:
        package: "db"
        out: "internal/db"
```

```bash
sqlc generate
```

The generated `New(db DBTX)` takes the interface this package's `Client`
satisfies, so nothing sits between the two.

---

## Hexagonal architecture pattern

The generated `Queries` is infrastructure, like `*gorm.DB`. The domain declares
what it needs and the adapter implements it with the generated calls:

```go
// internal/usecase/orders.go — no import of db, no import of database/sql
type OrderRepository interface {
    ByID(ctx context.Context, id string) (Order, error)
    Save(ctx context.Context, o Order) error
}

type OrderUseCase struct {
    repo OrderRepository
}
```

```go
// internal/repository/order_postgres.go
type orderRepository struct {
    q *db.Queries
}

func NewOrderRepository(c *sqlcdb.Client) usecase.OrderRepository {
    return &orderRepository{q: db.New(c)}
}

func (r *orderRepository) ByID(ctx context.Context, id string) (usecase.Order, error) {
    row, err := r.q.GetOrder(ctx, id)
    if sqlcdb.IsNotFound(err) {
        return usecase.Order{}, usecase.ErrOrderNotFound
    }
    if err != nil {
        return usecase.Order{}, err
    }
    return toDomain(row), nil
}
```
