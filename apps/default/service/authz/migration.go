package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/util"
)

// defaultMigrationBatchSize is the default number of subscriptions to process per batch.
const defaultMigrationBatchSize = 1000

// SubscriptionFetcher is an interface for fetching subscriptions during migration.
type SubscriptionFetcher interface {
	GetAllActive(ctx context.Context) ([]*models.RoomSubscription, error)
}

// MigrationConfig holds configuration for the migration process.
type MigrationConfig struct {
	// BatchSize is the number of subscriptions to process in each batch.
	BatchSize int
	// DryRun if true, only logs what would be done without making changes.
	DryRun bool
	// ContinueOnError if true, continues migration even if some tuples fail.
	ContinueOnError bool
}

// DefaultMigrationConfig returns a MigrationConfig with sensible defaults.
func DefaultMigrationConfig() MigrationConfig {
	return MigrationConfig{
		BatchSize:       defaultMigrationBatchSize,
		DryRun:          false,
		ContinueOnError: true,
	}
}

// MigrationResult holds the results of a migration run.
type MigrationResult struct {
	TotalProcessed int
	Successful     int
	Failed         int
	Errors         []MigrationError
}

// MigrationError represents a single migration error.
type MigrationError struct {
	RoomID    string
	ProfileID string
	Role      string
	Error     error
}

// MigrateSubscriptionsToKeto migrates all active subscriptions to Keto authorization tuples.
// This is used to sync the existing subscription data with the new authorization system.
func MigrateSubscriptionsToKeto(
	ctx context.Context,
	subFetcher SubscriptionFetcher,
	authzService security.Authorizer,
	cfg MigrationConfig,
) (*MigrationResult, error) {
	if subFetcher == nil {
		return nil, errors.New("subscription fetcher is required")
	}
	if authzService == nil {
		return nil, errors.New("authz service is required")
	}

	log := util.Log(ctx)
	log.Info("starting subscription to Keto migration")

	subs, err := subFetcher.GetAllActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subscriptions: %w", err)
	}

	result := &MigrationResult{
		TotalProcessed: len(subs),
		Errors:         make([]MigrationError, 0),
	}

	log.WithField("total_subscriptions", len(subs)).Info("fetched subscriptions for migration")

	for i := 0; i < len(subs); i += cfg.BatchSize {
		end := min(i+cfg.BatchSize, len(subs))
		tuples := buildTuplesFromBatch(subs[i:end])

		if cfg.DryRun {
			log.WithField("batch_size", len(tuples)).
				WithField("batch_start", i).
				Info("dry run: would write tuples")
			result.Successful += len(tuples)
			continue
		}

		if batchErr := writeBatchWithRetry(ctx, authzService, tuples, cfg, result); batchErr != nil {
			return result, fmt.Errorf("failed to write tuples batch at %d: %w", i, batchErr)
		}

		log.WithField("batch_processed", end).
			WithField("successful", result.Successful).
			WithField("failed", result.Failed).
			Debug("processed migration batch")
	}

	log.WithField("total_processed", result.TotalProcessed).
		WithField("successful", result.Successful).
		WithField("failed", result.Failed).
		Info("completed subscription to Keto migration")

	return result, nil
}

// buildTuplesFromBatch converts a batch of subscriptions into relation tuples.
func buildTuplesFromBatch(batch []*models.RoomSubscription) []security.RelationTuple {
	tuples := make([]security.RelationTuple, 0, len(batch))
	for _, sub := range batch {
		if sub.ProfileID == "" || sub.RoomID == "" {
			continue
		}

		role := sub.Role
		if role == "" {
			role = RoleMember
		}

		tuples = append(tuples, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: sub.RoomID},
			Relation: RoleToRelation(role),
			Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: sub.ProfileID},
		})
	}
	return tuples
}

// writeBatchWithRetry writes tuples as a batch and falls back to individual writes on failure.
// Returns nil if ContinueOnError is set and individual retries are attempted.
func writeBatchWithRetry(
	ctx context.Context,
	authzService security.Authorizer,
	tuples []security.RelationTuple,
	cfg MigrationConfig,
	result *MigrationResult,
) error {
	writeErr := authzService.WriteTuples(ctx, tuples)
	if writeErr == nil {
		result.Successful += len(tuples)
		return nil
	}

	util.Log(ctx).WithError(writeErr).
		WithField("batch_size", len(tuples)).
		Warn("failed to write batch of tuples")

	if !cfg.ContinueOnError {
		return writeErr
	}

	// Retry individual writes
	for _, tuple := range tuples {
		if individualErr := authzService.WriteTuple(ctx, tuple); individualErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, MigrationError{
				RoomID:    tuple.Object.ID,
				ProfileID: tuple.Subject.ID,
				Role:      RelationToRole(tuple.Relation),
				Error:     individualErr,
			})
		} else {
			result.Successful++
		}
	}
	return nil
}

// SyncRoomSubscriptions syncs all subscriptions for a specific room to Keto.
// This can be used for incremental sync or to repair a single room's permissions.
func SyncRoomSubscriptions(
	ctx context.Context,
	roomID string,
	subscriptions []*models.RoomSubscription,
	authzService security.Authorizer,
) error {
	if authzService == nil {
		return nil // No-op if authz service not configured
	}

	tuples := make([]security.RelationTuple, 0, len(subscriptions))

	for _, sub := range subscriptions {
		if !sub.IsActive() || sub.ProfileID == "" {
			continue
		}

		role := sub.Role
		if role == "" {
			role = RoleMember
		}

		tuples = append(tuples, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Relation: RoleToRelation(role),
			Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: sub.ProfileID},
		})
	}

	if len(tuples) == 0 {
		return nil
	}

	return authzService.WriteTuples(ctx, tuples)
}
