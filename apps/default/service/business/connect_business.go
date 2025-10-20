package business

import (
	"context"
	"fmt"
	"sync"
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Connection represents an active client connection.
type Connection struct {
	ProfileID  string
	DeviceID   string
	Stream     ConnectionStream
	RoomIDs    map[string]bool // Rooms the user is subscribed to
	LastActive time.Time
	mu         sync.RWMutex
}

type connectBusiness struct {
	service         *frame.Service
	subRepo         repository.RoomSubscriptionRepository
	eventRepo       repository.RoomEventRepository
	subscriptionSvc SubscriptionService

	// Connection management
	connections map[string]*Connection // key: profileID:deviceID
	connMu      sync.RWMutex

	// Room-based connection tracking for efficient broadcasting
	roomConnections map[string]map[string]*Connection // roomID -> (profileID:deviceID -> Connection)
	roomMu          sync.RWMutex
}

// NewConnectBusiness creates a new instance of ConnectBusiness.
func NewConnectBusiness(
	service *frame.Service,
	subRepo repository.RoomSubscriptionRepository,
	eventRepo repository.RoomEventRepository,
	subscriptionSvc SubscriptionService,
) ConnectBusiness {
	return &connectBusiness{
		service:         service,
		subRepo:         subRepo,
		eventRepo:       eventRepo,
		subscriptionSvc: subscriptionSvc,
		connections:     make(map[string]*Connection),
		roomConnections: make(map[string]map[string]*Connection),
	}
}

// HandleConnection manages a bidirectional streaming connection for real-time events.
func (cb *connectBusiness) HandleConnection(
	ctx context.Context,
	profileID string,
	deviceID string,
	stream ConnectionStream,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}

	// Create connection
	connKey := fmt.Sprintf("%s:%s", profileID, deviceID)
	conn := &Connection{
		ProfileID:  profileID,
		DeviceID:   deviceID,
		Stream:     stream,
		RoomIDs:    make(map[string]bool),
		LastActive: time.Now(),
	}

	// Register connection
	cb.connMu.Lock()
	cb.connections[connKey] = conn
	cb.connMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		cb.removeConnection(connKey)
	}()

	// Get user's subscribed rooms and register them
	roomIDs, err := cb.subscriptionSvc.GetSubscribedRoomIDs(ctx, profileID)
	if err != nil {
		return fmt.Errorf("failed to get subscribed rooms: %w", err)
	}

	// Register connection to all subscribed rooms
	for _, roomID := range roomIDs {
		cb.addConnectionToRoom(roomID, connKey, conn)
		conn.mu.Lock()
		conn.RoomIDs[roomID] = true
		conn.mu.Unlock()
	}

	// Send presence update (user is now online)
	for roomID := range conn.RoomIDs {
		_ = cb.SendPresenceUpdate(ctx, profileID, roomID, chatv1.PresenceStatus_PRESENCE_ONLINE)
	}

	// Handle incoming messages from client
	errChan := make(chan error, 1)
	go func() {
		for {
			req, err := stream.Receive()
			if err != nil {
				errChan <- err
				return
			}

			// Update last active time
			conn.mu.Lock()
			conn.LastActive = time.Now()
			conn.mu.Unlock()

			// Handle client commands
			if err := cb.handleClientCommand(ctx, conn, req); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		// Send presence update (user is now offline)
		for roomID := range conn.RoomIDs {
			_ = cb.SendPresenceUpdate(ctx, profileID, roomID, chatv1.PresenceStatus_PRESENCE_OFFLINE)
		}
		return ctx.Err()
	case err := <-errChan:
		// Send presence update (user is now offline)
		for roomID := range conn.RoomIDs {
			_ = cb.SendPresenceUpdate(ctx, profileID, roomID, chatv1.PresenceStatus_PRESENCE_OFFLINE)
		}
		return err
	}
}

// handleClientCommand processes commands from the client.
func (cb *connectBusiness) handleClientCommand(
	ctx context.Context,
	conn *Connection,
	req *chatv1.ConnectRequest,
) error {
	switch cmd := req.GetPayload().(type) {
	case *chatv1.ConnectRequest_Command:
		return cb.handleCommand(ctx, conn, cmd.Command)
	case *chatv1.ConnectRequest_Ack:
		// Client acknowledged a message - we can track delivery if needed
		return nil
	default:
		return nil
	}
}

// handleCommand processes specific client commands.
func (cb *connectBusiness) handleCommand(ctx context.Context, conn *Connection, cmd *chatv1.ClientCommand) error {
	switch payload := cmd.GetCmd().(type) {
	case *chatv1.ClientCommand_Typing:
		// Handle typing indicator
		return cb.SendTypingIndicator(ctx, conn.ProfileID, payload.Typing.GetRoomId(), payload.Typing.GetTyping())

	case *chatv1.ClientCommand_ReadMarker:
		// Handle read receipt - use event ID directly
		eventID := payload.ReadMarker.GetUpToEventId()
		if eventID == "" {
			return nil
		}
		return cb.SendReadReceipt(ctx, conn.ProfileID, payload.ReadMarker.GetRoomId(), eventID)

	default:
		return nil
	}
}

