package business

import (
	"context"
	"fmt"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/framedata"
	"google.golang.org/protobuf/types/known/structpb"
)

type roomBusiness struct {
	service         *frame.Service
	roomRepo        repository.RoomRepository
	eventRepo       repository.RoomEventRepository
	subRepo         repository.RoomSubscriptionRepository
	subscriptionSvc SubscriptionService
	messageBusiness MessageBusiness
}

// NewRoomBusiness creates a new instance of RoomBusiness
func NewRoomBusiness(
	service *frame.Service,
	roomRepo repository.RoomRepository,
	eventRepo repository.RoomEventRepository,
	subRepo repository.RoomSubscriptionRepository,
	subscriptionSvc SubscriptionService,
	messageBusiness MessageBusiness,
) RoomBusiness {
	return &roomBusiness{
		service:         service,
		roomRepo:        roomRepo,
		eventRepo:       eventRepo,
		subRepo:         subRepo,
		subscriptionSvc: subscriptionSvc,
		messageBusiness: messageBusiness,
	}
}

func (rb *roomBusiness) CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest, createdBy string) (*chatv1.Room, error) {
	// Validate request
	if req.GetName() == "" {
		return nil, service.ErrRoomNameRequired
	}

	// Create the room
	createdRoom := &models.Room{
		RoomType:    "group",
		Name:        req.Name,
		Description: req.Description,
		IsPublic:    !req.GetIsPrivate(),
	}
	createdRoom.GenID(ctx)
	if req.GetId() != "" {
		createdRoom.ID = req.GetId()
	}

	// Save the room
	err := rb.roomRepo.Save(ctx, createdRoom)
	if err != nil {
		return nil, fmt.Errorf("failed to save room: %w", err)
	}

	// Add creator as owner
	err = rb.addRoomMembers(ctx, createdRoom.GetID(), []string{createdBy}, "owner")
	if err != nil {
		return nil, fmt.Errorf("failed to add creator to room: %w", err)
	}

	// Add other members (excluding the creator)
	memberIDs := removeDuplicateAndExclude(req.GetMembers(), createdBy)
	if len(memberIDs) > 0 {
		err = rb.addRoomMembers(ctx, createdRoom.GetID(), memberIDs, "member")
		if err != nil {
			return nil, fmt.Errorf("failed to add members to room: %w", err)
		}
	}

	// Send room created event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, createdRoom.GetID(), createdBy, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		map[string]interface{}{"content": "Room created"},
		map[string]interface{}{"created_by": createdBy})

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
		if frame.ErrorIsNoRows(err) {
			return nil, service.ErrRoomNotFound
		}
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	return room.ToAPI(), nil
}

func (rb *roomBusiness) UpdateRoom(ctx context.Context, req *chatv1.UpdateRoomRequest, updatedBy string) (*chatv1.Room, error) {
	if req.RoomId == "" {
		return nil, service.ErrRoomIDRequired
	}

	// Check if the user is an admin of the room
	isAdmin, err := rb.subscriptionSvc.HasRole(ctx, updatedBy, req.RoomId, "admin")
	if err != nil {
		return nil, fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isAdmin {
		return nil, service.ErrRoomUpdateDenied
	}

	// Get the existing room
	room, err := rb.roomRepo.GetByID(ctx, req.RoomId)
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
	err = rb.roomRepo.Save(ctx, room)
	if err != nil {
		return nil, fmt.Errorf("failed to update room: %w", err)
	}

	// Send room updated event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, room.GetID(), updatedBy, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		map[string]interface{}{"content": "Room details updated"},
		map[string]interface{}{"updated_by": updatedBy})

	return room.ToAPI(), nil
}

func (rb *roomBusiness) DeleteRoom(ctx context.Context, req *chatv1.DeleteRoomRequest, profileID string) error {
	if req.RoomId == "" {
		return service.ErrRoomIDRequired
	}

	roomID := req.RoomId

	// Check if the user is an admin of the room
	isAdmin, err := rb.subscriptionSvc.HasRole(ctx, profileID, roomID, "admin")
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isAdmin {
		return service.ErrRoomDeleteDenied
	}

	// Soft delete the room
	if err := rb.roomRepo.Delete(ctx, roomID); err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	// Send room deleted event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, roomID, profileID, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		map[string]interface{}{"content": "Room deleted"},
		map[string]interface{}{"deleted_by": profileID})

	return nil
}

