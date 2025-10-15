# Status Handler Guide

## Overview

Status handlers process CloudEvents from Leaf Hubs. Two types:

1. **Database Handlers**: Define ORM models, persist to database
2. **Logic Handlers**: Business logic only, no database

---

## Handler Structure

```go
type xxxHandler struct {
    eventType     string
    eventSyncMode enum.EventSyncMode          // CompleteStateMode/DeltaStateMode/HybridStateMode
    eventPriority conflator.ConflationPriority
}

func RegisterXxxHandler(conflationManager *conflator.ConflationManager) {
    handler := &xxxHandler{
        eventType:     string(enum.XxxType),
        eventSyncMode: enum.CompleteStateMode,
        eventPriority: conflator.XxxPriority,
    }
    conflationManager.Register(conflator.NewConflationRegistration(
        handler.eventPriority,
        handler.eventSyncMode,
        handler.eventType,
        handler.handleEvent,
    ))
}
```

---

## Database Handler

### SQL → ORM Mapping

**SQL** (`operator/pkg/controllers/storage/database/2.tables.sql`):
```sql
CREATE TABLE status.leaf_hubs (
    leaf_hub_name character varying(254) NOT NULL,
    cluster_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    PRIMARY KEY (cluster_id, leaf_hub_name)
);
```

**ORM** (`pkg/database/models/status.go`):
```go
type LeafHub struct {
    LeafHubName string         `gorm:"column:leaf_hub_name;primaryKey"`
    ClusterID   string         `gorm:"column:cluster_id;primaryKey"`
    Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
    CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime:true"`
    UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
    DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (LeafHub) TableName() string {
    return "status.leaf_hubs"
}
```

### Handler Implementation

**Upsert**:
```go
func (h *handler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
    data := &Data{}
    evt.DataAs(data)
    payload, _ := json.Marshal(data)

    db := database.GetGorm()
    return db.Clauses(clause.OnConflict{
        Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "leaf_hub_name"}},
        DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
    }).Create(&models.LeafHub{
        LeafHubName: evt.Source(),
        ClusterID:   data.ClusterID,
        Payload:     payload,
    }).Error
}
```

**Batch Upsert**:
```go
func (h *handler) insertOrUpdate(objs []Object, leafHubName string) error {
    batch := []models.Resource{}
    for _, obj := range objs {
        payload, _ := json.Marshal(obj)
        batch = append(batch, models.Resource{
            ID:          obj.UID,
            LeafHubName: leafHubName,
            Payload:     payload,
        })
    }

    db := database.GetGorm()
    return db.Clauses(clause.OnConflict{
        Columns:   []clause.Column{{Name: "id"}},
        DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
    }).CreateInBatches(batch, 50).Error
}
```

**Delete** (soft delete):
```go
db.Where("cluster_id = ?", id).Delete(&models.Resource{})
```

---

## Logic Handler

**No ORM model, no database**. Example: Migration handler

```go
type migrationHandler struct {
    eventType     string
    eventSyncMode enum.EventSyncMode  // DeltaStateMode
    eventPriority conflator.ConflationPriority
}

func (h *migrationHandler) handle(ctx context.Context, evt *cloudevents.Event) error {
    bundle := &MigrationBundle{}
    evt.DataAs(bundle)

    hubName := evt.Source()

    if bundle.Resync {
        migration.ResetMigrationStatus(hubName)
        return nil
    }

    if len(bundle.ManagedClusters) > 0 {
        migration.SetClusterList(bundle.MigrationId, bundle.ManagedClusters)
    }

    if bundle.ErrMessage != "" {
        migration.SetErrorMessage(bundle.MigrationId, hubName, bundle.Stage, bundle.ErrMessage)
    } else {
        migration.SetFinished(bundle.MigrationId, hubName, bundle.Stage)
    }

    return nil
}
```

---

## Best Practices

### ORM Model
```go
// ✅ Good
type Resource struct {
    ID          string         `gorm:"column:id;primaryKey"`
    LeafHubName string         `gorm:"column:leaf_hub_name;primaryKey"`
    Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
    CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime:true"`
    UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
    DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

// ❌ Bad
type Resource struct {
    ID   string `gorm:"column:id"` // Missing primaryKey
    Name string `gorm:"not null"`  // Missing column name
}
```

### Upsert
```go
// ✅ Specify columns to update
DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"})

// ❌ Updates all fields
UpdateAll: true
```

### Batch Operations
```go
// ✅ Batch insert
db.CreateInBatches(records, 50)

// ❌ Single-record loop
for _, r := range records { db.Create(&r) }
```

---

## FAQ

**Composite primary key?**
```go
type Model struct {
    Key1 string `gorm:"column:key1;primaryKey"`
    Key2 string `gorm:"column:key2;primaryKey"`
}
```

**OnConflict Columns?**
Must match database `PRIMARY KEY` exactly:
```go
Columns: []clause.Column{{Name: "key1"}, {Name: "key2"}}
```

**UpdateAll vs DoUpdates?**
- `UpdateAll: true`: Updates all fields (including `created_at`)
- `DoUpdates`: Updates only specified fields ✅ Recommended

---

## Comparison

| Feature | Database Handler | Logic Handler |
|---------|-----------------|---------------|
| **ORM Model** | ✅ Required | ❌ Not needed |
| **Database** | ✅ GORM CRUD | ❌ No database |
| **Use Case** | Persistence, queries | Temporary state |
| **Examples** | ManagedCluster, LeafHub | Migration |
