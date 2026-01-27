package queues_test

import (
	"context"
	"testing"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/gateway/config"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/apps/gateway/service/queues"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GatewayDeliveryHandlerTestSuite struct {
	suite.Suite
	cfg *config.GatewayConfig
}

func TestGatewayDeliveryHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(GatewayDeliveryHandlerTestSuite))
}

func (s *GatewayDeliveryHandlerTestSuite) SetupTest() {
	s.cfg = &config.GatewayConfig{
		QueueOfflineEventDeliveryName: "offline.delivery",
	}
}

func (s *GatewayDeliveryHandlerTestSuite) TestHandle_ValidDelivery_DispatchesToConnection() {
	profileID := "user123"
	deviceID := "device456"

	// Create mock connection manager that returns a connection
	mockConn := newMockConnection(profileID, deviceID)
	mockCM := &mockConnectionManager{
		connections: map[string]business.Connection{
			internal.MetadataKey(profileID, deviceID): mockConn,
		},
	}

	handler := queues.NewGatewayEventsQueueHandler(s.cfg, nil, mockCM)

	// Create a valid delivery payload
	delivery := s.createTextDelivery("Hello, World!")
	payload, err := proto.Marshal(delivery)
	require.NoError(s.T(), err)

	headers := map[string]string{
		internal.HeaderProfileID: profileID,
		internal.HeaderDeviceID:  deviceID,
	}

	// Handle the message
	err = handler.Handle(context.Background(), headers, payload)
	require.NoError(s.T(), err)

	// Verify message was dispatched
	assert.Equal(s.T(), 1, mockConn.dispatchCount)
}

func (s *GatewayDeliveryHandlerTestSuite) TestHandle_ConnectionNotFound_ReturnsNil() {
	mockCM := &mockConnectionManager{
		connections: map[string]business.Connection{}, // Empty - no connections
	}

	handler := queues.NewGatewayEventsQueueHandler(s.cfg, nil, mockCM)

	delivery := s.createTextDelivery("Hello")
	payload, err := proto.Marshal(delivery)
	require.NoError(s.T(), err)

	headers := map[string]string{
		internal.HeaderProfileID: "user123",
		internal.HeaderDeviceID:  "device456",
	}

	// Should return nil (message consumed) when connection not found
	err = handler.Handle(context.Background(), headers, payload)
	assert.NoError(s.T(), err)
}

func (s *GatewayDeliveryHandlerTestSuite) TestHandle_MalformedPayload_ReturnsNil() {
	mockCM := &mockConnectionManager{
		connections: map[string]business.Connection{},
	}

	handler := queues.NewGatewayEventsQueueHandler(s.cfg, nil, mockCM)

	headers := map[string]string{
		internal.HeaderProfileID: "user123",
		internal.HeaderDeviceID:  "device456",
	}

	// Send invalid protobuf data
	err := handler.Handle(context.Background(), headers, []byte("invalid protobuf data"))
	assert.NoError(s.T(), err) // Should consume message even on parse error
}

func (s *GatewayDeliveryHandlerTestSuite) TestHandle_DispatchChannelFull_PublishesToOfflineQueue() {
	profileID := "user123"
	deviceID := "device456"

	// Create mock connection that simulates full channel
	mockConn := &mockConnection{
		metadata: &business.Metadata{
			ProfileID: profileID,
			DeviceID:  deviceID,
		},
		dispatchSuccess: false, // Simulate full channel
	}
	mockCM := &mockConnectionManager{
		connections: map[string]business.Connection{
			internal.MetadataKey(profileID, deviceID): mockConn,
		},
	}

	// Create mock queue manager with offline publisher
	mockPub := &mockPublisher{}
	mockQueueManager := &mockQueueManager{
		publishers: map[string]queue.Publisher{
			s.cfg.QueueOfflineEventDeliveryName: mockPub,
		},
	}

	handler := queues.NewGatewayEventsQueueHandler(s.cfg, mockQueueManager, mockCM)

	delivery := s.createTextDelivery("Hello")
	payload, err := proto.Marshal(delivery)
	require.NoError(s.T(), err)

	headers := map[string]string{
		internal.HeaderProfileID: profileID,
		internal.HeaderDeviceID:  deviceID,
	}

	err = handler.Handle(context.Background(), headers, payload)
	assert.NoError(s.T(), err)

	// Verify message was published to offline queue
	assert.Equal(s.T(), 1, mockPub.publishCount)
}

