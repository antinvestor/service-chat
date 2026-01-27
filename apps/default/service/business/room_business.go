package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	profilev1 "buf.build/gen/go/antinvestor/profile/protocolbuffers/go/profile/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/util"
)

// proposalExpiryHours is the number of hours before a proposal expires.
const proposalExpiryHours = 72

type roomBusiness struct {
	service         *frame.Service
	roomRepo        repository.RoomRepository
	eventRepo       repository.RoomEventRepository
	subRepo         repository.RoomSubscriptionRepository
	proposalRepo    repository.ProposalRepository
	subscriptionSvc SubscriptionService
	messageBusiness MessageBusiness
	profileCli      profilev1connect.ProfileServiceClient
	authzMiddleware authz.Middleware
}

// NewRoomBusiness creates a new instance of RoomBusiness.
func NewRoomBusiness(
	service *frame.Service,
	roomRepo repository.RoomRepository,
	eventRepo repository.RoomEventRepository,
	subRepo repository.RoomSubscriptionRepository,
	proposalRepo repository.ProposalRepository,
	subscriptionSvc SubscriptionService,
	messageBusiness MessageBusiness,
	profileCli profilev1connect.ProfileServiceClient,
	authzMiddleware authz.Middleware,
) RoomBusiness {
	return &roomBusiness{
		service:         service,
		roomRepo:        roomRepo,
		eventRepo:       eventRepo,
		subRepo:         subRepo,
		proposalRepo:    proposalRepo,
		subscriptionSvc: subscriptionSvc,
		messageBusiness: messageBusiness,
		profileCli:      profileCli,
		authzMiddleware: authzMiddleware,
	}
}

func (rb *roomBusiness) CreateRoom(
	ctx context.Context,
	req *chatv1.CreateRoomRequest,
	createdBy *commonv1.ContactLink,
) (*chatv1.Room, error) {
	if err := internal.IsValidContactLink(createdBy); err != nil {
		return nil, err
	}

	// Validate request
	if req.GetName() == "" {
		return nil, service.ErrRoomNameRequired
	}

	// Create the room
	createdRoom := &models.Room{
		RoomType:    "group",
		Name:        req.GetName(),
		Description: req.GetDescription(),
		IsPublic:    !req.GetIsPrivate(),
	}
	createdRoom.GenID(ctx)
	if req.GetId() != "" {
		createdRoom.ID = req.GetId()
	}

	// Save the room
	err := rb.roomRepo.Create(ctx, createdRoom)
	if err != nil {
		return nil, fmt.Errorf("failed to save room: %w", err)
	}

	// Add creator as owner
	ownerSubList, err := rb.addRoomMembers(
		ctx,
		createdRoom.GetID(),
		[]*commonv1.ContactLink{createdBy},
		roleOwner,
		"",
		createdBy,
	)
	if err != nil {
		// Rollback: delete the orphaned room
		_ = rb.roomRepo.Delete(ctx, createdRoom.GetID())
		return nil, fmt.Errorf("failed to add creator to room: %w", err)
	}

	ownerSub := ownerSubList[0]

	var memberSubscriptionIDLIst []string
	// Add other members (excluding the creator)
	members := req.GetMembers()
	if len(members) > 0 {
		newSubs, newSubErr := rb.addRoomMembers(
			ctx,
			createdRoom.GetID(),
			members,
			roleMember,
			ownerSub.GetID(),
			createdBy,
		)
		if newSubErr != nil {
			// Rollback: deactivate owner subscription and delete the room
			_ = rb.subRepo.Deactivate(ctx, ownerSub.GetID())
			_ = rb.roomRepo.Delete(ctx, createdRoom.GetID())
			return nil, fmt.Errorf("failed to add members to room: %w", newSubErr)
		}

		for _, sub := range newSubs {
			memberSubscriptionIDLIst = append(memberSubscriptionIDLIst, sub.GetID())
		}
	}

	// Send room created event
	if err = rb.sendRoomChangeEvent(ctx, createdRoom.GetID(), createdBy,
		chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_CREATED,
		ownerSub.GetID(), "Room created",
		memberSubscriptionIDLIst...); err != nil {
		util.Log(ctx).WithError(err).WithField("room_id", createdRoom.GetID()).
			Warn("failed to emit room created event")
	}

	// Return the created room as proto
	return createdRoom.ToAPI(), nil
}

