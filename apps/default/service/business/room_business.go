package business

import (
	"context"
	"fmt"
	"strconv"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	profilev1 "buf.build/gen/go/antinvestor/profile/protocolbuffers/go/profile/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
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
) RoomBusiness {
	return &roomBusiness{
		service:         service,
		roomRepo:        roomRepo,
		eventRepo:       eventRepo,
		subRepo:         subRepo,
		subscriptionSvc: subscriptionSvc,
		messageBusiness: messageBusiness,
		profileCli:      profileCli,
	}
}

func (rb *roomBusiness) CreateRoom(
	ctx context.Context,
	req *chatv1.CreateRoomRequest,
	createdBy *commonv1.ContactLink,
) (*chatv1.Room, error) {
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
	err = rb.addRoomMembers(ctx, createdRoom.GetID(), []*commonv1.ContactLink{createdBy}, "owner")
	if err != nil {
		return nil, fmt.Errorf("failed to add creator to room: %w", err)
	}

	// Add other members (excluding the creator)
	members := req.GetMembers()
	if len(members) > 0 {
		err = rb.addRoomMembers(ctx, createdRoom.GetID(), members, "member")
		if err != nil {
			return nil, fmt.Errorf("failed to add members to room: %w", err)
		}
	}

	// Send room created event using MessageBusiness
	createdByProfileID := ""
	if createdBy != nil {
		createdByProfileID = createdBy.GetProfileId()
	}
	_ = rb.sendRoomEvent(ctx, createdRoom.GetID(), createdByProfileID,
		map[string]interface{}{"content": "Room created"},
		map[string]interface{}{"created_by": createdByProfileID})

	// Return the created room as proto
	return createdRoom.ToAPI(), nil
}

func (rb *roomBusiness) GetRoom(ctx context.Context, roomID string, profileID string) (*chatv1.Room, error) {
	// Check if the user has access to the room
	hasAccess, err := rb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
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
	updatedBy string,
) (*chatv1.Room, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrRoomIDRequired
	}

	// Check if the user is an admin of the room
	isAdmin, err := rb.subscriptionSvc.HasRole(ctx, updatedBy, req.GetRoomId(), "admin")
	if err != nil {
		return nil, fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isAdmin {
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
	_ = rb.sendRoomEvent(ctx, room.GetID(), updatedBy,
		map[string]interface{}{"content": "Room details updated"},
		map[string]interface{}{"updated_by": updatedBy})

	return room.ToAPI(), nil
}

func (rb *roomBusiness) DeleteRoom(ctx context.Context, req *chatv1.DeleteRoomRequest, profileID string) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}

	roomID := req.GetRoomId()

	// Check if the user is an owner of the room
	isAdmin, err := rb.subscriptionSvc.HasRole(ctx, profileID, roomID, repository.RoleOwner)
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isAdmin {
		return service.ErrRoomDeleteDenied
	}

	// Soft delete the room
	if deleteErr := rb.roomRepo.Delete(ctx, roomID); deleteErr != nil {
		return fmt.Errorf("failed to delete room: %w", deleteErr)
	}

	// Send room deleted event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, roomID, profileID,
		map[string]interface{}{"content": "Room deleted"},
		map[string]interface{}{"deleted_by": profileID})

	return nil
}

func (rb *roomBusiness) SearchRooms(
	ctx context.Context,
	req *chatv1.SearchRoomsRequest,
	profileID string,
) ([]*chatv1.Room, error) {
	// Get all room IDs the user is subscribed to
	roomIDs, err := rb.subscriptionSvc.GetSubscribedRoomIDs(ctx, profileID)
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
	addedBy string,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.GetMembers()) == 0 {
		return service.ErrProfileIDsRequired
	}

	// Check if the user has permission to add members
	hasPermission, err := rb.subscriptionSvc.CanManageMembers(ctx, addedBy, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
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
	return rb.addRoomMembersWithRoles(ctx, req.GetRoomId(), roleMap)
}

func (rb *roomBusiness) RemoveRoomSubscriptions(
	ctx context.Context,
	req *chatv1.RemoveRoomSubscriptionsRequest,
	removerID string,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.GetSubscriptionId()) == 0 {
		return service.ErrProfileIDsRequired
	}

	// Check if the user has permission to remove members
	hasPermission, err := rb.subscriptionSvc.CanManageMembers(ctx, removerID, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
		return service.ErrRoomRemoveMembersDenied
	}

	// Remove members from the room by subscription ID
	return rb.removeRoomMembersBySubscriptionID(ctx, req.GetRoomId(), req.GetSubscriptionId(), removerID)
}

func (rb *roomBusiness) UpdateSubscriptionRole(
	ctx context.Context,
	req *chatv1.UpdateSubscriptionRoleRequest,
	updaterID string,
) error {
	if req.GetRoomId() == "" {
		return service.ErrRoomIDRequired
	}
	if req.GetSubscriptionId() == "" {
		return service.ErrUnspecifiedID
	}

	// Check if the updater has permission to update roles
	hasPermission, err := rb.subscriptionSvc.CanManageRoles(ctx, updaterID, req.GetRoomId())
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
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

	// Update roles (use first role or keep existing)
	if len(req.GetRoles()) > 0 {
		sub.Role = req.GetRoles()[0]
	}

	if _, updateErr := rb.subRepo.Update(ctx, sub); updateErr != nil {
		return fmt.Errorf("failed to update member role: %w", updateErr)
	}

	// Send member role updated event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, req.GetRoomId(), updaterID,
		map[string]interface{}{"content": "Member role updated"},
		map[string]interface{}{
			"subscription_id": req.GetSubscriptionId(),
			"roles":           req.GetRoles(),
			"updated_by":      updaterID,
		})

	return nil
}

