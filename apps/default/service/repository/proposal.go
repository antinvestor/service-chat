package repository

import (
	"context"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

type proposalRepository struct {
	datastore.BaseRepository[*models.Proposal]
}

// NewProposalRepository creates a new proposal repository instance.
func NewProposalRepository(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
) ProposalRepository {
	return &proposalRepository{
		BaseRepository: datastore.NewBaseRepository[*models.Proposal](
			ctx, dbPool, workMan, func() *models.Proposal { return &models.Proposal{} },
		),
	}
}

// GetPendingByScope retrieves all pending proposals for a given scope.
func (r *proposalRepository) GetPendingByScope(
	ctx context.Context,
	scopeType string,
	scopeID string,
) ([]*models.Proposal, error) {
	var proposals []*models.Proposal
	err := r.Pool().DB(ctx, true).
		Where("scope_type = ? AND scope_id = ? AND state = ? AND expires_at > ?",
			scopeType, scopeID, models.ProposalStatePending, time.Now()).
		Order("id ASC").
		Find(&proposals).Error
	return proposals, err
}

// UpdateState updates the state of a proposal.
func (r *proposalRepository) UpdateState(
	ctx context.Context,
	id string,
	state models.ProposalState,
	resolvedBy string,
	reason string,
) error {
	now := time.Now()
	return r.Pool().DB(ctx, false).
		Model(&models.Proposal{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"state":       state,
			"resolved_by": resolvedBy,
			"resolved_at": &now,
			"reason":      reason,
		}).Error
}

// ExpirePending marks all expired pending proposals as expired.
func (r *proposalRepository) ExpirePending(ctx context.Context) (int64, error) {
	result := r.Pool().DB(ctx, false).
		Model(&models.Proposal{}).
		Where("state = ? AND expires_at <= ?", models.ProposalStatePending, time.Now()).
		Update("state", models.ProposalStateExpired)
	return result.RowsAffected, result.Error
}