func (rb *roomBusiness) GetRoom(
	ctx context.Context,
	roomID string,
	searchedBy *commonv1.ContactLink,
) (*chatv1.Room, error) {
	if err := internal.IsValidContactLink(searchedBy); err != nil {
		return nil, err
	}

	// Check if the user has access to the room
	accessList, err := rb.subscriptionSvc.HasAccess(ctx, searchedBy, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	// Check if the user has access to the room
	hasAccess := false
	for _, sub := range accessList {
		if sub.RoomID == roomID && sub.IsActive() {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		return nil, service.ErrRoomAccessDenied
	}

	room, err := rb.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return nil, service.ErrRoomNotFound
		}
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	return room.ToAPI(), nil
}

func (rb *roomBusiness) UpdateRoom(
	ctx context.Context,
	req *chatv1.UpdateRoomRequest,
	updatedBy *commonv1.ContactLink,
) (*chatv1.Room, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrRoomIDRequired
	}

	if err := internal.IsValidContactLink(updatedBy); err != nil {
		return nil, err
	}

	// Check if the user is an admin of the room
	admin, err := rb.subscriptionSvc.HasRole(ctx, updatedBy, req.GetRoomId(), roleAdminLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to check admin status: %w", err)
	}

	if admin == nil {
		return nil, service.ErrRoomUpdateDenied
	}

	// Check if room requires approval for changes
	if needsApproval, approvalErr := rb.requiresApproval(ctx, req.GetRoomId()); approvalErr == nil && needsApproval {
		if crErr := rb.createProposal(ctx, req.GetRoomId(), models.ProposalTypeUpdateRoom,
			updatedBy.GetProfileId(), req); crErr != nil {
			return nil, fmt.Errorf("failed to create proposal: %w", crErr)
		}
		return nil, service.ErrProposalRequired
	}

	// Get the existing room
	room, err := rb.roomRepo.GetByID(ctx, req.GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	// Update fields if provided
	if req.GetName() != "" {
		room.Name = req.GetName()
	}
	if req.GetTopic() != "" {
		room.Description = req.GetTopic()
	}

	// Save the updated room
	_, err = rb.roomRepo.Update(ctx, room)
	if err != nil {
		return nil, fmt.Errorf("failed to update room: %w", err)
	}

	// Send room updated event
	if err = rb.sendRoomChangeEvent(ctx, req.GetRoomId(), updatedBy,
		chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_UPDATED,
		admin.GetID(), "Room details updated"); err != nil {
		util.Log(ctx).WithError(err).WithField("room_id", req.GetRoomId()).
			Warn("failed to emit room update event")
	}

	return room.ToAPI(), nil
}

func (rb *roomBusiness) DeleteRoom(
	ctx context.Context,
	req *chatv1.DeleteRoomRequest,
	deletedBy *commonv1.ContactLink,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}

	if err := internal.IsValidContactLink(deletedBy); err != nil {
		return err
	}

	roomID := req.GetRoomId()

	// Check if the user is an owner of the room
	admin, err := rb.subscriptionSvc.HasRole(ctx, deletedBy, roomID, roleOwnerLevel)
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if admin == nil {
		return service.ErrRoomDeleteDenied
	}

	// Check if room requires approval for changes
	if needsApproval, approvalErr := rb.requiresApproval(ctx, roomID); approvalErr == nil && needsApproval {
		if crErr := rb.createProposal(ctx, roomID, models.ProposalTypeDeleteRoom,
			deletedBy.GetProfileId(), req); crErr != nil {
			return fmt.Errorf("failed to create proposal: %w", crErr)
		}
		return service.ErrProposalRequired
	}

	// Deactivate all subscriptions and clean up authz
	if cleanupErr := rb.deactivateAllRoomSubscriptions(ctx, roomID); cleanupErr != nil {
		return cleanupErr
	}

	// Soft delete the room
	if deleteErr := rb.roomRepo.Delete(ctx, roomID); deleteErr != nil {
		return fmt.Errorf("failed to delete room: %w", deleteErr)
	}

	// Send room deleted event
	if err = rb.sendRoomChangeEvent(ctx, req.GetRoomId(), deletedBy,
		chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_DELETED,
		admin.GetID(), "Room deleted"); err != nil {
		util.Log(ctx).WithError(err).WithField("room_id", req.GetRoomId()).
			Warn("failed to emit room deleted event")
	}

	return nil
}

