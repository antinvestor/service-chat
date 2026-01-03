package models

import (
	"testing"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPayloadConverter_TextContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Text Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "event123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT),
			Content: data.JSONMap{
				"text": "Hello, world!",
			},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)

		assert.Equal(t, event.ID, protoEvent.GetId())
		assert.Equal(t, event.RoomID, protoEvent.GetRoomId())
		assert.Equal(t, event.SenderID, protoEvent.GetSenderId())
		assert.Equal(t, chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT, protoEvent.GetType())

		text := protoEvent.GetText()
		require.NotNil(t, text)
		assert.Equal(t, "Hello, world!", text.GetBody())
	})

	t.Run("FromProto - Text Content", func(t *testing.T) {
		protoEvent := &chatv1.RoomEvent{
			Id:       "event123",
			RoomId:   "room456",
			SenderId: "user789",
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetText(&chatv1.TextContent{
			Body: "Hello, world!",
		})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		require.NotNil(t, event)

		assert.Equal(t, protoEvent.GetId(), event.ID)
		assert.Equal(t, protoEvent.GetRoomId(), event.RoomID)
		assert.Equal(t, protoEvent.GetSenderId(), event.SenderID)

		require.NotNil(t, event.Content)
		assert.Equal(t, "Hello, world!", event.Content["text"])
	})

	t.Run("Round Trip - Text Content", func(t *testing.T) {
		original := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "event123",
				CreatedAt: time.Now().Truncate(time.Second),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT),
			Content: data.JSONMap{
				"text":   "Hello, world!",
				"format": "<b>Hello, world!</b>",
			},
		}

		// Convert to proto
		protoEvent, err := converter.ToProtoRoomEvent(original)
		require.NoError(t, err)

		// Convert back to domain
		result, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)

		assert.Equal(t, original.ID, result.ID)
		assert.Equal(t, original.RoomID, result.RoomID)
		assert.Equal(t, original.SenderID, result.SenderID)
		assert.Equal(t, original.Content["text"], result.Content["text"])
	})

	t.Run("Validate Text Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"text": "Hello",
		}
		err := converter.ValidateTextContent(validContent)
		assert.NoError(t, err)

		invalidContent := data.JSONMap{
			"message": "Hello",
		}
		err = converter.ValidateTextContent(invalidContent)
		assert.Error(t, err)

		nilContent := data.JSONMap(nil)
		err = converter.ValidateTextContent(nilContent)
		assert.Error(t, err)
	})

	t.Run("Create Text Content", func(t *testing.T) {
		content := converter.CreateTextContent("Test message")
		require.NotNil(t, content)
		assert.Equal(t, "Test message", content["text"])
	})
}

func TestPayloadConverter_AttachmentContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Attachment Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "event123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT),
			Content: data.JSONMap{
				"attachmentId": "attach123",
				"fileName":     "document.pdf",
				"mimeType":     "application/pdf",
				"size":         int64(1024000),
			},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)

		assert.Equal(t, chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT, protoEvent.GetType())

		attachment := protoEvent.GetAttachment()
		require.NotNil(t, attachment)
		assert.Equal(t, "attach123", attachment.GetAttachmentId())
		assert.Equal(t, "document.pdf", attachment.GetFilename())
		assert.Equal(t, "application/pdf", attachment.GetMimeType())
		assert.Equal(t, int64(1024000), attachment.GetSizeBytes())
	})

	t.Run("FromProto - Attachment Content", func(t *testing.T) {
		protoEvent := &chatv1.RoomEvent{
			Id:       "event123",
			RoomId:   "room456",
			SenderId: "user789",
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetAttachment(&chatv1.AttachmentContent{
			AttachmentId: "attach123",
			Filename:     "photo.jpg",
			MimeType:     "image/jpeg",
			SizeBytes:    512000,
		})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		require.NotNil(t, event)

		require.NotNil(t, event.Content)
		assert.Equal(t, "attach123", event.Content["attachmentId"])
		assert.Equal(t, "photo.jpg", event.Content["fileName"])
		assert.Equal(t, "image/jpeg", event.Content["mimeType"])
		assert.Equal(t, int64(512000), event.Content["size"])
	})

	t.Run("Validate Attachment Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"attachmentId": "attach123",
		}
		err := converter.ValidateAttachmentContent(validContent)
		assert.NoError(t, err)

		invalidContent := data.JSONMap{
			"fileId": "attach123",
		}
		err = converter.ValidateAttachmentContent(invalidContent)
		assert.Error(t, err)
	})

	t.Run("Create Attachment Content", func(t *testing.T) {
		content := converter.CreateAttachmentContent("attach123", "file.txt", "text/plain", 2048)
		require.NotNil(t, content)
		assert.Equal(t, "attach123", content["attachmentId"])
		assert.Equal(t, "file.txt", content["fileName"])
		assert.Equal(t, "text/plain", content["mimeType"])
		assert.Equal(t, int64(2048), content["size"])
	})
}