func (rb *roomBusiness) SearchRooms(ctx context.Context, req *chatv1.SearchRoomsRequest, profileID string) ([]*chatv1.Room, error) {
	// Get all room IDs the user is subscribed to
	roomIDs, err := rb.subscriptionSvc.GetSubscribedRoomIDs(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	if len(roomIDs) == 0 {
		return []*chatv1.Room{}, nil
	}

	// Build the search query
	query := &framedata.SearchQuery{
		Query: req.Query,
	}

	if req.Count > 0 {
		offset := 0
		if req.Page > 0 {
			offset = int(req.Page-1) * int(req.Count)
		}
		query.Pagination = &framedata.Paginator{
			Limit:  int(req.Count),
			Offset: offset,
		}
	}

	// Get rooms - need to convert JobResultPipe to slice
	roomsPipe, err := rb.roomRepo.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search rooms: %w", err)
	}

	// Collect rooms from pipe
	var rooms []*models.Room
	// TODO: Implement proper pipe reading based on frame.JobResultPipe interface
	// For now, return empty list
	_ = roomsPipe

	// Filter rooms by subscribed IDs
	roomIDMap := make(map[string]bool)
	for _, id := range roomIDs {
		roomIDMap[id] = true
	}

	// Convert to proto and filter
	protoRooms := make([]*chatv1.Room, 0, len(rooms))
	for _, room := range rooms {
		if roomIDMap[room.GetID()] {
			protoRooms = append(protoRooms, room.ToAPI())
		}
	}

	return protoRooms, nil
}

func (rb *roomBusiness) AddRoomSubscriptions(ctx context.Context, req *chatv1.AddRoomSubscriptionsRequest, addedBy string) error {
	if req.RoomId == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.GetMembers()) == 0 {
		return service.ErrProfileIDsRequired
	}

	// Check if the user has permission to add members
	hasPermission, err := rb.subscriptionSvc.CanManageMembers(ctx, addedBy, req.RoomId)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
		return service.ErrRoomAddMembersDenied
	}

	// Extract profile IDs and roles from members
	var profileIDs []string
	roleMap := make(map[string]string)

	for _, member := range req.GetMembers() {
		profileIDs = append(profileIDs, member.ProfileId)
		// Use first role or default to "member"
		role := "member"
		if len(member.Roles) > 0 {
			role = member.Roles[0]
		}
		roleMap[member.ProfileId] = role
	}

	// Add members with their respective roles
	return rb.addRoomMembersWithRoles(ctx, req.RoomId, roleMap)
}

func (rb *roomBusiness) RemoveRoomSubscriptions(ctx context.Context, req *chatv1.RemoveRoomSubscriptionsRequest, removerID string) error {
	if req.RoomId == "" {
		return service.ErrRoomIDRequired
	}
	if len(req.ProfileIds) == 0 {
		return service.ErrProfileIDsRequired
	}

	// Check if the user has permission to remove members
	hasPermission, err := rb.subscriptionSvc.CanManageMembers(ctx, removerID, req.RoomId)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
		return service.ErrRoomRemoveMembersDenied
	}

	// Remove members from the room
	return rb.removeRoomMembers(ctx, req.RoomId, req.ProfileIds, removerID)
}

