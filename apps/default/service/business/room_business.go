package business

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

type roomBusiness struct {
	service         *frame.Service
	roomRepo        repository.RoomRepository
	eventRepo       repository.RoomEventRepository
	subRepo         repository.RoomSubscriptionRepository
	subscriptionSvc SubscriptionService
	messageBusiness MessageBusiness
	profileCli      profilev1connect.ProfileServiceClient
	authzMiddleware authz.AuthzMiddleware
}

// NewRoomBusiness creates a new instance of RoomBusiness.
func NewRoomBusiness(
	service *frame.Service,
	roomRepo repository.RoomRepository,
	eventRepo repository.RoomEventRepository,
	subRepo repository.RoomSubscriptionRepository,
	subscriptionSvc SubscriptionService,
	messageBusiness MessageBusiness,
	profileCli profilev1connect.ProfileServiceClient,
	authzMiddleware authz.AuthzMiddleware,
) RoomBusiness {
	return &roomBusiness{
		service:         service,
		roomRepo:        roomRepo,
		eventRepo:       eventRepo,
		subRepo:         subRepo,
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
			return nil, fmt.Errorf("failed to add members to room: %w", newSubErr)
		}

		for _, sub := range newSubs {
			memberSubscriptionIDLIst = append(memberSubscriptionIDLIst, sub.GetID())
		}
	}

	// Send room created event using MessageBusiness
	err = rb.sendRoomEvent(ctx, createdRoom.GetID(), createdBy,

		&chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
			Data: &chatv1.Payload_Moderation{
				Moderation: &chatv1.ModerationContent{
					Body:                  "Room created",
					ActorSubscriptionId:   ownerSub.GetID(),
					TargetSubscriptionIds: memberSubscriptionIDLIst,
				},
			},
		})
	if err != nil {
		return nil, fmt.Errorf("failed to emit room created event: %w", err)
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
	for _, sub := range accessList {
		if sub.RoomID != roomID || !sub.IsActive() {
			return nil, service.ErrRoomAccessDenied
		}
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

	// Send room updated event using MessageBusiness
	err = rb.sendRoomEvent(ctx, req.GetRoomId(), updatedBy,

		&chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
			Data: &chatv1.Payload_Moderation{
				Moderation: &chatv1.ModerationContent{
					Body:                "Room details updated",
					ActorSubscriptionId: admin.GetID(),
				},
			},
		})
	if err != nil {
		return nil, fmt.Errorf("failed to emit room update event: %w", err)
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

	// Soft delete the room
	if deleteErr := rb.roomRepo.Delete(ctx, roomID); deleteErr != nil {
		return fmt.Errorf("failed to delete room: %w", deleteErr)
	}

	// Send room deleted event using MessageBusiness
	return rb.sendRoomEvent(ctx, req.GetRoomId(), deletedBy,

		&chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
			Data: &chatv1.Payload_Moderation{
				Moderation: &chatv1.ModerationContent{
					Body:                "Room deleted",
					ActorSubscriptionId: admin.GetID(),
				},
			},
		})
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
		data.WithSearchFiltersOrByValue(
			map[string]any{
				"name % ?": req.GetQuery(),
				"searchable @@ websearch_to_tsquery( 'english', ?) ": req.GetQuery()},
		)}

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

	// Extract ContactLinks and roles from members
	roleMap := make(map[*commonv1.ContactLink]string)

	for _, member := range req.GetMembers() {
		// Use first role or default to "member"
		role := "member"
		if len(member.GetRoles()) > 0 {
			role = member.GetRoles()[0]
		}
		// Use ContactLink directly as key
		if member.GetMember() != nil {
			roleMap[member.GetMember()] = role
		}
	}

	// Add members with their respective roles
	_, err = rb.addRoomMembersWithRoles(ctx, req.GetRoomId(), roleMap, admin.GetID(), addedBy)
	if err != nil {
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

	// Send member role updated event using MessageBusiness
	return rb.sendRoomEvent(ctx, req.GetRoomId(), actor,

		&chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
			Data: &chatv1.Payload_Moderation{
				Moderation: &chatv1.ModerationContent{
					Body:                  "Member(s) role updated",
					ActorSubscriptionId:   admin.GetID(),
					TargetSubscriptionIds: []string{req.GetSubscriptionId()},
				},
			},
		})
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
	for _, sub := range accessList {
		if sub.RoomID != req.GetRoomId() || !sub.IsActive() {
			return nil, service.ErrRoomAccessDenied
		}
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

// Helper function to add members to a room with specific roles.
func (rb *roomBusiness) addRoomMembersWithRoles(
	ctx context.Context,
	roomID string,
	roleMap map[*commonv1.ContactLink]string,
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

	// Create new subscriptions for profiles that don't exist
	var newSubs []*models.RoomSubscription
	for link, role := range roleMap {
		// Validate Contact and Profile ID via Profile Service
		if validateErr := rb.validateContactProfile(ctx, link); validateErr != nil {
			return nil, validateErr
		}

		if !existingProfileMap[link.GetProfileId()] {
			sub := &models.RoomSubscription{
				RoomID:            roomID,
				ProfileID:         link.GetProfileId(),
				SubscriptionState: models.RoomSubscriptionStateActive,
				Role:              role,
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

		_ = rb.sendRoomEvent(ctx, roomID, actor,

			&chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
				Data: &chatv1.Payload_Moderation{
					Moderation: &chatv1.ModerationContent{
						Body:                  "Member(s) added to room",
						ActorSubscriptionId:   actorSubscriptionID,
						TargetSubscriptionIds: newMembers,
					},
				},
			})
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
	roleMap := make(map[*commonv1.ContactLink]string)
	for _, member := range members {
		if member != nil {
			roleMap[member] = role
		}
	}
	return rb.addRoomMembersWithRoles(ctx, roomID, roleMap, actorSubscriptionID, actor)
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
	// Get subscriptions to find profile IDs for authz cleanup
	var profileIDsToRemove []string
	for _, subID := range subscriptionIDs {
		sub, subErr := rb.subRepo.GetByID(ctx, subID)
		if subErr == nil && sub != nil {
			profileIDsToRemove = append(profileIDsToRemove, sub.ProfileID)
		}
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

	// Send member removed event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, roomID, actor,

		&chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
			Data: &chatv1.Payload_Moderation{
				Moderation: &chatv1.ModerationContent{
					Body:                  "Member(s) removed from room",
					ActorSubscriptionId:   actorSubscriptionID,
					TargetSubscriptionIds: subscriptionIDs,
				},
			},
		})

	return nil
}

// Helper function to send room event using MessageBusiness.
func (rb *roomBusiness) sendRoomEvent(
	ctx context.Context,
	roomID string,
	senderContact *commonv1.ContactLink,
	payload *chatv1.Payload,
) error {
	// System events don't have payloads in the new API
	// TODO: If specific data needs to be sent, use appropriate payload type (TextContent, etc.)
	req := &chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{
			{
				RoomId:  roomID,
				Type:    chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
				Payload: payload,
			},
		},
	}

	_, err := rb.messageBusiness.SendEvents(ctx, req, senderContact)
	return err
}