func TestPayloadConverter_ReactionContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Reaction Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "reaction123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			ParentID:  "original_msg_123",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION),
			Content: data.JSONMap{
				"emoji": "👍",
			},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)

		assert.Equal(t, chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION, protoEvent.GetType())
		assert.NotNil(t, protoEvent.GetParentId())
		assert.Equal(t, "original_msg_123", protoEvent.GetParentId())

		reaction := protoEvent.GetReaction()
		require.NotNil(t, reaction)
		assert.Equal(t, "👍", reaction.GetReaction())
	})

	t.Run("FromProto - Reaction Content", func(t *testing.T) {
		parentID := "original_msg_123"
		protoEvent := &chatv1.RoomEvent{
			Id:       "reaction123",
			RoomId:   "room456",
			SenderId: "user789",
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION,
			ParentId: &parentID,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetReaction(&chatv1.ReactionContent{
			Reaction: "❤️",
			Add:      true,
		})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		require.NotNil(t, event)

		assert.Equal(t, "original_msg_123", event.ParentID)
		require.NotNil(t, event.Content)
		assert.Equal(t, "❤️", event.Content["emoji"])
	})

	t.Run("Validate Reaction Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"emoji": "😀",
		}
		err := converter.ValidateReactionContent(validContent)
		assert.NoError(t, err)

		invalidContent := data.JSONMap{
			"reaction": "😀",
		}
		err = converter.ValidateReactionContent(invalidContent)
		assert.Error(t, err)
	})

	t.Run("Create Reaction Content", func(t *testing.T) {
		content := converter.CreateReactionContent("🎉")
		require.NotNil(t, content)
		assert.Equal(t, "🎉", content["emoji"])
	})
}

func TestPayloadConverter_CallContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Call Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "call123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL),
			Content: data.JSONMap{
				"sdp": "v=0\r\no=- 123 456...",
			},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)

		assert.Equal(t, chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL, protoEvent.GetType())

		call := protoEvent.GetCall()
		require.NotNil(t, call)
		assert.NotNil(t, call.GetSdp())
		assert.Equal(t, "v=0\r\no=- 123 456...", call.GetSdp())
	})

	t.Run("FromProto - Call Content", func(t *testing.T) {
		sdp := "v=0\r\no=- 789..."
		protoEvent := &chatv1.RoomEvent{
			Id:       "call123",
			RoomId:   "room456",
			SenderId: "user789",
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetCall(&chatv1.CallContent{
			Sdp: &sdp,
		})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		require.NotNil(t, event)

		require.NotNil(t, event.Content)
		assert.Equal(t, "v=0\r\no=- 789...", event.Content["sdp"])
	})

	t.Run("Validate Call Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"callId": "call123",
		}
		err := converter.ValidateCallContent(validContent)
		assert.NoError(t, err)

		invalidContent := data.JSONMap{
			"sessionId": "call123",
		}
		err = converter.ValidateCallContent(invalidContent)
		assert.Error(t, err)
	})

	t.Run("Create Call Content", func(t *testing.T) {
		content := converter.CreateCallContent("call123", "audio", "invite")
		require.NotNil(t, content)
		assert.Equal(t, "call123", content["callId"])
		assert.Equal(t, "audio", content["callType"])
		assert.Equal(t, "invite", content["action"])
	})
}