func (rb *roomBusiness) SearchRoomSubscriptions(
	ctx context.Context,
	req *chatv1.SearchRoomSubscriptionsRequest,
	profileID string,
) ([]*chatv1.RoomSubscription, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrRoomIDRequired
	}

	// Check if the user has access to the room
	hasAccess, err := rb.subscriptionSvc.HasAccess(ctx, profileID, req.GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	if !hasAccess {
		return nil, service.ErrRoomAccessDenied
	}

	// Get all active subscriptions for the room
	subscriptions, err := rb.subRepo.GetByRoomID(ctx, req.GetRoomId(), true)
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
) error {
	// Get existing subscriptions to avoid duplicates
	existingSubs, err := rb.subRepo.GetByRoomID(ctx, roomID, false)
	if err != nil {
		return fmt.Errorf("failed to check existing subscriptions: %w", err)
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
		if link.GetContactId() != "" && rb.profileCli != nil {
			resp, err := rb.profileCli.GetByContact(ctx, connect.NewRequest(&profilev1.GetByContactRequest{
				Contact: link.GetContactId(),
			}))
			if err != nil {
				return fmt.Errorf("contact validation failed for %s: %w", link.GetContactId(), err)
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
		}

		if !existingProfileMap[link.GetProfileId()] {
			sub := &models.RoomSubscription{
				RoomID:            roomID,
				ProfileID:         link.GetProfileId(),
				ContactID:         link.GetContactId(),
				Role:              role,
				SubscriptionState: models.RoomSubscriptionStateActive,
			}
			sub.GenID(ctx)
			newSubs = append(newSubs, sub)
		}
	}

	// Save new subscriptions individually
	if len(newSubs) > 0 {
		err = rb.subRepo.BulkCreate(ctx, newSubs)
		if err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}

		// Send member added events using MessageBusiness
		for _, sub := range newSubs {
			_ = rb.sendRoomEvent(ctx, roomID, sub.ProfileID,
				map[string]interface{}{"content": "Member added to room"},
				map[string]interface{}{
					"profile_id": sub.ProfileID,
					"role":       sub.Role,
				})
		}
	}

	return nil
}

// Helper function to add members to a room (legacy support).
func (rb *roomBusiness) addRoomMembers(
	ctx context.Context,
	roomID string,
	members []*commonv1.ContactLink,
	role string,
) error {
	roleMap := make(map[*commonv1.ContactLink]string)
	for _, member := range members {
		if member != nil {
			roleMap[member] = role
		}
	}
	return rb.addRoomMembersWithRoles(ctx, roomID, roleMap)
}

// Helper function to remove members from a room.
func (rb *roomBusiness) removeRoomMembers(
	ctx context.Context,
	roomID string,
	profileIDs []string,
	removerID string,
) error {
	// Get all subscriptions for the room
	subscriptionsToRemove, err := rb.subRepo.GetByRoomIDAndProfiles(ctx, roomID, profileIDs...)
	if err != nil {
		return fmt.Errorf("failed to get subscriptions: %w", err)
	}

	var subscriptionIDs []string
	// Deactivate subscriptions
	for _, sub := range subscriptionsToRemove {
		subscriptionIDs = append(subscriptionIDs, sub.GetID())
	}

	err = rb.subRepo.Deactivate(ctx, subscriptionIDs...)
	if err != nil {
		return fmt.Errorf("failed to deactivate subscription: %w", err)
	}

	for _, sub := range subscriptionsToRemove {
		// Send member removed event using MessageBusiness
		_ = rb.sendRoomEvent(ctx, roomID, removerID,
			map[string]interface{}{"content": "Member removed from room"},
			map[string]interface{}{
				"profile_id": sub.ProfileID,
				"removed_by": removerID,
			})
	}

	return nil
}

func (rb *roomBusiness) removeRoomMembersBySubscriptionID(
	ctx context.Context,
	roomID string,
	subscriptionIDs []string,
	removerID string,
) error {
	// Deactivate subscriptions directly
	err := rb.subRepo.Deactivate(ctx, subscriptionIDs...)
	if err != nil {
		return fmt.Errorf("failed to deactivate subscription: %w", err)
	}

	// Send member removed events for each subscription
	for _, subID := range subscriptionIDs {
		// Send member removed event using MessageBusiness
		_ = rb.sendRoomEvent(ctx, roomID, removerID,
			map[string]interface{}{"content": "Member removed from room"},
			map[string]interface{}{
				"subscription_id": subID,
				"removed_by":      removerID,
			})
	}

	return nil
}

// Helper function to send room event using MessageBusiness.
func (rb *roomBusiness) sendRoomEvent(
	ctx context.Context,
	roomID string,
	senderSubscriptionID string,
	content map[string]interface{},
	metadata map[string]interface{},
) error {
	// System events don't have payloads in the new API
	// TODO: If specific data needs to be sent, use appropriate payload type (TextContent, etc.)
	req := &chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{
			{
				RoomId: roomID,
				Source: &commonv1.ContactLink{
					ProfileId: senderSubscriptionID,
				},
				Type: chatv1.RoomEventType_ROOM_EVENT_TYPE_SYSTEM,
				// No payload for system events
			},
		},
	}

	_, err := rb.messageBusiness.SendEvents(ctx, req, senderSubscriptionID)
	return err
}