// ReceiveEvent receives an event from a connected user.
func (cb *connectBusiness) ReceiveEvent(ctx context.Context, event *chatv1.RoomEvent) (error, *chatv1.StreamAck) {
	return nil, nil
}

// BroadcastEvent sends an event to all subscribers of a room.
func (cb *connectBusiness) BroadcastEvent(ctx context.Context, roomID string, event *chatv1.ServerEvent) error {
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	cb.roomMu.RLock()
	connections, exists := cb.roomConnections[roomID]
	cb.roomMu.RUnlock()

	if !exists || len(connections) == 0 {
		// No active connections for this room
		return nil
	}

	// Send to all connections in the room
	var sendErrors []error
	for _, conn := range connections {
		if err := conn.Stream.Send(event); err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("failed to send to %s: %w", conn.ProfileID, err))
		}
	}

	if len(sendErrors) > 0 {
		return fmt.Errorf("failed to broadcast to some connections: %v", sendErrors)
	}

	return nil
}

// SendPresenceUpdate sends presence updates to room subscribers.
func (cb *connectBusiness) SendPresenceUpdate(
	ctx context.Context,
	profileID string,
	roomID string,
	status chatv1.PresenceStatus,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	event := &chatv1.ServerEvent{
		Payload: &chatv1.ServerEvent_PresenceEvent{
			PresenceEvent: &chatv1.PresenceEvent{
				ProfileId:  profileID,
				Status:     status,
				LastActive: timestamppb.Now(),
			},
		},
	}

	return cb.BroadcastEvent(ctx, roomID, event)
}

// SendTypingIndicator sends typing indicators to room subscribers.
func (cb *connectBusiness) SendTypingIndicator(
	ctx context.Context,
	profileID string,
	roomID string,
	isTyping bool,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	hasAccess, err := cb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}
	if !hasAccess {
		return service.ErrRoomAccessDenied
	}

	event := &chatv1.ServerEvent{
		Payload: &chatv1.ServerEvent_TypingEvent{
			TypingEvent: &chatv1.TypingEvent{
				ProfileId: profileID,
				RoomId:    roomID,
				Typing:    isTyping,
				Since:     timestamppb.Now(),
			},
		},
	}

	return cb.BroadcastEvent(ctx, roomID, event)
}

// SendReadReceipt sends read receipts to room subscribers.
func (cb *connectBusiness) SendReadReceipt(ctx context.Context, profileID string, roomID string, eventID string) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	hasAccess, err := cb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}
	if !hasAccess {
		return service.ErrRoomAccessDenied
	}

	// Update the subscription's last read event ID
	sub, err := cb.subRepo.GetByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update to the new event ID
	// UnreadCount is now a generated column and will be automatically calculated
	sub.LastReadEventID = eventID
	sub.LastReadAt = time.Now().Unix()

	if err := cb.subRepo.Save(ctx, sub); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Broadcast read receipt to other room members
	receiptEvent := &chatv1.ServerEvent{
		Payload: &chatv1.ServerEvent_ReceiptEvent{
			ReceiptEvent: &chatv1.ReceiptEvent{
				ProfileId: profileID,
				RoomId:    roomID,
				MessageId: eventID,
				ReadAt:    timestamppb.Now(),
			},
		},
	}

	return cb.BroadcastEvent(ctx, roomID, receiptEvent)
}

// Helper methods for connection management

func (cb *connectBusiness) removeConnection(connKey string) {
	cb.connMu.Lock()
	conn, exists := cb.connections[connKey]
	if exists {
		delete(cb.connections, connKey)
	}
	cb.connMu.Unlock()

	if !exists {
		return
	}

	// Remove from all rooms
	conn.mu.RLock()
	roomIDs := make([]string, 0, len(conn.RoomIDs))
	for roomID := range conn.RoomIDs {
		roomIDs = append(roomIDs, roomID)
	}
	conn.mu.RUnlock()

	for _, roomID := range roomIDs {
		cb.removeConnectionFromRoom(roomID, connKey)
	}
}

func (cb *connectBusiness) addConnectionToRoom(roomID string, connKey string, conn *Connection) {
	cb.roomMu.Lock()
	defer cb.roomMu.Unlock()

	if _, exists := cb.roomConnections[roomID]; !exists {
		cb.roomConnections[roomID] = make(map[string]*Connection)
	}
	cb.roomConnections[roomID][connKey] = conn
}

func (cb *connectBusiness) removeConnectionFromRoom(roomID string, connKey string) {
	cb.roomMu.Lock()
	defer cb.roomMu.Unlock()

	if connections, exists := cb.roomConnections[roomID]; exists {
		delete(connections, connKey)
		if len(connections) == 0 {
			delete(cb.roomConnections, roomID)
		}
	}
}