func (rb *roomBusiness) SearchRooms(
	ctx context.Context,
	req *chatv1.SearchRoomsRequest,
	searchedBy *commonv1.ContactLink,
) ([]*chatv1.Room, error) {
	if err := internal.IsValidContactLink(searchedBy); err != nil {
		return nil, err
	}

	// Get all room IDs the user is subscribed to
	roomIDs, err := rb.subscriptionSvc.GetSubscribedRoomIDs(ctx, searchedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	var resultList []*chatv1.Room

	if len(roomIDs) == 0 {
		return resultList, nil
	}

	// Build the search query
	searchOpts := []data.SearchOption{
		data.WithSearchFiltersAndByValue(
			map[string]any{"id": roomIDs},
		),
	}

	// Only add text search filters when a query is provided
	if query := req.GetQuery(); query != "" {
		likePattern := "%" + query + "%"
		searchOpts = append(searchOpts, data.WithSearchFiltersOrByValue(
			map[string]any{
				"name ILIKE ?":        likePattern,
				"description ILIKE ?": likePattern,
			},
		))
	}

	cursor := req.GetCursor()
	if cursor != nil {
		page, strErr := strconv.Atoi(cursor.GetPage())
		if strErr != nil {
			page = 0
		}
		searchOpts = append(searchOpts, data.WithSearchOffset(page), data.WithSearchLimit(int(cursor.GetLimit())))
	}

	query := data.NewSearchQuery(searchOpts...)

	// Get rooms - need to convert JobResultPipe to slice
	roomsPipe, err := rb.roomRepo.Search(ctx, query)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to search rooms")
		return nil, fmt.Errorf("failed to search rooms: %w", err)
	}

	for res := range roomsPipe.ResultChan() {
		if res.IsError() {
			return resultList, res.Error()
		}

		resultRoomSlice := res.Item()
		for _, room := range resultRoomSlice {
			resultList = append(resultList, room.ToAPI())
		}
	}

	return resultList, nil
}

func (rb *roomBusiness) AddRoomSubscriptions(
	ctx context.Context,
	req *chatv1.AddRoomSubscriptionsRequest,
	addedBy *commonv1.ContactLink,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.GetMembers()) == 0 {
		return service.ErrProfileIDsRequired
	}

	if err := internal.IsValidContactLink(addedBy); err != nil {
		return err
	}

	// Check if the user has permission to add members
	admin, err := rb.subscriptionSvc.CanManageMembers(ctx, addedBy, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if admin == nil {
		return service.ErrRoomAddMembersDenied
	}

	// Check if room requires approval for changes
	if needsApproval, approvalErr := rb.requiresApproval(ctx, req.GetRoomId()); approvalErr == nil && needsApproval {
		if crErr := rb.createProposal(ctx, req.GetRoomId(), models.ProposalTypeAddSubscriptions,
			addedBy.GetProfileId(), req); crErr != nil {
			return fmt.Errorf("failed to create proposal: %w", crErr)
		}
		return service.ErrProposalRequired
	}

	// Extract ContactLinks and roles from members, preserving request order
	var membersWithRoles []memberWithRole

	for _, member := range req.GetMembers() {
		// Use first role or default to "member"
		role := "member"
		if len(member.GetRoles()) > 0 {
			role = member.GetRoles()[0]
		}
		if member.GetMember() != nil {
			membersWithRoles = append(membersWithRoles, memberWithRole{
				Link: member.GetMember(),
				Role: role,
			})
		}
	}

	// Add members with their respective roles.
	// PartialBatchError is returned when some members were added successfully
	// but others failed validation. Propagate it so the handler can report details.
	_, err = rb.addRoomMembersWithRoles(ctx, req.GetRoomId(), membersWithRoles, admin.GetID(), addedBy)
	if err != nil {
		if _, ok := service.IsPartialBatchError(err); ok {
			return err
		}
		return fmt.Errorf("failed to add members to room: %w", err)
	}

	return nil
}

