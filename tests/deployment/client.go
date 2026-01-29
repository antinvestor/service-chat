package deployment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	chatv1connect "buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	profilev1connect "buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	profilev1 "buf.build/gen/go/antinvestor/profile/protocolbuffers/go/profile/v1"
	"connectrpc.com/connect"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/structpb"
)

// generateID generates a unique ID that matches the validation pattern [0-9a-z_-]{3,40}.
func generateID() string {
	return util.IDString()
}

// TestEmails is a fixed list of 30 test emails to avoid creating infinite profiles.
// These emails are reused across test runs.
var TestEmails = []string{
	"alice.test@integration-test.local",
	"bob.test@integration-test.local",
	"charlie.test@integration-test.local",
	"diana.test@integration-test.local",
	"edward.test@integration-test.local",
	"fiona.test@integration-test.local",
	"george.test@integration-test.local",
	"hannah.test@integration-test.local",
	"ivan.test@integration-test.local",
	"julia.test@integration-test.local",
	"kevin.test@integration-test.local",
	"laura.test@integration-test.local",
	"michael.test@integration-test.local",
	"nancy.test@integration-test.local",
	"oliver.test@integration-test.local",
	"patricia.test@integration-test.local",
	"quinn.test@integration-test.local",
	"rachel.test@integration-test.local",
	"samuel.test@integration-test.local",
	"tina.test@integration-test.local",
	"ulysses.test@integration-test.local",
	"victoria.test@integration-test.local",
	"walter.test@integration-test.local",
	"xena.test@integration-test.local",
	"yolanda.test@integration-test.local",
	"zachary.test@integration-test.local",
	"admin.test@integration-test.local",
	"moderator.test@integration-test.local",
	"support.test@integration-test.local",
	"system.test@integration-test.local",
}

// TestNames maps to TestEmails for display names.
var TestNames = []string{
	"Alice Test", "Bob Test", "Charlie Test", "Diana Test", "Edward Test",
	"Fiona Test", "George Test", "Hannah Test", "Ivan Test", "Julia Test",
	"Kevin Test", "Laura Test", "Michael Test", "Nancy Test", "Oliver Test",
	"Patricia Test", "Quinn Test", "Rachel Test", "Samuel Test", "Tina Test",
	"Ulysses Test", "Victoria Test", "Walter Test", "Xena Test", "Yolanda Test",
	"Zachary Test", "Admin Test", "Moderator Test", "Support Test", "System Test",
}

// TestProfile represents a profile created for testing.
type TestProfile struct {
	ProfileID string
	ContactID string
	Email     string
	Name      string
}

// Client wraps the chat service client with test utilities.
type Client struct {
	config  *Config
	chat    chatv1connect.ChatServiceClient
	gateway chatv1connect.GatewayServiceClient
	profile profilev1connect.ProfileServiceClient
	http    *http.Client
	created *TestResources

	// Cache for test profiles to avoid recreating them
	profileCache     map[string]*TestProfile
	profileCacheMu   sync.RWMutex
	testEmailIndex   int
	testEmailIndexMu sync.Mutex
}

// TestResources tracks resources created during tests for cleanup.
type TestResources struct {
	RoomIDs         []string
	SubscriptionIDs map[string][]string // roomID -> subscriptionIDs
	ProfileIDs      []string            // profiles created for testing
}

// NewTestResources creates a new resource tracker.
func NewTestResources() *TestResources {
	return &TestResources{
		RoomIDs:         make([]string, 0),
		SubscriptionIDs: make(map[string][]string),
		ProfileIDs:      make([]string, 0),
	}
}

// NewClient creates a new test client from configuration.
func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &AuthTransport{
			Token: cfg.AuthToken,
			Base:  http.DefaultTransport,
		},
	}

	chatClient := chatv1connect.NewChatServiceClient(
		httpClient,
		cfg.ServiceEndpoint,
	)

	gatewayClient := chatv1connect.NewGatewayServiceClient(
		httpClient,
		cfg.GetGatewayEndpoint(),
	)

	profileClient := profilev1connect.NewProfileServiceClient(
		httpClient,
		cfg.ProfileEndpoint,
	)

	return &Client{
		config:         cfg,
		chat:           chatClient,
		gateway:        gatewayClient,
		profile:        profileClient,
		http:           httpClient,
		created:        NewTestResources(),
		profileCache:   make(map[string]*TestProfile),
		testEmailIndex: 0,
	}, nil
}

