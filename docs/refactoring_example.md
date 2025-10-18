# Refactoring Example: RoomEventRepository with BaseRepository

## Before: Explicit Implementation (Current)

```go
type roomEventRepository struct {
    service *frame.Service
}

func (rer *roomEventRepository) GetByID(ctx context.Context, id string) (*models.RoomEvent, error) {
    event := &models.RoomEvent{}
    err := rer.service.DB(ctx, true).First(event, "id = ?", id).Error
    return event, err
}

func (rer *roomEventRepository) Save(ctx context.Context, event *models.RoomEvent) error {
    return rer.service.DB(ctx, false).Save(event).Error
}

func (rer *roomEventRepository) Delete(ctx context.Context, id string) error {
    event, err := rer.GetByID(ctx, id)
    if err != nil {
        return err
    }
    return rer.service.DB(ctx, false).Delete(event).Error
}

// Custom methods...
func (rer *roomEventRepository) GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomEvent, error) {
    // Implementation
}

func (rer *roomEventRepository) GetNextSequence(ctx context.Context, roomID string) (int64, error) {
    // Implementation
}
```

**Lines of code**: ~200 lines (including CRUD + custom methods)

## After: Using BaseRepository

```go
type roomEventRepository struct {
    BaseRepository[*models.RoomEvent]
    service *frame.Service
}

func NewRoomEventRepository(service *frame.Service) RoomEventRepository {
    baseRepo := NewBaseRepository[*models.RoomEvent](
        service,
        func() *models.RoomEvent { return &models.RoomEvent{} },
    )
    
    return &roomEventRepository{
        BaseRepository: baseRepo,
        service:        service,
    }
}

// GetByID, Save, Delete are inherited - no need to implement!

// Only implement custom methods
func (rer *roomEventRepository) GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomEvent, error) {
    var events []*models.RoomEvent
    query := rer.service.DB(ctx, true).
        Where("room_id = ?", roomID).
        Order("sequence ASC")

    if limit > 0 {
        query = query.Limit(limit)
    }

    err := query.Find(&events).Error
    return events, err
}

func (rer *roomEventRepository) GetNextSequence(ctx context.Context, roomID string) (int64, error) {
    var maxSequence int64

    err := rer.service.DB(ctx, false).Transaction(func(tx *gorm.DB) error {
        var room models.Room
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            Where("id = ?", roomID).
            First(&room).Error; err != nil {
            return fmt.Errorf("failed to lock room: %w", err)
        }

        result := tx.Model(&models.RoomEvent{}).
            Where("room_id = ?", roomID).
            Select("COALESCE(MAX(sequence), 0)").
            Scan(&maxSequence)

        if result.Error != nil {
            return fmt.Errorf("failed to get max sequence: %w", result.Error)
        }

        maxSequence++
        return nil
    })

    return maxSequence, err
}

// ... other custom methods
```

**Lines of code**: ~150 lines (only custom methods)

## Benefits

### Code Reduction
- **Before**: 200 lines
- **After**: 150 lines
- **Savings**: 25% reduction

### Consistency
All repositories now have identical CRUD behavior:
- Same error handling
- Same transaction management
- Same soft delete behavior

### Maintainability
Changes to CRUD logic only need to be made in one place (BaseRepository).

### Type Safety
Generics ensure compile-time type checking:
```go
// This won't compile - type mismatch
var wrongRepo BaseRepository[*models.Room]
wrongRepo = NewBaseRepository[*models.RoomEvent](service, ...)  // ❌ Compile error
```

## Step-by-Step Refactoring

### Step 1: Update the interface (optional)

```go
type RoomEventRepository interface {
    BaseRepository[*models.RoomEvent]
    
    // Custom methods only
    GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomEvent, error)
    GetHistory(ctx context.Context, roomID string, beforeSequence, afterSequence int64, limit int) ([]*models.RoomEvent, error)
    GetNextSequence(ctx context.Context, roomID string) (int64, error)
    // ... other custom methods
}
```

### Step 2: Update the struct

```go
type roomEventRepository struct {
    BaseRepository[*models.RoomEvent]  // Add this
    service *frame.Service
}
```

### Step 3: Update the constructor

```go
func NewRoomEventRepository(service *frame.Service) RoomEventRepository {
    baseRepo := NewBaseRepository[*models.RoomEvent](
        service,
        func() *models.RoomEvent { return &models.RoomEvent{} },
    )
    
    return &roomEventRepository{
        BaseRepository: baseRepo,
        service:        service,
    }
}
```

### Step 4: Remove CRUD methods

Delete these methods (they're now inherited):
- `GetByID`
- `Save`
- `Delete`

### Step 5: Run tests

```bash
go test ./apps/default/service/repository/... -v
```

All existing tests should pass without modification!

## Advanced: Override Base Methods

If you need custom behavior for a base method, just implement it:

```go
// Override Save to add validation
func (rer *roomEventRepository) Save(ctx context.Context, event *models.RoomEvent) error {
    // Custom validation
    if event.Sequence <= 0 {
        return errors.New("sequence must be positive")
    }
    
    // Call base implementation
    return rer.BaseRepository.Save(ctx, event)
}
```

## Testing Strategy

### Test Base Repository Once

```go
// test_base_repository.go
func TestBaseRepository_CRUD(t *testing.T) {
    // Test with multiple entity types
    testCases := []struct {
        name string
        repo BaseRepository[any]
    }{
        {"Room", NewBaseRepository[*models.Room](...)},
        {"RoomEvent", NewBaseRepository[*models.RoomEvent](...)},
        {"RoomOutbox", NewBaseRepository[*models.RoomOutbox](...)},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Test GetByID, Save, Delete
        })
    }
}
```

### Test Custom Methods in Specific Repositories

```go
// room_event_repository_test.go
func TestRoomEventRepository_GetNextSequence(t *testing.T) {
    // Test the custom sequence generation logic
}

func TestRoomEventRepository_GetByRoomID(t *testing.T) {
    // Test the custom query
}
```

## Migration Checklist

- [ ] Create `base.go` with `BaseRepository` interface and implementation
- [ ] Choose migration strategy (gradual or complete)
- [ ] Update one repository as proof-of-concept
- [ ] Run tests to verify behavior unchanged
- [ ] Document any issues or edge cases
- [ ] Migrate remaining repositories
- [ ] Update documentation
- [ ] Remove deprecated code

## Conclusion

Using `BaseRepository` with generics provides:
- ✅ **25-40% code reduction** for typical repositories
- ✅ **Consistent CRUD behavior** across all entities
- ✅ **Type safety** at compile time
- ✅ **Easy maintenance** - fix once, apply everywhere
- ✅ **Backward compatible** - existing tests work unchanged

**Recommendation**: Refactor repositories one at a time, starting with the simplest (e.g., `RoomCallRepository`) to validate the approach.
