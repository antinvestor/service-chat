package queues_test

import (
	"context"
	"errors"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	devicemocks "github.com/antinvestor/apis/go/device/mocks"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/gojuno/minimock/v3"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
)

type OfflineDeliveryQueueHandlerTestSuite struct {
	suite.Suite
	ctrl *minimock.Controller
}

func TestOfflineDeliveryQueueHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(OfflineDeliveryQueueHandlerTestSuite))
}

func (s *OfflineDeliveryQueueHandlerTestSuite) SetupTest() {
	s.ctrl = minimock.NewController(s.T())
}

func (s *OfflineDeliveryQueueHandlerTestSuite) createConfig() *config.ChatConfig {
	return &config.ChatConfig{
		QueueOfflineEventDeliveryName: "offline.event.delivery",
	}
}

func (s *OfflineDeliveryQueueHandlerTestSuite) createDeliveryWithPayload(
	profileID, deviceID string, payload *chatv1.Payload,
) []byte {
	delivery := &eventsv1.Delivery{
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
		Payload: payload,
	}

	data, err := proto.Marshal(delivery)
	s.Require().NoError(err)
	return data
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_TextMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	// Create mock device client
	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		// Verify notification content
		require.Equal(t, deviceID, req.Msg.GetDeviceId())
		require.Len(t, req.Msg.GetNotifications(), 1)
		require.Equal(t, "Hello, World!", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
		Data: &chatv1.Payload_Text{
			Text: &chatv1.TextContent{Body: "Hello, World!"},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_AttachmentMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		// Attachment with caption should use caption text
		require.Equal(t, "Check out this photo", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
		Data: &chatv1.Payload_Attachment{
			Attachment: &chatv1.AttachmentContent{
				AttachmentId: util.IDString(),
				Uri:          "https://example.com/photo.jpg",
				MimeType:     "image/jpeg",
				Caption:      &chatv1.TextContent{Body: "Check out this photo"},
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_AttachmentWithoutCaption() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		// Attachment without caption should have generic message
		require.Equal(t, "Sent an attachment", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
		Data: &chatv1.Payload_Attachment{
			Attachment: &chatv1.AttachmentContent{
				AttachmentId: util.IDString(),
				Uri:          "https://example.com/file.pdf",
				MimeType:     "application/pdf",
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_ReactionMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "Reacted with 👍", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_REACTION,
		Data: &chatv1.Payload_Reaction{
			Reaction: &chatv1.ReactionContent{
				TargetEventId: util.IDString(),
				Reaction:      "👍",
				Add:           true,
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_CallMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "Started a call", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_CALL,
		Data: &chatv1.Payload_Call{
			Call: &chatv1.CallContent{
				CallId: util.IDString(),
				Type:   chatv1.CallContent_CALL_TYPE_UNSPECIFIED,
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_EncryptedMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "Sent an encrypted message", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED,
		Data: &chatv1.Payload_Encrypted{
			Encrypted: &chatv1.EncryptedContent{
				Algorithm:  "AES-256-GCM",
				Ciphertext: []byte("encrypted data"),
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_ModerationMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "This message was removed for violating community guidelines", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_MODERATION,
		Data: &chatv1.Payload_Moderation{
			Moderation: &chatv1.ModerationContent{
				Body: "This message was removed for violating community guidelines",
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_MotionMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "Created a motion", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_MOTION,
		Data: &chatv1.Payload_Motion{
			Motion: &chatv1.MotionContent{
				Id:    util.IDString(),
				Title: "Approve budget",
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_VoteMessage() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		require.Equal(t, "Voted", req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_VOTE,
		Data: &chatv1.Payload_Vote{
			Vote: &chatv1.VoteCast{
				MotionId: util.IDString(),
			},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_NotificationServiceFailure() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Return(nil, errors.New("notification service unavailable"))

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
		Data: &chatv1.Payload_Text{
			Text: &chatv1.TextContent{Body: "Test message"},
		},
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "notification service unavailable")
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_EmptyProfileID() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	// Notify should NOT be called when profile ID is empty

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	// Create delivery with empty profile ID
	delivery := &eventsv1.Delivery{
		DeviceId: deviceID,
		Event: &eventsv1.Link{
			EventId: util.IDString(),
			RoomId:  util.IDString(),
		},
		Destination: &eventsv1.Subscription{
			SubscriptionId: util.IDString(),
			ContactLink: &commonv1.ContactLink{
				ProfileId: "", // Empty profile ID
				ContactId: util.IDString(),
			},
		},
		Payload: &chatv1.Payload{
			Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
			Data: &chatv1.Payload_Text{
				Text: &chatv1.TextContent{Body: "Test"},
			},
		},
	}
	data, marshalErr := proto.Marshal(delivery)
	require.NoError(t, marshalErr)

	err := handler.Handle(ctx, nil, data)
	// Should succeed but skip notification
	require.NoError(t, err)
	// Verify Notify was never called
	require.Equal(t, uint64(0), deviceCli.NotifyAfterCounter())
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_MalformedPayload() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	invalidPayload := []byte("not a valid protobuf")
	err := handler.Handle(ctx, nil, invalidPayload)

	require.Error(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_NilPayload() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		// With nil payload, body should be empty
		require.Empty(t, req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	// Create delivery with nil payload
	delivery := &eventsv1.Delivery{
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
		Payload: nil,
	}
	data, marshalErr := proto.Marshal(delivery)
	require.NoError(t, marshalErr)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}

func (s *OfflineDeliveryQueueHandlerTestSuite) TestHandle_UnspecifiedPayloadType() {
	t := s.T()
	ctx := context.Background()

	cfg := s.createConfig()
	profileID := util.IDString()
	deviceID := util.IDString()

	deviceCli := devicemocks.NewDeviceServiceClientMock(s.ctrl)
	deviceCli.NotifyMock.Set(func(
		_ context.Context, req *connect.Request[devicev1.NotifyRequest],
	) (*connect.Response[devicev1.NotifyResponse], error) {
		// Unspecified type should have empty body
		require.Empty(t, req.Msg.GetNotifications()[0].GetBody())
		return connect.NewResponse(&devicev1.NotifyResponse{}), nil
	})

	handler := queues.NewOfflineDeliveryQueueHandler(cfg, deviceCli)

	payload := &chatv1.Payload{
		Type: chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED,
	}
	data := s.createDeliveryWithPayload(profileID, deviceID, payload)

	err := handler.Handle(ctx, nil, data)
	require.NoError(t, err)
}