func (rb *roomBusiness) UpdateSubscriptionRole(ctx context.Context, req *chatv1.UpdateSubscriptionRoleRequest, updaterID string) error {
	if req.RoomId == "" {
		return service.ErrRoomIDRequired
	}
	if req.ProfileId == "" {
		return service.ErrUnspecifiedID
	}

	// Check if the updater has permission to update roles
	hasPermission, err := rb.subscriptionSvc.CanManageRoles(ctx, updaterID, req.RoomId)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasPermission {
		return service.ErrRoomUpdateRoleDenied
	}

	// Update the member's role
	sub, err := rb.subRepo.GetActiveByRoomAndProfile(ctx, req.RoomId, req.ProfileId)
	if err != nil {
		if frame.ErrorIsNoRows(err) {
			return service.ErrRoomMemberNotFound
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update roles (use first role or keep existing)
	if len(req.Roles) > 0 {
		sub.Role = req.Roles[0]
	}

	if err := rb.subRepo.Save(ctx, sub); err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	// Send member role updated event using MessageBusiness
	_ = rb.sendRoomEvent(ctx, req.RoomId, updaterID, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		map[string]interface{}{"content": "Member role updated"},
		map[string]interface{}{
			"profile_id": req.ProfileId,
			"roles":      req.Roles,
			"updated_by": updaterID,
		})

	return nil
}

func (rb *roomBusiness) SearchRoomSubscriptions(ctx context.Context, req *chatv1.SearchRoomSubscriptionsRequest, profileID string) ([]*chatv1.RoomSubscription, error) {
	if req.RoomId == "" {
		return nil, service.ErrRoomIDRequired
	}

	// Check if the user has access to the room
	hasAccess, err := rb.subscriptionSvc.HasAccess(ctx, profileID, req.RoomId)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	if !hasAccess {
		return nil, service.ErrRoomAccessDenied
	}

	// Get all active subscriptions for the room
	subscriptions, err := rb.subRepo.GetByRoomID(ctx, req.RoomId, true)
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

// Helper function to add members to a room with specific roles
func (rb *roomBusiness) addRoomMembersWithRoles(ctx context.Context, roomID string, roleMap map[string]string) error {
	profileIDs := make([]string, 0, len(roleMap))
	for profileID := range roleMap {
		profileIDs = append(profileIDs, profileID)
	}

	// Get existing subscriptions to avoid duplicates
	existingSubs, err := rb.subRepo.GetByRoomID(ctx, roomID, false)
	if err != nil {
		return fmt.Errorf("failed to check existing subscriptions: %w", err)
	}

	// Create a map of existing profile IDs
	existingMap := make(map[string]bool)
	for _, sub := range existingSubs {
		existingMap[sub.ProfileID] = true
	}

	// Create new subscriptions for profiles that don't exist
	var newSubs []*models.RoomSubscription
	for profileID, role := range roleMap {
		if !existingMap[profileID] {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: profileID,
				Role:      role,
				IsActive:  true,
			}
			sub.GenID(ctx)
			newSubs = append(newSubs, sub)
		}
	}

	// Save new subscriptions individually
	if len(newSubs) > 0 {
		for _, sub := range newSubs {
			if err := rb.subRepo.Save(ctx, sub); err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}
		}

		// Send member added events using MessageBusiness
		for _, sub := range newSubs {
			_ = rb.sendRoomEvent(ctx, roomID, sub.ProfileID, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
				map[string]interface{}{"content": "Member added to room"},
				map[string]interface{}{
					"profile_id": sub.ProfileID,
					"role":       sub.Role,
				})
		}
	}

	return nil
}

// Helper function to add members to a room (legacy support)
func (rb *roomBusiness) addRoomMembers(ctx context.Context, roomID string, profileIDs []string, role string) error {
	roleMap := make(map[string]string)
	for _, profileID := range profileIDs {
		roleMap[profileID] = role
	}
	return rb.addRoomMembersWithRoles(ctx, roomID, roleMap)
}

// Helper function to remove members from a room
func (rb *roomBusiness) removeRoomMembers(ctx context.Context, roomID string, profileIDs []string, removerID string) error {
	// Get all subscriptions for the room
	allSubs, err := rb.subRepo.GetByRoomID(ctx, roomID, false)
	if err != nil {
		return fmt.Errorf("failed to get subscriptions: %w", err)
	}

	// Create a map of profile IDs to remove
	removeMap := make(map[string]bool)
	for _, id := range profileIDs {
		removeMap[id] = true
	}

	// Deactivate subscriptions
	for _, sub := range allSubs {
		if removeMap[sub.ProfileID] {
			sub.IsActive = false
			if err := rb.subRepo.Save(ctx, sub); err != nil {
				return fmt.Errorf("failed to deactivate subscription: %w", err)
			}

			// Send member removed event using MessageBusiness
			_ = rb.sendRoomEvent(ctx, roomID, removerID, chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
				map[string]interface{}{"content": "Member removed from room"},
				map[string]interface{}{
					"profile_id": sub.ProfileID,
					"removed_by": removerID,
				})
		}
	}

	return nil
}

// Helper function to send room event using MessageBusiness
func (rb *roomBusiness) sendRoomEvent(ctx context.Context, roomID string, senderID string, eventType chatv1.RoomEventType, content map[string]interface{}, metadata map[string]interface{}) error {
	// Merge content and metadata into a single payload
	combinedPayload := make(map[string]interface{})
	for k, v := range content {
		combinedPayload[k] = v
	}
	for k, v := range metadata {
		combinedPayload[k] = v
	}

	payload, err := structpb.NewStruct(combinedPayload)
	if err != nil {
		return fmt.Errorf("failed to create payload: %w", err)
	}

	req := &chatv1.SendMessageRequest{
		Message: []*chatv1.RoomEvent{
			{
				RoomId:   roomID,
				SenderId: senderID,
				Type:     eventType,
				Payload:  payload,
			},
		},
	}

	_, err = rb.messageBusiness.SendMessage(ctx, req, senderID)
	return err
}

// Helper function to remove duplicates and exclude a specific value from a string slice
func removeDuplicateAndExclude(slice []string, exclude string) []string {
	keys := make(map[string]bool)
	result := []string{}

	for _, entry := range slice {
		if entry != exclude && !keys[entry] {
			keys[entry] = true
			result = append(result, entry)
		}
	}

	return result
}
