# go-engine/database/mongodb

MongoDB client for go-engine built on the official [mongo-driver](https://github.com/mongodb/mongo-go-driver).

```bash
go get github.com/skolldire/go-engine/database/mongodb
```

---

## Configuration

```yaml
mongodb_clients:
  - analytics:
      uri: "mongodb://localhost:27017"
      database: "analytics"
      timeout: 30s
      max_pool_size: 100
      min_pool_size: 5
      enable_logging: true
  - audit:
      uri: "mongodb+srv://user:pass@cluster.mongodb.net"
      database: "audit"
```

Resilience:

```yaml
mongodb_clients:
  - analytics:
      with_resilience: true
      resilience:
        retry_config:
          max_retries: 3
          initial_delay: 200ms
```

---

## Usage

`mongodb.Service` exposes `GetDatabase()` and `GetCollection()` for direct access to the mongo-driver primitives. This gives full query flexibility without wrapper overhead.

```go
mdb := engine.GetMongoDBClientByName("analytics")

// Get a collection
col := mdb.GetCollection("exam_results")

// Insert
_, err := col.InsertOne(ctx, bson.M{
    "exam_id":    "e-001",
    "student_id": "s-042",
    "score":      9.2,
    "at":         time.Now(),
})

// Find one
var result bson.M
err = col.FindOne(ctx, bson.M{"exam_id": "e-001", "student_id": "s-042"}).Decode(&result)

// Find many
cursor, err := col.Find(ctx, bson.M{"exam_id": "e-001"})
defer cursor.Close(ctx)
var results []bson.M
if err := cursor.All(ctx, &results); err != nil { ... }

// Update
_, err = col.UpdateOne(ctx,
    bson.M{"_id": docID},
    bson.M{"$set": bson.M{"score": 9.5}},
)

// Delete
_, err = col.DeleteOne(ctx, bson.M{"_id": docID})

// Aggregation
pipeline := mongo.Pipeline{
    {{"$match", bson.M{"exam_id": "e-001"}}},
    {{"$group", bson.M{"_id": "$student_id", "avg": bson.M{"$avg": "$score"}}}},
}
cursor, err = col.Aggregate(ctx, pipeline)

// Health check
err = mdb.Ping(ctx)

// Shutdown
defer mdb.Disconnect(ctx)
```

**Errors:** `mongodb.ErrConnection`, `mongodb.ErrNotFound`, `mongodb.ErrInvalidInput`.