func (rb *roomBusiness) RemoveRoomSubscriptions(
	ctx context.Context,
	req *chatv1.RemoveRoomSubscriptionsRequest,
	removedBy *commonv1.ContactLink,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.GetSubscriptionId()) == 0 {
		return service.ErrProfileIDsRequired
	}

	if err := internal.IsValidContactLink(removedBy); err != nil {
		return err
	}

	// Check if the user has permission to remove members
	admin, err := rb.subscriptionSvc.CanManageMembers(ctx, removedBy, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if admin == nil {
		return service.ErrRoomRemoveMembersDenied
	}

	// Check if room requires approval for changes
	if needsApproval, approvalErr := rb.requiresApproval(ctx, req.GetRoomId()); approvalErr == nil && needsApproval {
		if crErr := rb.createProposal(ctx, req.GetRoomId(), models.ProposalTypeRemoveSubscriptions,
			removedBy.GetProfileId(), req); crErr != nil {
			return fmt.Errorf("failed to create proposal: %w", crErr)
		}
		return service.ErrProposalRequired
	}

	// Remove members from the room by subscription ID
	return rb.removeRoomMembersBySubscriptionID(ctx, req.GetRoomId(), req.GetSubscriptionId(), admin.GetID(), removedBy)
}

func (rb *roomBusiness) UpdateSubscriptionRole(
	ctx context.Context,
	req *chatv1.UpdateSubscriptionRoleRequest,
	actor *commonv1.ContactLink,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if req.GetSubscriptionId() == "" {
		return service.ErrUnspecifiedID
	}

	if err := internal.IsValidContactLink(actor); err != nil {
		return err
	}

	// Check if the updater has permission to update roles
	admin, err := rb.subscriptionSvc.CanManageRoles(ctx, actor, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if admin == nil {
		return service.ErrRoomUpdateRoleDenied
	}

	// Check if room requires approval for changes
	if needsApproval, approvalErr := rb.requiresApproval(ctx, req.GetRoomId()); approvalErr == nil && needsApproval {
		if crErr := rb.createProposal(ctx, req.GetRoomId(), models.ProposalTypeUpdateSubscriptionRole,
			actor.GetProfileId(), req); crErr != nil {
			return fmt.Errorf("failed to create proposal: %w", crErr)
		}
		return service.ErrProposalRequired
	}

	// Update the member's role by subscription ID
	sub, err := rb.subRepo.GetByID(ctx, req.GetSubscriptionId())
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return service.ErrRoomMemberNotFound
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Verify subscription belongs to the specified room
	if sub.RoomID != req.GetRoomId() {
		return service.ErrRoomMemberNotFound
	}

	// Capture old role for authz update
	oldRole := sub.Role

	// Update roles (use first role or keep existing)
	var newRole string
	if len(req.GetRoles()) > 0 {
		newRole = req.GetRoles()[0] // Primary role for authz
		sub.Role = strings.Join(req.GetRoles(), ",")
	}

	if _, updateErr := rb.subRepo.Update(ctx, sub, "role"); updateErr != nil {
		return fmt.Errorf("failed to update member role: %w", updateErr)
	}

	// Sync authorization tuple - update role in Keto
	if rb.authzMiddleware != nil && newRole != "" {
		if authzErr := rb.authzMiddleware.UpdateRoomMemberRole(
			ctx,
			req.GetRoomId(),
			sub.ProfileID,
			oldRole,
			newRole,
		); authzErr != nil {
			util.Log(ctx).WithError(authzErr).
				WithField("room_id", req.GetRoomId()).
				WithField("profile_id", sub.ProfileID).
				WithField("old_role", oldRole).
				WithField("new_role", newRole).
				Warn("failed to update authorization tuple for role change")
		}
	}

	// Send member role updated event
	if err = rb.sendRoomChangeEvent(ctx, req.GetRoomId(), actor,
		chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_ROLE_CHANGED,
		admin.GetID(), "Member(s) role updated",
		req.GetSubscriptionId()); err != nil {
		util.Log(ctx).WithError(err).WithField("room_id", req.GetRoomId()).
			Warn("failed to emit role update event")
	}

	return nil
}

func (rb *roomBusiness) SearchRoomSubscriptions(
	ctx context.Context,
	req *chatv1.SearchRoomSubscriptionsRequest,
	searchedBy *commonv1.ContactLink,
) ([]*chatv1.RoomSubscription, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrRoomIDRequired
	}

	if err := internal.IsValidContactLink(searchedBy); err != nil {
		return nil, err
	}

	// Check if the user has access to the room
	accessList, err := rb.subscriptionSvc.HasAccess(ctx, searchedBy, req.GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	// Check if the user has access to the room
	hasAccess := false
	for _, sub := range accessList {
		if sub.RoomID == req.GetRoomId() && sub.IsActive() {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		return nil, service.ErrRoomAccessDenied
	}

	// Get all active subscriptions for the room
	subscriptions, err := rb.subRepo.GetByRoomID(ctx, req.GetRoomId(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get room members: %w", err)
	}

	// Convert to proto
	protoSubs := make([]*chatv1.RoomSubscription, 0, len(subscriptions))
	for _, sub := range subscriptions {
		protoSubs = append(protoSubs, sub.ToAPI())
	}

	return protoSubs, nil
}

// memberWithRole pairs a contact link with a role, preserving request order.
type memberWithRole struct {
	Link *commonv1.ContactLink
	Role string
}

// Helper function to add members to a room with specific roles.
func (rb *roomBusiness) addRoomMembersWithRoles(
	ctx context.Context,
	roomID string,
	members []memberWithRole,
	actorSubscriptionID string,
	actor *commonv1.ContactLink,
) ([]*models.RoomSubscription, error) {
	// Get existing subscriptions to avoid duplicates
	existingSubs, err := rb.subRepo.GetByRoomID(ctx, roomID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing subscriptions: %w", err)
	}

	// Create a map of existing profile IDs
	existingProfileMap := make(map[string]bool)
	for _, sub := range existingSubs {
		existingProfileMap[sub.ProfileID] = true
	}

	// Create new subscriptions for profiles that don't exist.
	// Continue processing after individual validation failures so that
	// valid members are still added (partial success).
	var newSubs []*models.RoomSubscription
	var itemErrors []service.ItemError
	for idx, mwr := range members {
		// Validate Contact and Profile ID via Profile Service
		if validateErr := rb.validateContactProfile(ctx, mwr.Link); validateErr != nil {
			itemErrors = append(itemErrors, service.ItemError{
				Index:   idx,
				ItemID:  mwr.Link.GetProfileId(),
				Message: validateErr.Error(),
			})
			continue
		}

		if !existingProfileMap[mwr.Link.GetProfileId()] {
			sub := &models.RoomSubscription{
				RoomID:            roomID,
				ProfileID:         mwr.Link.GetProfileId(),
				ContactID:         mwr.Link.GetContactId(),
				SubscriptionState: models.RoomSubscriptionStateActive,
				Role:              mwr.Role,
			}
			newSubs = append(newSubs, sub)
		}
	}

	// Save new subscriptions individually
	if len(newSubs) > 0 {
		err = rb.subRepo.BulkCreate(ctx, newSubs)
		if err != nil {
			return nil, fmt.Errorf("failed to create subscription: %w", err)
		}

		var newMembers []string
		for _, sub := range newSubs {
			newMembers = append(newMembers, sub.GetID())

			// Sync authorization tuple to Keto
			if rb.authzMiddleware != nil {
				if authzErr := rb.authzMiddleware.AddRoomMember(ctx, roomID, sub.ProfileID, sub.Role); authzErr != nil {
					util.Log(ctx).WithError(authzErr).
						WithField("room_id", roomID).
						WithField("profile_id", sub.ProfileID).
						WithField("role", sub.Role).
						Warn("failed to sync authorization tuple for new room member")
				}
			}
		}

		_ = rb.sendRoomChangeEvent(ctx, roomID, actor,
			chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_MEMBER_ADDED,
			actorSubscriptionID, "Member(s) added to room",
			newMembers...)
	}

	// If some members failed validation, return partial batch error
	// alongside the successfully added subscriptions
	if len(itemErrors) > 0 {
		return newSubs, &service.PartialBatchError{
			Succeeded: len(newSubs),
			Failed:    len(itemErrors),
			Errors:    itemErrors,
		}
	}

	return newSubs, nil
}

// Helper function to add members to a room (legacy support).
func (rb *roomBusiness) addRoomMembers(
	ctx context.Context,
	roomID string,
	members []*commonv1.ContactLink,
	role string,

	actorSubscriptionID string,
	actor *commonv1.ContactLink,
) ([]*models.RoomSubscription, error) {
	var membersWithRoles []memberWithRole
	for _, member := range members {
		if member != nil {
			membersWithRoles = append(membersWithRoles, memberWithRole{Link: member, Role: role})
		}
	}
	return rb.addRoomMembersWithRoles(ctx, roomID, membersWithRoles, actorSubscriptionID, actor)
}

// validateContactProfile validates contact and profile ID via Profile Service.
func (rb *roomBusiness) validateContactProfile(ctx context.Context, link *commonv1.ContactLink) error {
	if link.GetContactId() == "" || rb.profileCli == nil {
		return nil
	}

	resp, profileErr := rb.profileCli.GetByContact(ctx, connect.NewRequest(&profilev1.GetByContactRequest{
		Contact: link.GetContactId(),
	}))
	if profileErr != nil {
		return fmt.Errorf("contact validation failed for %s: %w", link.GetContactId(), profileErr)
	}

	// The profile as source of truth
	foundProfileID := resp.Msg.GetData().GetId()

	if link.GetProfileId() != "" {
		if link.GetProfileId() != foundProfileID {
			return fmt.Errorf("profile id mismatch for contact %s: expected %s, got %s",
				link.GetContactId(), foundProfileID, link.GetProfileId())
		}
	} else {
		// Populate ProfileID if missing
		link.ProfileId = foundProfileID
	}

	return nil
}

func (rb *roomBusiness) removeRoomMembersBySubscriptionID(
	ctx context.Context,
	roomID string,
	subscriptionIDs []string,
	actorSubscriptionID string,
	actor *commonv1.ContactLink,
) error {
	// Get subscriptions to find profile IDs for authz cleanup.
	// Track lookup failures to report to the caller.
	var profileIDsToRemove []string
	var lookupErrors []service.ItemError
	for i, subID := range subscriptionIDs {
		sub, subErr := rb.subRepo.GetByID(ctx, subID)
		if subErr != nil || sub == nil {
			msg := "subscription not found"
			if subErr != nil {
				msg = subErr.Error()
			}
			lookupErrors = append(lookupErrors, service.ItemError{
				Index:   i,
				ItemID:  subID,
				Message: msg,
			})
			continue
		}
		profileIDsToRemove = append(profileIDsToRemove, sub.ProfileID)
	}

	// Deactivate subscriptions directly
	err := rb.subRepo.Deactivate(ctx, subscriptionIDs...)
	if err != nil {
		return fmt.Errorf("failed to deactivate subscription: %w", err)
	}

	// Sync authorization tuples - remove from Keto
	if rb.authzMiddleware != nil {
		for _, profileID := range profileIDsToRemove {
			if authzErr := rb.authzMiddleware.RemoveRoomMember(ctx, roomID, profileID); authzErr != nil {
				util.Log(ctx).WithError(authzErr).
					WithField("room_id", roomID).
					WithField("profile_id", profileID).
					Warn("failed to remove authorization tuple for removed room member")
			}
		}
	}

	// Send member removed event
	_ = rb.sendRoomChangeEvent(ctx, roomID, actor,
		chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_MEMBER_REMOVED,
		actorSubscriptionID, "Member(s) removed from room",
		subscriptionIDs...)

	// If some subscription lookups failed, return partial batch error
	if len(lookupErrors) > 0 {
		return &service.PartialBatchError{
			Succeeded: len(subscriptionIDs) - len(lookupErrors),
			Failed:    len(lookupErrors),
			Errors:    lookupErrors,
		}
	}

	return nil
}

// deactivateAllRoomSubscriptions deactivates all subscriptions for a room and removes authz tuples.
func (rb *roomBusiness) deactivateAllRoomSubscriptions(ctx context.Context, roomID string) error {
	allSubs, subErr := rb.subRepo.GetByRoomID(ctx, roomID, nil)
	if subErr != nil {
		return fmt.Errorf("failed to get room subscriptions for cleanup: %w", subErr)
	}

	if len(allSubs) == 0 {
		return nil
	}

	subIDs := make([]string, 0, len(allSubs))
	for _, sub := range allSubs {
		subIDs = append(subIDs, sub.GetID())
	}
	if deactivateErr := rb.subRepo.Deactivate(ctx, subIDs...); deactivateErr != nil {
		return fmt.Errorf("failed to deactivate room subscriptions: %w", deactivateErr)
	}

	if rb.authzMiddleware != nil {
		for _, sub := range allSubs {
			if authzErr := rb.authzMiddleware.RemoveRoomMember(ctx, roomID, sub.ProfileID); authzErr != nil {
				util.Log(ctx).WithError(authzErr).
					WithField("room_id", roomID).
					WithField("profile_id", sub.ProfileID).
					Warn("failed to remove authorization tuple during room deletion")
			}
		}
	}

	return nil
}

// sendRoomChangeEvent emits a RoomChangeContent event for room lifecycle changes.
func (rb *roomBusiness) sendRoomChangeEvent(
	ctx context.Context,
	roomID string,
	senderContact *commonv1.ContactLink,
	action chatv1.RoomChangeAction,
	actorSubscriptionID string,
	body string,
	targetSubscriptionIDs ...string,
) error {
	req := &chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{
			{
				RoomId: roomID,
				Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_SYSTEM,
				Payload: &chatv1.Payload{
					Type: chatv1.PayloadType_PAYLOAD_TYPE_ROOM_CHANGE,
					Data: &chatv1.Payload_RoomChange{
						RoomChange: &chatv1.RoomChangeContent{
							Action:                action,
							ActorSubscriptionId:   actorSubscriptionID,
							TargetSubscriptionIds: targetSubscriptionIDs,
							Body:                  body,
						},
					},
				},
			},
		},
	}

	_, err := rb.messageBusiness.SendEvents(ctx, req, senderContact)
	return err
}

// requiresApproval checks if a room has the approval flag set.
// Returns false if the context indicates a pre-approved change.
func (rb *roomBusiness) requiresApproval(ctx context.Context, roomID string) (bool, error) {
	if isApprovedChange(ctx) {
		return false, nil
	}

	room, err := rb.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to get room: %w", err)
	}
	return room.RequiresApproval, nil
}

// createProposal creates a pending proposal for a room operation that requires approval.
func (rb *roomBusiness) createProposal(
	ctx context.Context,
	roomID string,
	proposalType models.ProposalType,
	requestedBy string,
	payload any,
) error {
	if rb.proposalRepo == nil {
		return errors.New("proposal repository not configured; cannot create approval requests")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal proposal payload: %w", err)
	}

	var payloadMap data.JSONMap
	if err = json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		return fmt.Errorf("failed to convert payload to map: %w", err)
	}

	proposal := &models.Proposal{
		ScopeType:    models.ProposalScopeRoom,
		ScopeID:      roomID,
		ProposalType: proposalType,
		RequestedBy:  requestedBy,
		Payload:      payloadMap,
		State:        models.ProposalStatePending,
		ExpiresAt:    time.Now().Add(proposalExpiryHours * time.Hour),
	}
	proposal.GenID(ctx)

	if createErr := rb.proposalRepo.Create(ctx, proposal); createErr != nil {
		return fmt.Errorf("failed to create proposal: %w", createErr)
	}

	return nil
}
