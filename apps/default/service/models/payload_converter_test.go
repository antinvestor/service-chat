package models

import (
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayloadConverter_TextContent(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("ToProto - Text Content", func(t *testing.T) {
		content := data.JSONMap{
			"text": "Hello, world!",
		}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_TEXT, payload.GetType())

		text := payload.GetText()
		require.NotNil(t, text)
		assert.Equal(t, "Hello, world!", text.GetBody())
	})

	t.Run("FromProto - Text Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_TEXT)
		payload.SetText(&chatv1.TextContent{
			Body: "Hello, world!",
		})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, "Hello, world!", content["text"])
	})

	t.Run("Round Trip - Text Content", func(t *testing.T) {
		original := data.JSONMap{
			"text":   "Hello, world!",
			"format": "<b>Hello, world!</b>",
		}

		// Convert to proto
		payload, err := converter.ToProtoRoomEvent(original)
		require.NoError(t, err)

		// Convert back to domain
		result, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)

		assert.Equal(t, original["text"], result["text"])
		assert.Equal(t, original["format"], result["format"])
	})

	t.Run("Validate Text Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"text": "Hello",
		}
		err := converter.ValidateTextContent(validContent)
		require.NoError(t, err)

		invalidContent := data.JSONMap{
			"message": "Hello",
		}
		err = converter.ValidateTextContent(invalidContent)
		require.Error(t, err)

		nilContent := data.JSONMap(nil)
		err = converter.ValidateTextContent(nilContent)
		require.Error(t, err)
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
		content := data.JSONMap{
			"attachmentId": "attach123",
			"fileName":     "document.pdf",
			"mimeType":     "application/pdf",
			"size":         int64(1024000),
		}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT, payload.GetType())

		attachment := payload.GetAttachment()
		require.NotNil(t, attachment)
		assert.Equal(t, "attach123", attachment.GetAttachmentId())
		assert.Equal(t, "document.pdf", attachment.GetFilename())
		assert.Equal(t, "application/pdf", attachment.GetMimeType())
		assert.Equal(t, int64(1024000), attachment.GetSizeBytes())
	})

	t.Run("FromProto - Attachment Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT)
		payload.SetAttachment(&chatv1.AttachmentContent{
			AttachmentId: "attach123",
			Filename:     "photo.jpg",
			MimeType:     "image/jpeg",
			SizeBytes:    512000,
		})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, "attach123", content["attachmentId"])
		assert.Equal(t, "photo.jpg", content["fileName"])
		assert.Equal(t, "image/jpeg", content["mimeType"])
		assert.Equal(t, int64(512000), content["size"])
	})

	t.Run("Validate Attachment Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"attachmentId": "attach123",
		}
		err := converter.ValidateAttachmentContent(validContent)
		require.NoError(t, err)

		invalidContent := data.JSONMap{
			"fileId": "attach123",
		}
		err = converter.ValidateAttachmentContent(invalidContent)
		require.Error(t, err)
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
		content := data.JSONMap{
			"emoji": "👍",
		}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_REACTION, payload.GetType())

		reaction := payload.GetReaction()
		require.NotNil(t, reaction)
		assert.Equal(t, "👍", reaction.GetReaction())
	})

	t.Run("FromProto - Reaction Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_REACTION)
		payload.SetReaction(&chatv1.ReactionContent{
			Reaction: "❤️",
			Add:      true,
		})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, "❤️", content["emoji"])
	})

	t.Run("Validate Reaction Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"emoji": "😀",
		}
		err := converter.ValidateReactionContent(validContent)
		require.NoError(t, err)

		invalidContent := data.JSONMap{
			"reaction": "😀",
		}
		err = converter.ValidateReactionContent(invalidContent)
		require.Error(t, err)
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
		content := data.JSONMap{
			"sdp": "v=0\r\no=- 123 456...",
		}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_CALL, payload.GetType())

		call := payload.GetCall()
		require.NotNil(t, call)
		assert.NotNil(t, call.GetSdp())
		assert.Equal(t, "v=0\r\no=- 123 456...", call.GetSdp())
	})

	t.Run("FromProto - Call Content", func(t *testing.T) {
		sdp := "v=0\r\no=- 789..."
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_CALL)
		payload.SetCall(&chatv1.CallContent{
			Sdp: &sdp,
		})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, "v=0\r\no=- 789...", content["sdp"])
	})

	t.Run("Validate Call Content", func(t *testing.T) {
		validContent := data.JSONMap{
			"callId": "call123",
		}
		err := converter.ValidateCallContent(validContent)
		require.NoError(t, err)

		invalidContent := data.JSONMap{
			"sessionId": "call123",
		}
		err = converter.ValidateCallContent(invalidContent)
		require.Error(t, err)
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
		content := data.JSONMap{
			"ciphertext": "encrypted_base64_data",
			"algorithm":  "m.olm.v1.curve25519-aes-sha2",
			"sessionId":  "session_abc",
		}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED, payload.GetType())

		encrypted := payload.GetEncrypted()
		require.NotNil(t, encrypted)
		assert.Equal(t, "encrypted_base64_data", string(encrypted.GetCiphertext()))
		assert.Equal(t, "m.olm.v1.curve25519-aes-sha2", encrypted.GetAlgorithm())
		assert.NotNil(t, encrypted.GetSessionId())
		assert.Equal(t, "session_abc", encrypted.GetSessionId())
	})

	t.Run("FromProto - Encrypted Content", func(t *testing.T) {
		sessionID := "sess456"
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED)
		payload.SetEncrypted(&chatv1.EncryptedContent{
			Ciphertext: []byte("encrypted_data"),
			Algorithm:  "m.megolm.v1.aes-sha2",
			SessionId:  &sessionID,
		})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, "encrypted_data", content["ciphertext"])
		assert.Equal(t, "m.megolm.v1.aes-sha2", content["algorithm"])
		assert.Equal(t, "sess456", content["sessionId"])
	})
}

func TestPayloadConverter_EdgeCases(t *testing.T) {
	converter := NewPayloadConverter()

	t.Run("Nil Event", func(t *testing.T) {
		_, err := converter.ToProtoRoomEvent(nil)
		require.Error(t, err)

		_, err = converter.FromProtoRoomEvent(nil)
		require.Error(t, err)
	})

	t.Run("Empty Content", func(t *testing.T) {
		content := data.JSONMap{}

		payload, err := converter.ToProtoRoomEvent(content)
		require.NoError(t, err)
		require.NotNil(t, payload)
	})

	t.Run("Missing Content in Proto", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetText(&chatv1.TextContent{Body: "Hello"})

		content, err := converter.FromProtoRoomEvent(payload)
		require.NoError(t, err)
		require.NotNil(t, content)
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
