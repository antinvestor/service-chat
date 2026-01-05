package models_test

import (
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Payload field constants (re-exported from models package for testing).
const (
	PayloadTypeField = models.PayloadTypeField
	ContentField     = models.ContentField
)

func TestPayloadConverter_TextContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Text Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number()),
			ContentField:     []byte(`{"body":"Hello, world!"}`),
		}

		payload, err := converter.ToProto(content)
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

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number(), content[PayloadTypeField])

		// Content is now stored as []byte in the JSONMap
		contentBytes, ok := content[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), "Hello, world!")
	})

	t.Run("Round Trip - Text Content", func(t *testing.T) {
		original := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number()),
			ContentField:     []byte(`{"body":"Hello, world!","format":"<b>Hello, world!</b>"}`),
		}

		// Convert to proto
		payload, err := converter.ToProto(original)
		require.NoError(t, err)

		// Convert back to domain
		result, err := converter.FromProto(payload)
		require.NoError(t, err)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number(), result[PayloadTypeField])

		// Content is now stored as []byte in the JSONMap
		contentBytes, ok := result[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), "Hello, world!")
	})
}

func TestPayloadConverter_AttachmentContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Attachment Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT.Number()),
			ContentField: []byte(
				`{"attachment_id":"attach123","filename":"document.pdf","mime_type":"application/pdf","size_bytes":1024000}`,
			),
		}

		payload, err := converter.ToProto(content)
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

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT.Number(), content[PayloadTypeField])
		assert.Contains(t, string(content[ContentField].([]byte)), "attach123")
	})

	// TODO: Re-enable when validation methods are implemented
	/*
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
	*/
}

func TestPayloadConverter_ReactionContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Reaction Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_REACTION.Number()),
			ContentField:     []byte(`{"reaction":"👍"}`),
		}

		payload, err := converter.ToProto(content)
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

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_REACTION.Number(), content[PayloadTypeField])

		// Content is now stored as []byte in the JSONMap
		contentBytes, ok := content[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), "❤️")
	})

	// TODO: Re-enable when validation methods are implemented
	/*
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
	*/
}

func TestPayloadConverter_CallContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Call Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_CALL.Number()),
			ContentField:     []byte(`{"sdp":"v=0\r\no=- 123..."}`),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_CALL, payload.GetType())

		call := payload.GetCall()
		require.NotNil(t, call)
		assert.NotNil(t, call.GetSdp())
		assert.Equal(t, "v=0\r\no=- 123...", call.GetSdp())
	})

	t.Run("FromProto - Call Content", func(t *testing.T) {
		sdp := "v=0\r\no=- 789..."
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_CALL)
		payload.SetCall(&chatv1.CallContent{
			Sdp: &sdp,
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_CALL.Number(), content[PayloadTypeField])

		// Content is now stored as []byte in the JSONMap
		contentBytes, ok := content[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), `"v=0\r\no=- 789..."`)
	})

	// TODO: Re-enable when validation methods are implemented
	/*
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
	*/
}

func TestPayloadConverter_EncryptedContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Encrypted Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED.Number()),
			ContentField: []byte(
				`{"ciphertext":"ZW5jcnlwdGVkX2RhdGE=","algorithm":"m.megolm.v1.aes-sha2","session_id":"session_abc"}`,
			),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED, payload.GetType())

		encrypted := payload.GetEncrypted()
		require.NotNil(t, encrypted)
		assert.Equal(t, "encrypted_data", string(encrypted.GetCiphertext()))
		assert.Equal(t, "m.megolm.v1.aes-sha2", encrypted.GetAlgorithm())
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

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED.Number(), content[PayloadTypeField])
		assert.Contains(t, string(content[ContentField].([]byte)), `"ciphertext":"ZW5jcnlwdGVkX2RhdGE="`)
	})
}

func TestPayloadConverter_EdgeCases(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("Nil Event", func(t *testing.T) {
		_, err := converter.ToProto(nil)
		require.Error(t, err)

		_, err = converter.FromProto(nil)
		require.Error(t, err)
	})

	t.Run("Empty Content", func(t *testing.T) {
		content := data.JSONMap{}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)
	})

	t.Run("Missing Content in Proto", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetText(&chatv1.TextContent{Body: "Hello"})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)
	})
}
