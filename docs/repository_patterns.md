# Repository Pattern with Generics - Best Practices

## Overview

This document explains how to use the generic `BaseRepository` to reduce code duplication while maintaining flexibility for entity-specific operations.

## BaseRepository Interface

```go
type BaseRepository[T any] interface {
    GetByID(ctx context.Context, id string) (T, error)
    Save(ctx context.Context, entity T) error
    Delete(ctx context.Context, id string) error
    FindAll(ctx context.Context, limit int) ([]T, error)
    Count(ctx context.Context) (int64, error)
}
```

## Usage Patterns

### Pattern 1: Composition (Recommended)

Embed `BaseRepository` in your specific repository struct:

```go
type roomRepository struct {
    BaseRepository[*models.Room]
    service *frame.Service
}

func NewRoomRepository(service *frame.Service) RoomRepository {
    baseRepo := NewBaseRepository[*models.Room](
        service,
        func() *models.Room { return &models.Room{} },
    )
    
    return &roomRepository{
        BaseRepository: baseRepo,
        service:        service,
    }
}

// Custom methods
func (rr *roomRepository) GetByTenantAndType(ctx context.Context, tenantID, roomType string) ([]*models.Room, error) {
    var rooms []*models.Room
    err := rr.service.DB(ctx, true).
        Where("tenant_id = ? AND room_type = ?", tenantID, roomType).
        Find(&rooms).Error
    return rooms, err
}
```

**Benefits:**
- ✅ No need to reimplement CRUD methods
- ✅ Consistent behavior across all repositories
- ✅ Easy to add custom methods
- ✅ Type-safe with generics

### Pattern 2: Explicit Implementation (Current)

Implement all methods explicitly in each repository:

```go
type roomRepository struct {
    service *frame.Service
}

func (rr *roomRepository) GetByID(ctx context.Context, id string) (*models.Room, error) {
    room := &models.Room{}
    err := rr.service.DB(ctx, true).First(room, "id = ?", id).Error
    return room, err
}

func (rr *roomRepository) Save(ctx context.Context, room *models.Room) error {
    return rr.service.DB(ctx, false).Save(room).Error
}

// ... more methods
```

**Benefits:**
- ✅ Full control over each method
- ✅ Easy to customize per entity
- ❌ Code duplication
- ❌ More maintenance

### Pattern 3: Hybrid Approach

Use base repository internally but expose custom interface:

```go
type roomRepository struct {
    base    BaseRepository[*models.Room]
    service *frame.Service
}

func NewRoomRepository(service *frame.Service) RoomRepository {
    return &roomRepository{
        base:    NewBaseRepository[*models.Room](service, func() *models.Room { return &models.Room{} }),
        service: service,
    }
}

// Delegate to base for standard operations
func (rr *roomRepository) GetByID(ctx context.Context, id string) (*models.Room, error) {
    return rr.base.GetByID(ctx, id)
}

func (rr *roomRepository) Save(ctx context.Context, room *models.Room) error {
    return rr.base.Save(ctx, room)
}

// Custom method
func (rr *roomRepository) GetByTenantAndType(ctx context.Context, tenantID, roomType string) ([]*models.Room, error) {
    // Custom implementation
}
```

**Benefits:**
- ✅ Explicit interface implementation
- ✅ Reuse base functionality
- ✅ Easy to override specific methods
- ❌ Some boilerplate for delegation

## Refactoring Guide

### Step 1: Update Interface (Optional)

If you want to enforce base methods, update your interface:

```go
type RoomRepository interface {
    BaseRepository[*models.Room]
    
    // Custom methods
    Search(ctx context.Context, query *framedata.SearchQuery) ([]*models.Room, error)
    GetByTenantAndType(ctx context.Context, tenantID, roomType string) ([]*models.Room, error)
    GetRoomsByProfileID(ctx context.Context, profileID string) ([]*models.Room, error)
}
```

### Step 2: Refactor Implementation

```go
type roomRepository struct {
    BaseRepository[*models.Room]
    service *frame.Service
}

func NewRoomRepository(service *frame.Service) RoomRepository {
    baseRepo := NewBaseRepository[*models.Room](
        service,
        func() *models.Room { return &models.Room{} },
    )
    
    return &roomRepository{
        BaseRepository: baseRepo,
        service:        service,
    }
}

// Only implement custom methods - CRUD is inherited!
func (rr *roomRepository) Search(ctx context.Context, query *framedata.SearchQuery) ([]*models.Room, error) {
    // Implementation
}
```

### Step 3: Update Tests

Tests remain the same since the interface hasn't changed:

```go
func TestRoomRepository_GetByID(t *testing.T) {
    // Same test code works with both implementations
    repo := NewRoomRepository(service)
    room, err := repo.GetByID(ctx, "room-id")
    // assertions...
}
```

## Advanced: Custom Base Repository

You can create specialized base repositories for different needs:

```go
// BaseRepositoryWithTenant adds tenant-aware operations
type BaseRepositoryWithTenant[T any] interface {
    BaseRepository[T]
    GetByTenantID(ctx context.Context, tenantID string, limit int) ([]T, error)
    CountByTenantID(ctx context.Context, tenantID string) (int64, error)
}

// BaseRepositoryWithSoftDelete adds soft delete operations
type BaseRepositoryWithSoftDelete[T any] interface {
    BaseRepository[T]
    SoftDelete(ctx context.Context, id string) error
    Restore(ctx context.Context, id string) error
    FindDeleted(ctx context.Context, limit int) ([]T, error)
}
```

## Migration Strategy

### Option A: Gradual Migration (Recommended)

1. Keep existing repositories as-is
2. Create new repositories using BaseRepository
3. Migrate one repository at a time
4. Run tests after each migration

### Option B: Complete Refactor

1. Update all repository interfaces to embed BaseRepository
2. Refactor all implementations simultaneously
3. Run full test suite
4. Fix any issues

## Testing BaseRepository

```go
func TestBaseRepository_GetByID(t *testing.T) {
    tests := []struct {
        name    string
        id      string
        want    *models.Room
        wantErr bool
    }{
        {
            name: "existing room",
            id:   "room-123",
            want: &models.Room{ID: "room-123", Name: "Test Room"},
            wantErr: false,
        },
        {
            name: "non-existent room",
            id:   "room-999",
            want: nil,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := NewBaseRepository[*models.Room](
                service,
                func() *models.Room { return &models.Room{} },
            )
            
            got, err := repo.GetByID(ctx, tt.id)
            if (err != nil) != tt.wantErr {
                t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("GetByID() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Performance Considerations

1. **Model Factory**: The `modelFactory` function is called for each query. Keep it lightweight.
2. **Type Assertions**: Generics eliminate runtime type assertions, improving performance.
3. **Query Optimization**: Custom methods can still use indexes and optimized queries.

## When NOT to Use BaseRepository

- **Complex queries**: Repositories with many joins, subqueries, or raw SQL
- **Non-standard IDs**: Entities using composite keys or non-string IDs
- **Special deletion logic**: Entities requiring cascade deletes or cleanup
- **Audit requirements**: Entities needing detailed change tracking

In these cases, explicit implementation is clearer and more maintainable.

## Conclusion

**Recommendation**: Use **Pattern 1 (Composition)** for new repositories. It provides:
- Maximum code reuse
- Type safety with generics
- Flexibility for custom methods
- Easy maintenance

For existing repositories, continue with explicit implementation unless refactoring provides clear benefits.