// Chat returns the underlying chat service client.
func (c *Client) Chat() chatv1connect.ChatServiceClient {
	return c.chat
}

// Gateway returns the underlying gateway service client.
func (c *Client) Gateway() chatv1connect.GatewayServiceClient {
	return c.gateway
}

// Config returns the client configuration.
func (c *Client) Config() *Config {
	return c.config
}

// Created returns the resource tracker.
func (c *Client) Created() *TestResources {
	return c.created
}

// TrackRoom adds a room ID to the cleanup list.
func (c *Client) TrackRoom(roomID string) {
	c.created.RoomIDs = append(c.created.RoomIDs, roomID)
}

// TrackSubscription adds a subscription ID to the cleanup list.
func (c *Client) TrackSubscription(roomID, subscriptionID string) {
	c.created.SubscriptionIDs[roomID] = append(c.created.SubscriptionIDs[roomID], subscriptionID)
}

// Cleanup removes all tracked resources.
func (c *Client) Cleanup(ctx context.Context) []error {
	var errs []error

	// Delete rooms (which should cascade to subscriptions)
	for _, roomID := range c.created.RoomIDs {
		_, err := c.chat.DeleteRoom(ctx, connect.NewRequest(&chatv1.DeleteRoomRequest{
			RoomId: roomID,
		}))
		if err != nil {
			errs = append(errs, err)
		}
	}

	// Reset tracking
	c.created = NewTestResources()

	return errs
}

// Helper methods for common operations

// CreateTestRoom creates a room with test prefix and tracks it for cleanup.
func (c *Client) CreateTestRoom(
	ctx context.Context,
	name string,
	members []*commonv1.ContactLink,
) (*chatv1.Room, error) {
	fullName := c.config.TestDataPrefix + name
	roomID := generateID()

	resp, err := c.chat.CreateRoom(ctx, connect.NewRequest(&chatv1.CreateRoomRequest{
		Id:        roomID,
		Name:      fullName,
		IsPrivate: false,
		Members:   members,
	}))
	if err != nil {
		return nil, err
	}

	room := resp.Msg.GetRoom()
	if room != nil {
		c.TrackRoom(room.GetId())
	}

	return room, nil
}

// CreateTestRoomWithMetadata creates a room with metadata and tracks it for cleanup.
func (c *Client) CreateTestRoomWithMetadata(
	ctx context.Context,
	name, description string,
	isPrivate bool,
	metadata map[string]any,
) (*chatv1.Room, error) {
	fullName := c.config.TestDataPrefix + name
	roomID := generateID()

	var metaStruct *structpb.Struct
	if metadata != nil {
		var err error
		metaStruct, err = structpb.NewStruct(metadata)
		if err != nil {
			return nil, err
		}
	}

	resp, err := c.chat.CreateRoom(ctx, connect.NewRequest(&chatv1.CreateRoomRequest{
		Id:          roomID,
		Name:        fullName,
		Description: description,
		IsPrivate:   isPrivate,
		Metadata:    metaStruct,
	}))
	if err != nil {
		return nil, err
	}

	room := resp.Msg.GetRoom()
	if room != nil {
		c.TrackRoom(room.GetId())
	}

	return room, nil
}

// SendTextMessage sends a text message to a room.
func (c *Client) SendTextMessage(ctx context.Context, roomID, text string) ([]*chatv1.AckEvent, error) {
	resp, err := c.chat.SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     generateID(),
			RoomId: roomID,
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{
					Text: &chatv1.TextContent{
						Body:   text,
						Format: "plain",
					},
				},
			},
		}},
	}))
	if err != nil {
		return nil, err
	}

	return resp.Msg.GetAck(), nil
}

