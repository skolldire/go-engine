# go-engine/database/sql

GORM wrapper (`gormsql.DBClient`) for go-engine. Exposes a method-level API that hides GORM from the domain layer.

```bash
go get github.com/skolldire/go-engine
```

**Important:** `DBClient` is NOT auto-initialized by the engine. Build the GORM connection yourself and inject it via `WithCustomClient`.

---

## Configuration

```go
import (
    gormsql "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

dialector := postgres.Open("host=localhost user=app password=secret dbname=mydb port=5432 sslmode=disable")

client, err := gormsql.New(gormsql.Config{
    Type:               "postgres",
    MaxIdleConnections: 10,
    MaxOpenConnections: 100,
    ConnMaxLifetime:    5 * time.Minute,
    EnableLogging:      true,
    LogLevel:           "warn",
    TablePrefix:        "app_",
    AutoMigrate:        false,
    WithResilience:     false,
}, dialector, log)

engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithCustomClient("main-db", client).
    WithRouter().
    Build()

// Retrieve:
raw := engine.GetCustomClient("main-db")
db, _ := client.SafeTypeAssert[*gormsql.DBClient](raw)
```

---

## Available methods

| Method | Description |
|---|---|
| `Create(ctx, value)` | INSERT |
| `First(ctx, dest, conditions...)` | SELECT first match; returns `ErrNotFound` if missing |
| `Find(ctx, dest, conditions...)` | SELECT all matches |
| `Update(ctx, model, updates)` | UPDATE fields |
| `Delete(ctx, value, conditions...)` | DELETE |
| `Count(ctx, model, &count, conditions...)` | COUNT |
| `Where(ctx, dest, query, args...)` | WHERE + Find |
| `Order(ctx, dest, value)` | ORDER BY + Find |
| `Limit(ctx, dest, n)` | LIMIT + Find |
| `Offset(ctx, dest, n)` | OFFSET + Find |
| `Preload(ctx, dest, relation, conditions...)` | eager-load association |
| `Upsert(ctx, value, conflictCols, updateCols)` | INSERT ON CONFLICT DO UPDATE |
| `Raw(ctx, dest, sql, values...)` | raw SELECT |
| `Exec(ctx, sql, values...)` | raw DML |
| `Transaction(ctx, fn func(*gorm.DB) error)` | wraps fn in a transaction |
| `AutoMigrate(models...)` | runs GORM AutoMigrate |
| `Ping(ctx)` | connectivity check |
| `DB()` | returns raw `*gorm.DB` for complex queries |
| `WithContext(ctx)` | returns `*gorm.DB` scoped to ctx |
| `Close()` | closes the underlying connection pool |

**Errors:** `gormsql.ErrNotFound`, `gormsql.ErrConnection`, `gormsql.ErrTransaction`.

---

## Hexagonal architecture pattern

### ❌ Anti-pattern — GORM in the domain

```go
// assessment-service/internal/usecase/scoring.go
import "gorm.io/gorm"              // VIOLATION: domain depends on infrastructure

type ScoringUseCase struct {
    db *gorm.DB                    // VIOLATION
}

func (u *ScoringUseCase) GetItem(id string) (*Item, error) {
    var item Item
    u.db.First(&item, "id = ?", id) // VIOLATION: GORM in business logic
    return &item, nil
}
```

### ✅ Correct pattern

**Domain port (no GORM, no go-engine):**

```go
// internal/domain/port/output/item_repository.go
type ItemRepository interface {
    FindByID(ctx context.Context, id string) (*Item, error)
    FindByExamID(ctx context.Context, examID string) ([]Item, error)
}
```

**Use case depends on the interface:**

```go
// internal/usecase/scoring.go
type ScoringUseCase struct {
    items output.ItemRepository    // interface — testable with any mock
}

func (u *ScoringUseCase) GetItem(ctx context.Context, id string) (*Item, error) {
    return u.items.FindByID(ctx, id)
}
```

**Adapter (GORM confined here):**

```go
// internal/adapter/output/postgres/item_repository.go
import gormsql "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"

type itemModel struct {
    ID         string `gorm:"primaryKey"`
    ExamID     string
    Difficulty float64
}

type postgresItemRepository struct{ db *gormsql.DBClient }

func NewItemRepository(db *gormsql.DBClient) output.ItemRepository {
    return &postgresItemRepository{db: db}
}

func (r *postgresItemRepository) FindByID(ctx context.Context, id string) (*Item, error) {
    var m itemModel
    if err := r.db.First(ctx, &m, "id = ?", id); err != nil {
        if errors.Is(err, gormsql.ErrNotFound) {
            return nil, ErrItemNotFound  // domain error
        }
        return nil, err
    }
    return toDomain(&m), nil
}
```

**Enforcement:** `make lint-arch` fails if `gorm.io/gorm` is imported in `pkg/`.

---

## Multi-entity transactions

```go
func (r *orderRepo) PlaceOrder(ctx context.Context, order *Order) error {
    return r.db.Transaction(ctx, func(tx *gorm.DB) error {
        if err := tx.Create(toOrderModel(order)).Error; err != nil {
            return err
        }
        return tx.Model(&inventoryModel{}).
            Where("id = ?", order.ItemID).
            Update("stock", gorm.Expr("stock - 1")).Error
    })
}
```

The `func(tx *gorm.DB)` callback is intentional — multi-entity transactions must stay in the adapter layer.