func (s *GatewayDeliveryHandlerTestSuite) TestHandle_AllPayloadTypes() {
	profileID := "user123"
	deviceID := "device456"

	mockConn := newMockConnection(profileID, deviceID)
	mockCM := &mockConnectionManager{
		connections: map[string]business.Connection{
			internal.MetadataKey(profileID, deviceID): mockConn,
		},
	}

	handler := queues.NewGatewayEventsQueueHandler(s.cfg, nil, mockCM)

	headers := map[string]string{
		internal.HeaderProfileID: profileID,
		internal.HeaderDeviceID:  deviceID,
	}

	// Test different payload types
	testCases := []struct {
		name    string
		payload *chatv1.Payload
	}{
		{
			name: "TextContent",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "Hello"}},
			},
		},
		{
			name: "AttachmentContent",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
				Data: &chatv1.Payload_Attachment{Attachment: &chatv1.AttachmentContent{
					Uri:      "https://example.com/file.jpg",
					MimeType: "image/jpeg",
				}},
			},
		},
		{
			name: "ReactionContent",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_REACTION,
				Data: &chatv1.Payload_Reaction{Reaction: &chatv1.ReactionContent{
					TargetEventId: "event123",
					Reaction:      "thumbs_up",
				}},
			},
		},
		{
			name: "CallContent",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_CALL,
				Data: &chatv1.Payload_Call{Call: &chatv1.CallContent{
					Type: chatv1.CallContent_CALL_TYPE_UNSPECIFIED,
				}},
			},
		},
	}

	initialDispatchCount := mockConn.dispatchCount

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			delivery := s.createDeliveryWithPayload(tc.payload)
			payload, err := proto.Marshal(delivery)
			require.NoError(s.T(), err)

			err = handler.Handle(context.Background(), headers, payload)
			assert.NoError(s.T(), err)
		})
	}

	// Verify all payload types were dispatched
	assert.Equal(s.T(), initialDispatchCount+len(testCases), mockConn.dispatchCount)
}

// Helper methods
func (s *GatewayDeliveryHandlerTestSuite) createTextDelivery(body string) *eventsv1.Delivery {
	return s.createDeliveryWithPayload(&chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
		Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: body}},
	})
}

func (s *GatewayDeliveryHandlerTestSuite) createDeliveryWithPayload(
	payload *chatv1.Payload,
) *eventsv1.Delivery {
	return &eventsv1.Delivery{
		Event: &eventsv1.Link{
			EventId:   "event123",
			RoomId:    "room456",
			EventType: chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			CreatedAt: timestamppb.Now(),
		},
		Destination: &eventsv1.Subscription{
			SubscriptionId: "sub123",
		},
		Payload: payload,
	}
}

// Mock implementations

type mockConnection struct {
	metadata        *business.Metadata
	dispatchCount   int
	dispatchSuccess bool
	stream          business.DeviceStream
}

func newMockConnection(profileID, deviceID string) *mockConnection {
	return &mockConnection{
		metadata: &business.Metadata{
			ProfileID:     profileID,
			DeviceID:      deviceID,
			LastActive:    time.Now().Unix(),
			LastHeartbeat: time.Now().Unix(),
			Connected:     time.Now().Unix(),
			GatewayID:     "gateway-test",
		},
		dispatchSuccess: true,
	}
}

func (m *mockConnection) Lock()   {}
func (m *mockConnection) Unlock() {}

func (m *mockConnection) Metadata() *business.Metadata {
	return m.metadata
}

func (m *mockConnection) Dispatch(_ *chatv1.StreamResponse) bool {
	m.dispatchCount++
	return m.dispatchSuccess
}

func (m *mockConnection) ConsumeDispatch(_ context.Context) *chatv1.StreamResponse {
	return nil
}

func (m *mockConnection) Stream() business.DeviceStream {
	return m.stream
}

func (m *mockConnection) AllowInbound() bool {
	return true
}

func (m *mockConnection) Close() {}

type mockConnectionManager struct {
	connections map[string]business.Connection
}

func (m *mockConnectionManager) HandleConnection(
	_ context.Context,
	_ string,
	_ string,
	_ business.DeviceStream,
) error {
	return nil
}

func (m *mockConnectionManager) GetConnection(
	_ context.Context,
	profileID string,
	deviceID string,
) (business.Connection, bool) {
	key := internal.MetadataKey(profileID, deviceID)
	conn, ok := m.connections[key]
	return conn, ok
}

type mockQueueManager struct {
	publishers map[string]queue.Publisher
}

func (m *mockQueueManager) AddPublisher(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockQueueManager) GetPublisher(reference string) (queue.Publisher, error) {
	pub, ok := m.publishers[reference]
	if !ok {
		return nil, nil
	}
	return pub, nil
}

func (m *mockQueueManager) DiscardPublisher(_ context.Context, _ string) error {
	return nil
}

func (m *mockQueueManager) AddSubscriber(
	_ context.Context,
	_ string,
	_ string,
	_ ...queue.SubscribeWorker,
) error {
	return nil
}

func (m *mockQueueManager) DiscardSubscriber(_ context.Context, _ string) error {
	return nil
}

func (m *mockQueueManager) GetSubscriber(_ string) (queue.Subscriber, error) {
	return nil, nil
}

func (m *mockQueueManager) Publish(_ context.Context, _ string, _ any, _ ...map[string]string) error {
	return nil
}

func (m *mockQueueManager) Init(_ context.Context) error {
	return nil
}

type mockPublisher struct {
	publishCount int
	publishErr   error
	lastMsg      any
	lastHeaders  []map[string]string
	initiated    bool
	ref          string
}

func (m *mockPublisher) Initiated() bool {
	return m.initiated
}

func (m *mockPublisher) Ref() string {
	return m.ref
}

func (m *mockPublisher) Init(_ context.Context) error {
	m.initiated = true
	return nil
}

func (m *mockPublisher) Publish(_ context.Context, msg any, headers ...map[string]string) error {
	m.publishCount++
	m.lastMsg = msg
	m.lastHeaders = headers
	return m.publishErr
}

func (m *mockPublisher) Stop(_ context.Context) error {
	return nil
}

func (m *mockPublisher) As(_ any) bool {
	return false
}