// SendBatchMessages sends multiple messages in a single request.
func (c *Client) SendBatchMessages(ctx context.Context, roomID string, messages []string) ([]*chatv1.AckEvent, error) {
	events := make([]*chatv1.RoomEvent, 0, len(messages))
	for _, msg := range messages {
		events = append(events, &chatv1.RoomEvent{
			Id:     generateID(),
			RoomId: roomID,
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{
					Text: &chatv1.TextContent{
						Body:   msg,
						Format: "plain",
					},
				},
			},
		})
	}

	resp, err := c.chat.SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: events,
	}))
	if err != nil {
		return nil, err
	}

	return resp.Msg.GetAck(), nil
}

// GetHistory retrieves message history from a room.
func (c *Client) GetHistory(
	ctx context.Context,
	roomID string,
	limit int32,
	cursor string,
) (*chatv1.GetHistoryResponse, error) {
	resp, err := c.chat.GetHistory(ctx, connect.NewRequest(&chatv1.GetHistoryRequest{
		RoomId: roomID,
		Cursor: &commonv1.PageCursor{
			Limit: limit,
			Page:  cursor,
		},
	}))
	if err != nil {
		return nil, err
	}

	return resp.Msg, nil
}

// AddMembers adds members to a room.
func (c *Client) AddMembers(ctx context.Context, roomID string, members []*chatv1.RoomSubscription) error {
	_, err := c.chat.AddRoomSubscriptions(ctx, connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
		RoomId:  roomID,
		Members: members,
	}))
	return err
}

// RemoveMembers removes members from a room.
func (c *Client) RemoveMembers(ctx context.Context, roomID string, subscriptionIDs []string) error {
	_, err := c.chat.RemoveRoomSubscriptions(ctx, connect.NewRequest(&chatv1.RemoveRoomSubscriptionsRequest{
		RoomId:         roomID,
		SubscriptionId: subscriptionIDs,
	}))
	return err
}

// SearchSubscriptions retrieves room members.
func (c *Client) SearchSubscriptions(ctx context.Context, roomID string) ([]*chatv1.RoomSubscription, error) {
	resp, err := c.chat.SearchRoomSubscriptions(ctx, connect.NewRequest(&chatv1.SearchRoomSubscriptionsRequest{
		RoomId: roomID,
	}))
	if err != nil {
		return nil, err
	}

	return resp.Msg.GetMembers(), nil
}

// UpdateRoom updates room properties.
func (c *Client) UpdateRoom(ctx context.Context, roomID, name, topic string) (*chatv1.Room, error) {
	resp, err := c.chat.UpdateRoom(ctx, connect.NewRequest(&chatv1.UpdateRoomRequest{
		RoomId: roomID,
		Name:   name,
		Topic:  topic,
	}))
	if err != nil {
		return nil, err
	}

	return resp.Msg.GetRoom(), nil
}

// UpdateSubscriptionRole changes a member's role.
func (c *Client) UpdateSubscriptionRole(ctx context.Context, roomID, subscriptionID string, roles []string) error {
	_, err := c.chat.UpdateSubscriptionRole(ctx, connect.NewRequest(&chatv1.UpdateSubscriptionRoleRequest{
		RoomId:         roomID,
		SubscriptionId: subscriptionID,
		Roles:          roles,
	}))
	return err
}

// SendLiveUpdate sends real-time state updates (typing, read markers, etc.).
func (c *Client) SendLiveUpdate(ctx context.Context, commands []*chatv1.ClientCommand) error {
	_, err := c.chat.Live(ctx, connect.NewRequest(&chatv1.LiveRequest{
		ClientStates: commands,
	}))
	return err
}

// SendTypingIndicator sends a typing indicator.
func (c *Client) SendTypingIndicator(ctx context.Context, roomID, subscriptionID string, typing bool) error {
	return c.SendLiveUpdate(ctx, []*chatv1.ClientCommand{{
		State: &chatv1.ClientCommand_Typing{
			Typing: &chatv1.TypingEvent{
				RoomId:         roomID,
				SubscriptionId: subscriptionID,
				Typing:         typing,
			},
		},
	}})
}

