package queues_test

import (
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
)

type HotPathDeliveryQueueHandlerTestSuite struct {
	tests.BaseTestSuite
}

func TestHotPathDeliveryQueueHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HotPathDeliveryQueueHandlerTestSuite))
}

func (s *HotPathDeliveryQueueHandlerTestSuite) createConfig() *config.ChatConfig {
	return &config.ChatConfig{
		QueueDeviceEventDeliveryName:  "gateway.event.delivery.%d",
		QueueOfflineEventDeliveryName: "offline.event.delivery",
		ShardCount:                    2,
	}
}

func (s *HotPathDeliveryQueueHandlerTestSuite) createDeliveryPayload(profileID string) []byte {
	delivery := &eventsv1.Delivery{
		Event: &eventsv1.Link{
			EventId: util.IDString(),
			RoomId:  util.IDString(),
		},
		Destination: &eventsv1.Subscription{
			SubscriptionId: util.IDString(),
			ContactLink: &commonv1.ContactLink{
				ProfileId: profileID,
				ContactId: util.IDString(),
			},
		},
		Payload: &chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
			Data: &chatv1.Payload_Text{
				Text: &chatv1.TextContent{Body: "Test message"},
			},
		},
	}

	data, _ := proto.Marshal(delivery)
	return data
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestHandle_ValidDelivery() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		cfg := s.createConfig()
		qMan := svc.QueueManager()
		workMan := svc.WorkManager()
		deviceCli := s.GetDevice(t)

		handler := queues.NewHotPathDeliveryQueueHandler(cfg, qMan, workMan, deviceCli)

		profileID := util.IDString()
		payload := s.createDeliveryPayload(profileID)

		// Handler should process without error (even if device service returns error)
		err := handler.Handle(ctx, nil, payload)
		// The handler returns error from device service as expected in mock setup
		require.Error(t, err) // Expected: device service mock returns "no devices found"
	})
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestHandle_MalformedPayload() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		cfg := s.createConfig()
		qMan := svc.QueueManager()
		workMan := svc.WorkManager()
		deviceCli := s.GetDevice(t)

		handler := queues.NewHotPathDeliveryQueueHandler(cfg, qMan, workMan, deviceCli)

		// Send invalid protobuf data
		invalidPayload := []byte("not a valid protobuf")
		err := handler.Handle(ctx, nil, invalidPayload)

		require.Error(t, err)
	})
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestHandle_EmptyPayload() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		cfg := s.createConfig()
		qMan := svc.QueueManager()
		workMan := svc.WorkManager()
		deviceCli := s.GetDevice(t)

		handler := queues.NewHotPathDeliveryQueueHandler(cfg, qMan, workMan, deviceCli)

		// Send empty payload
		err := handler.Handle(ctx, nil, []byte{})

		// Empty payload unmarshals to empty Delivery, which has empty profile ID
		// Device search will still be called with empty query
		require.Error(t, err) // Device service mock returns error
	})
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestHandle_DeliveryWithoutDestination() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		cfg := s.createConfig()
		qMan := svc.QueueManager()
		workMan := svc.WorkManager()
		deviceCli := s.GetDevice(t)

		handler := queues.NewHotPathDeliveryQueueHandler(cfg, qMan, workMan, deviceCli)

		// Create delivery without destination
		delivery := &eventsv1.Delivery{
			Event: &eventsv1.Link{
				EventId: util.IDString(),
				RoomId:  util.IDString(),
			},
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{
					Text: &chatv1.TextContent{Body: "Test message"},
				},
			},
		}
		data, _ := proto.Marshal(delivery)

		err := handler.Handle(ctx, nil, data)
		// Should still call device service with empty profile ID
		require.Error(t, err)
	})
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestDeliveryMessageSerialization() {
	// Test that delivery message serializes and deserializes correctly
	profileID := util.IDString()
	deviceID := util.IDString()

	original := &eventsv1.Delivery{
		DeviceId: deviceID,
		Event: &eventsv1.Link{
			EventId: util.IDString(),
			RoomId:  util.IDString(),
		},
		Destination: &eventsv1.Subscription{
			SubscriptionId: util.IDString(),
			ContactLink: &commonv1.ContactLink{
				ProfileId: profileID,
				ContactId: util.IDString(),
			},
		},
		Payload: &chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
			Data: &chatv1.Payload_Text{
				Text: &chatv1.TextContent{Body: "Hello, World!"},
			},
		},
		RetryCount: 3,
	}

	data, err := proto.Marshal(original)
	require.NoError(s.T(), err)

	parsed := &eventsv1.Delivery{}
	err = proto.Unmarshal(data, parsed)
	require.NoError(s.T(), err)

	s.Equal(original.GetDeviceId(), parsed.GetDeviceId())
	s.Equal(original.GetRetryCount(), parsed.GetRetryCount())
	s.Equal(original.GetDestination().GetContactLink().GetProfileId(), parsed.GetDestination().GetContactLink().GetProfileId())
	s.Equal(original.GetPayload().GetText().GetBody(), parsed.GetPayload().GetText().GetBody())
}

func (s *HotPathDeliveryQueueHandlerTestSuite) TestDeliveryPayloadTypes() {
	testCases := []struct {
		name    string
		payload *chatv1.Payload
	}{
		{
			name: "text payload",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{
					Text: &chatv1.TextContent{Body: "Test message"},
				},
			},
		},
		{
			name: "attachment payload",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
				Data: &chatv1.Payload_Attachment{
					Attachment: &chatv1.AttachmentContent{
						AttachmentId: util.IDString(),
						Uri:          "https://example.com/file.pdf",
						MimeType:     "application/pdf",
					},
				},
			},
		},
		{
			name: "reaction payload",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_REACTION,
				Data: &chatv1.Payload_Reaction{
					Reaction: &chatv1.ReactionContent{
						TargetEventId: util.IDString(),
						Reaction:      "👍",
						Add:           true,
					},
				},
			},
		},
		{
			name: "encrypted payload",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED,
				Data: &chatv1.Payload_Encrypted{
					Encrypted: &chatv1.EncryptedContent{
						Algorithm:  "AES-256-GCM",
						Ciphertext: []byte("encrypted data"),
					},
				},
			},
		},
		{
			name: "call payload",
			payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_CALL,
				Data: &chatv1.Payload_Call{
					Call: &chatv1.CallContent{
						CallId: util.IDString(),
						Type:   chatv1.CallContent_CALL_TYPE_UNSPECIFIED,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			delivery := &eventsv1.Delivery{
				Event: &eventsv1.Link{
					EventId: util.IDString(),
					RoomId:  util.IDString(),
				},
				Destination: &eventsv1.Subscription{
					ContactLink: &commonv1.ContactLink{
						ProfileId: util.IDString(),
					},
				},
				Payload: tc.payload,
			}

			data, err := proto.Marshal(delivery)
			require.NoError(s.T(), err)

			parsed := &eventsv1.Delivery{}
			err = proto.Unmarshal(data, parsed)
			require.NoError(s.T(), err)

			s.Equal(tc.payload.GetType(), parsed.GetPayload().GetType())
		})
	}
}