func TestPayloadConverter_EncryptedContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Encrypted Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "event123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED),
			Content: data.JSONMap{
				"ciphertext": "encrypted_base64_data",
				"algorithm":  "m.olm.v1.curve25519-aes-sha2",
				"sessionId":  "session_abc",
			},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)

		assert.Equal(t, chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED, protoEvent.GetType())

		encrypted := protoEvent.GetEncrypted()
		require.NotNil(t, encrypted)
		assert.Equal(t, "encrypted_base64_data", string(encrypted.GetCiphertext()))
		assert.Equal(t, "m.olm.v1.curve25519-aes-sha2", encrypted.GetAlgorithm())
		assert.NotNil(t, encrypted.GetSessionId())
		assert.Equal(t, "session_abc", encrypted.GetSessionId())
	})

	t.Run("FromProto - Encrypted Content", func(t *testing.T) {
		sessionID := "sess456"
		protoEvent := &chatv1.RoomEvent{
			Id:       "event123",
			RoomId:   "room456",
			SenderId: "user789",
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetEncrypted(&chatv1.EncryptedContent{
			Ciphertext: []byte("encrypted_data"),
			Algorithm:  "m.megolm.v1.aes-sha2",
			SessionId:  &sessionID,
		})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		require.NotNil(t, event)

		require.NotNil(t, event.Content)
		assert.Equal(t, "encrypted_data", event.Content["ciphertext"])
		assert.Equal(t, "m.megolm.v1.aes-sha2", event.Content["algorithm"])
		assert.Equal(t, "sess456", event.Content["sessionId"])
	})
}

func TestPayloadConverter_EdgeCases(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("Nil Event", func(t *testing.T) {
		_, err := converter.ToProtoRoomEvent(nil)
		assert.Error(t, err)

		_, err = converter.FromProtoRoomEvent(nil)
		assert.Error(t, err)
	})

	t.Run("Empty Content", func(t *testing.T) {
		event := &RoomEvent{
			BaseModel: data.BaseModel{
				ID:        "event123",
				CreatedAt: time.Now(),
			},
			RoomID:    "room456",
			SenderID:  "user789",
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT),
			Content:   data.JSONMap{},
		}

		protoEvent, err := converter.ToProtoRoomEvent(event)
		require.NoError(t, err)
		require.NotNil(t, protoEvent)
	})

	t.Run("Missing SenderId in Proto", func(t *testing.T) {
		protoEvent := &chatv1.RoomEvent{
			Id:       "event123",
			RoomId:   "room456",
			SenderId: "", // Empty sender ID
			Type:     chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT,
			SentAt:   timestamppb.Now(),
		}
		protoEvent.SetText(&chatv1.TextContent{Body: "Hello"})

		event, err := converter.FromProtoRoomEvent(protoEvent)
		require.NoError(t, err)
		assert.Empty(t, event.SenderID)
	})
}

func TestPayloadConverter_JSON(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ContentToJSON", func(t *testing.T) {
		content := data.JSONMap{
			"text":    "Hello",
			"counter": 42,
		}

		jsonStr, err := converter.ContentToJSON(content)
		require.NoError(t, err)
		assert.Contains(t, jsonStr, "Hello")
		assert.Contains(t, jsonStr, "42")
	})

	t.Run("ContentFromJSON", func(t *testing.T) {
		jsonStr := `{"text":"Hello","counter":42}`

		content, err := converter.ContentFromJSON(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, "Hello", content["text"])
		assert.Equal(t, float64(42), content["counter"])
	})

	t.Run("Round Trip JSON", func(t *testing.T) {
		original := data.JSONMap{
			"text":   "Test message",
			"number": 123,
			"bool":   true,
		}

		jsonStr, err := converter.ContentToJSON(original)
		require.NoError(t, err)

		result, err := converter.ContentFromJSON(jsonStr)
		require.NoError(t, err)

		assert.Equal(t, original["text"], result["text"])
		assert.Equal(t, float64(123), result["number"])
		assert.Equal(t, true, result["bool"])
	})

	t.Run("Empty JSON", func(t *testing.T) {
		content, err := converter.ContentFromJSON("")
		require.NoError(t, err)
		assert.Empty(t, content)

		jsonStr, err := converter.ContentToJSON(nil)
		require.NoError(t, err)
		assert.JSONEq(t, "{}", jsonStr)
	})
}