// SendReadMarker sends a read marker.
func (c *Client) SendReadMarker(ctx context.Context, roomID, upToEventID, subscriptionID string) error {
	return c.SendLiveUpdate(ctx, []*chatv1.ClientCommand{{
		State: &chatv1.ClientCommand_ReadMarker{
			ReadMarker: &chatv1.ReadMarker{
				RoomId:         &roomID,
				UpToEventId:    upToEventID,
				SubscriptionId: subscriptionID,
			},
		},
	}})
}

// WithTimeout creates a context with the configured request timeout.
func (c *Client) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.config.RequestTimeout)
}

// WithLongTimeout creates a context with a longer timeout for slow operations.
func (c *Client) WithLongTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 2*time.Minute)
}

// Profile returns the underlying profile service client.
func (c *Client) Profile() profilev1connect.ProfileServiceClient {
	return c.profile
}

// GetOrCreateTestProfile creates a test profile with the given email and name, or returns a cached one.
func (c *Client) GetOrCreateTestProfile(ctx context.Context, email, name string) (*TestProfile, error) {
	// Check cache first
	c.profileCacheMu.RLock()
	if cached, ok := c.profileCache[email]; ok {
		c.profileCacheMu.RUnlock()
		return cached, nil
	}
	c.profileCacheMu.RUnlock()

	// Create new profile via profile service
	properties, err := structpb.NewStruct(map[string]any{
		"name":  name,
		"email": email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create properties struct: %w", err)
	}

	resp, err := c.profile.Create(ctx, connect.NewRequest(&profilev1.CreateRequest{
		Type:       profilev1.ProfileType_PERSON,
		Contact:    email,
		Properties: properties,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	profileData := resp.Msg.GetData()
	if profileData == nil {
		return nil, errors.New("profile creation returned nil data")
	}

	// Extract contact ID from the first contact if available
	contactID := ""
	if contacts := profileData.GetContacts(); len(contacts) > 0 {
		contactID = contacts[0].GetId()
	}

	profile := &TestProfile{
		ProfileID: profileData.GetId(),
		ContactID: contactID,
		Email:     email,
		Name:      name,
	}

	// Cache and track for cleanup
	c.profileCacheMu.Lock()
	c.profileCache[email] = profile
	c.profileCacheMu.Unlock()

	c.created.ProfileIDs = append(c.created.ProfileIDs, profile.ProfileID)

	return profile, nil
}

// GetTestProfiles creates multiple test profiles using the fixed email list and returns them.
// Uses round-robin selection from TestEmails to avoid creating infinite profiles.
func (c *Client) GetTestProfiles(ctx context.Context, count int) ([]*TestProfile, error) {
	profiles := make([]*TestProfile, 0, count)

	c.testEmailIndexMu.Lock()
	startIndex := c.testEmailIndex
	c.testEmailIndex = (c.testEmailIndex + count) % len(TestEmails)
	c.testEmailIndexMu.Unlock()

	for i := range count {
		idx := (startIndex + i) % len(TestEmails)
		email := TestEmails[idx]
		name := TestNames[idx]

		profile, err := c.GetOrCreateTestProfile(ctx, email, name)
		if err != nil {
			return profiles, err
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// GetTestProfileByIndex returns a test profile at a specific index from the fixed list.
func (c *Client) GetTestProfileByIndex(ctx context.Context, index int) (*TestProfile, error) {
	idx := index % len(TestEmails)
	return c.GetOrCreateTestProfile(ctx, TestEmails[idx], TestNames[idx])
}

// ToContactLinks converts TestProfiles to ContactLinks for use in room operations.
func (c *Client) ToContactLinks(profiles []*TestProfile) []*commonv1.ContactLink {
	links := make([]*commonv1.ContactLink, len(profiles))
	for i, p := range profiles {
		links[i] = &commonv1.ContactLink{
			ProfileId: p.ProfileID,
			ContactId: p.ContactID,
		}
	}
	return links
}

// ToRoomSubscriptions converts TestProfiles to RoomSubscriptions for adding members.
func (c *Client) ToRoomSubscriptions(profiles []*TestProfile) []*chatv1.RoomSubscription {
	subs := make([]*chatv1.RoomSubscription, len(profiles))
	for i, p := range profiles {
		subs[i] = &chatv1.RoomSubscription{
			Id: generateID(),
			Member: &commonv1.ContactLink{
				ProfileId: p.ProfileID,
				ContactId: p.ContactID,
			},
			Roles: []string{"member"},
		}
	}
	return subs
}

// GatewayStream represents an active gateway stream connection.
type GatewayStream struct {
	stream   *connect.BidiStreamForClient[chatv1.StreamRequest, chatv1.StreamResponse]
	messages chan *chatv1.StreamResponse
	errors   chan error
	done     chan struct{}
	closed   bool
	mu       sync.Mutex
}

// ConnectToGateway establishes a bidirectional stream connection to the gateway.
func (c *Client) ConnectToGateway(ctx context.Context) (*GatewayStream, error) {
	stream := c.gateway.Stream(ctx)

	gs := &GatewayStream{
		stream:   stream,
		messages: make(chan *chatv1.StreamResponse, 100),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
	}

	// Start receiving messages in background
	go gs.receiveLoop()

	return gs, nil
}

// SendHello sends the initial StreamHello message to establish the connection.
func (gs *GatewayStream) SendHello(resumeToken string, capabilities map[string]string) error {
	if capabilities == nil {
		capabilities = make(map[string]string)
	}

	return gs.stream.Send(&chatv1.StreamRequest{
		Payload: &chatv1.StreamRequest_Hello{
			Hello: &chatv1.StreamHello{
				ResumeToken:  resumeToken,
				Capabilities: capabilities,
			},
		},
	})
}

// SendCommand sends a client command through the stream.
func (gs *GatewayStream) SendCommand(command *chatv1.ClientCommand) error {
	return gs.stream.Send(&chatv1.StreamRequest{
		Payload: &chatv1.StreamRequest_Command{
			Command: command,
		},
	})
}

// Close closes the gateway stream.
func (gs *GatewayStream) Close() error {
	gs.mu.Lock()
	if gs.closed {
		gs.mu.Unlock()
		return nil
	}
	gs.closed = true
	gs.mu.Unlock()

	close(gs.done)
	return gs.stream.CloseRequest()
}

// Messages returns the channel for receiving stream responses.
func (gs *GatewayStream) Messages() <-chan *chatv1.StreamResponse {
	return gs.messages
}

// Errors returns the channel for receiving errors.
func (gs *GatewayStream) Errors() <-chan error {
	return gs.errors
}

// receiveLoop continuously receives messages from the stream.
func (gs *GatewayStream) receiveLoop() {
	defer close(gs.messages)
	defer close(gs.errors)

	for {
		select {
		case <-gs.done:
			return
		default:
			msg, err := gs.stream.Receive()
			if err != nil {
				gs.mu.Lock()
				closed := gs.closed
				gs.mu.Unlock()

				if !closed {
					select {
					case gs.errors <- err:
					default:
					}
				}
				return
			}

			select {
			case gs.messages <- msg:
			case <-gs.done:
				return
			}
		}
	}
}

// WaitForMessage waits for a message on the stream with timeout.
func (gs *GatewayStream) WaitForMessage(timeout time.Duration) (*chatv1.StreamResponse, error) {
	select {
	case msg := <-gs.messages:
		return msg, nil
	case err := <-gs.errors:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for message after %v", timeout)
	}
}

// WaitForRoomEvent waits for a specific room event type with timeout.
func (gs *GatewayStream) WaitForRoomEvent(timeout time.Duration, roomID string) (*chatv1.RoomEvent, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		msg, err := gs.WaitForMessage(remaining)
		if err != nil {
			return nil, err
		}

		if event := msg.GetMessage(); event != nil {
			if roomID == "" || event.GetRoomId() == roomID {
				return event, nil
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for room event in room %s", roomID)
}
