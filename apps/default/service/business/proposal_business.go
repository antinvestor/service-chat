package business

import (
	"context"
	"encoding/json"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/util"
)

// roomProposalManagement is a room-scoped implementation of ProposalManagement.
// It handles room-specific permission checks and dispatches approved proposals
// to the appropriate room business methods.
type roomProposalManagement struct {
	proposalRepo    repository.ProposalRepository
	roomBusiness    RoomBusiness
	subscriptionSvc SubscriptionService
}

// NewRoomProposalManagement creates a new room-scoped ProposalManagement instance.
func NewRoomProposalManagement(
	proposalRepo repository.ProposalRepository,
	roomBusiness RoomBusiness,
	subscriptionSvc SubscriptionService,
) ProposalManagement {
	return &roomProposalManagement{
		proposalRepo:    proposalRepo,
		roomBusiness:    roomBusiness,
		subscriptionSvc: subscriptionSvc,
	}
}

func (rpm *roomProposalManagement) Approve(
	ctx context.Context,
	scopeID string,
	proposalID string,
	approvedBy *commonv1.ContactLink,
) error {
	if err := internal.IsValidContactLink(approvedBy); err != nil {
		return err
	}

	// Only owners can approve proposals
	admin, err := rpm.subscriptionSvc.HasRole(ctx, approvedBy, scopeID, roleOwnerLevel)
	if err != nil || admin == nil {
		return service.ErrProposalApprovalDenied
	}

	// Get the proposal
	proposal, err := rpm.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return service.ErrProposalNotFound
	}

	if proposal.ScopeID != scopeID {
		return service.ErrProposalNotFound
	}

	if !proposal.IsPending() {
		return service.ErrProposalNotPending
	}

	if proposal.IsExpired() {
		_ = rpm.proposalRepo.UpdateState(ctx, proposalID,
			models.ProposalStateExpired, approvedBy.GetProfileId(), "expired at approval time")
		return service.ErrProposalExpired
	}

	// Execute the proposed change
	if execErr := rpm.executeProposal(ctx, proposal, approvedBy); execErr != nil {
		util.Log(ctx).WithError(execErr).
			WithField("proposal_id", proposalID).
			WithField("scope_id", scopeID).
			Error("failed to execute approved proposal")
		return fmt.Errorf("failed to execute proposal: %w", execErr)
	}

	// Mark as approved
	return rpm.proposalRepo.UpdateState(ctx, proposalID,
		models.ProposalStateApproved, approvedBy.GetProfileId(), "")
}

func (rpm *roomProposalManagement) Reject(
	ctx context.Context,
	scopeID string,
	proposalID string,
	reason string,
	rejectedBy *commonv1.ContactLink,
) error {
	if err := internal.IsValidContactLink(rejectedBy); err != nil {
		return err
	}

	// Only owners can reject proposals
	admin, err := rpm.subscriptionSvc.HasRole(ctx, rejectedBy, scopeID, roleOwnerLevel)
	if err != nil || admin == nil {
		return service.ErrProposalApprovalDenied
	}

	// Get the proposal
	proposal, err := rpm.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return service.ErrProposalNotFound
	}

	if proposal.ScopeID != scopeID {
		return service.ErrProposalNotFound
	}

	if !proposal.IsPending() {
		return service.ErrProposalNotPending
	}

	// Mark as rejected
	return rpm.proposalRepo.UpdateState(ctx, proposalID,
		models.ProposalStateRejected, rejectedBy.GetProfileId(), reason)
}

func (rpm *roomProposalManagement) ListPending(
	ctx context.Context,
	scopeID string,
	searchedBy *commonv1.ContactLink,
) ([]*models.Proposal, error) {
	if err := internal.IsValidContactLink(searchedBy); err != nil {
		return nil, err
	}

	if scopeID == "" {
		return nil, service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	_, err := rpm.subscriptionSvc.HasAccess(ctx, searchedBy, scopeID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	return rpm.proposalRepo.GetPendingByScope(ctx, models.ProposalScopeRoom, scopeID)
}

// executeProposal dispatches an approved proposal to the appropriate business method.
func (rpm *roomProposalManagement) executeProposal(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	switch proposal.ProposalType {
	case models.ProposalTypeUpdateRoom:
		return rpm.executeUpdateRoom(ctx, proposal, actor)
	case models.ProposalTypeDeleteRoom:
		return rpm.executeDeleteRoom(ctx, proposal, actor)
	case models.ProposalTypeAddSubscriptions:
		return rpm.executeAddSubscriptions(ctx, proposal, actor)
	case models.ProposalTypeRemoveSubscriptions:
		return rpm.executeRemoveSubscriptions(ctx, proposal, actor)
	case models.ProposalTypeUpdateSubscriptionRole:
		return rpm.executeUpdateSubscriptionRole(ctx, proposal, actor)
	default:
		return fmt.Errorf("unknown proposal type: %d", proposal.ProposalType)
	}
}

func (rpm *roomProposalManagement) executeUpdateRoom(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	var req chatv1.UpdateRoomRequest
	payloadBytes, err := json.Marshal(proposal.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	if err = json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to unmarshal update room request: %w", err)
	}
	approvedCtx := withApprovedChange(ctx)
	_, updateErr := rpm.roomBusiness.UpdateRoom(approvedCtx, &req, actor)
	return updateErr
}

func (rpm *roomProposalManagement) executeDeleteRoom(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	var req chatv1.DeleteRoomRequest
	payloadBytes, err := json.Marshal(proposal.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	if err = json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to unmarshal delete room request: %w", err)
	}
	approvedCtx := withApprovedChange(ctx)
	return rpm.roomBusiness.DeleteRoom(approvedCtx, &req, actor)
}

func (rpm *roomProposalManagement) executeAddSubscriptions(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	var req chatv1.AddRoomSubscriptionsRequest
	payloadBytes, err := json.Marshal(proposal.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	if err = json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to unmarshal add subscriptions request: %w", err)
	}
	approvedCtx := withApprovedChange(ctx)
	return rpm.roomBusiness.AddRoomSubscriptions(approvedCtx, &req, actor)
}

func (rpm *roomProposalManagement) executeRemoveSubscriptions(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	var req chatv1.RemoveRoomSubscriptionsRequest
	payloadBytes, err := json.Marshal(proposal.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	if err = json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to unmarshal remove subscriptions request: %w", err)
	}
	approvedCtx := withApprovedChange(ctx)
	return rpm.roomBusiness.RemoveRoomSubscriptions(approvedCtx, &req, actor)
}

func (rpm *roomProposalManagement) executeUpdateSubscriptionRole(
	ctx context.Context,
	proposal *models.Proposal,
	actor *commonv1.ContactLink,
) error {
	var req chatv1.UpdateSubscriptionRoleRequest
	payloadBytes, err := json.Marshal(proposal.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	if err = json.Unmarshal(payloadBytes, &req); err != nil {
		return fmt.Errorf("failed to unmarshal update subscription role request: %w", err)
	}
	approvedCtx := withApprovedChange(ctx)
	return rpm.roomBusiness.UpdateSubscriptionRole(approvedCtx, &req, actor)
}

// Context key for indicating an approved change (bypass approval check).
type approvedChangeKeyType struct{}

// withApprovedChange returns a context that signals the change has been pre-approved.
func withApprovedChange(ctx context.Context) context.Context {
	return context.WithValue(ctx, approvedChangeKeyType{}, true)
}

// isApprovedChange checks if the context indicates a pre-approved change.
func isApprovedChange(ctx context.Context) bool {
	val, ok := ctx.Value(approvedChangeKeyType{}).(bool)
	return ok && val
}
