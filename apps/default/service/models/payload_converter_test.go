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

	t.Run("Round Trip - Attachment Content", func(t *testing.T) {
		original := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT.Number()),
			ContentField:     []byte(`{"attachment_id":"attach789","filename":"report.pdf","mime_type":"application/pdf"}`),
		}

		payload, err := converter.ToProto(original)
		require.NoError(t, err)

		result, err := converter.FromProto(payload)
		require.NoError(t, err)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT.Number(), result[PayloadTypeField])
		contentBytes := result[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "attach789")
	})
}

func TestPayloadConverter_ReactionContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Reaction Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_REACTION.Number()),
			ContentField:     []byte(`{"reaction":"👍","target_event_id":"evt123","add":true}`),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_REACTION, payload.GetType())

		reaction := payload.GetReaction()
		require.NotNil(t, reaction)
		assert.Equal(t, "👍", reaction.GetReaction())
		assert.Equal(t, "evt123", reaction.GetTargetEventId())
		assert.True(t, reaction.GetAdd())
	})

	t.Run("FromProto - Reaction Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_REACTION)
		payload.SetReaction(&chatv1.ReactionContent{
			Reaction:      "❤️",
			TargetEventId: "evt456",
			Add:           true,
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_REACTION.Number(), content[PayloadTypeField])

		contentBytes, ok := content[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), "❤️")
		assert.Contains(t, string(contentBytes), "evt456")
	})

	t.Run("Round Trip - Reaction Content", func(t *testing.T) {
		original := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_REACTION.Number()),
			ContentField:     []byte(`{"reaction":"🎉","target_event_id":"msg123","add":true}`),
		}

		payload, err := converter.ToProto(original)
		require.NoError(t, err)

		result, err := converter.FromProto(payload)
		require.NoError(t, err)

		contentBytes := result[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "🎉")
	})
}

func TestPayloadConverter_CallContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Call Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_CALL.Number()),
			ContentField:     []byte(`{"sdp":"v=0\r\no=- 123...","type":1}`),
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
			Sdp:  &sdp,
			Type: chatv1.CallContent_CALL_TYPE_VIDEO,
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_CALL.Number(), content[PayloadTypeField])

		contentBytes, ok := content[ContentField].([]byte)
		require.True(t, ok, "ContentField should be []byte")
		assert.Contains(t, string(contentBytes), `"v=0\r\no=- 789..."`)
	})

	t.Run("Round Trip - Call Content", func(t *testing.T) {
		original := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_CALL.Number()),
			ContentField:     []byte(`{"sdp":"test-sdp-data","type":2}`),
		}

		payload, err := converter.ToProto(original)
		require.NoError(t, err)

		result, err := converter.FromProto(payload)
		require.NoError(t, err)

		contentBytes := result[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "test-sdp-data")
	})
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

	t.Run("Round Trip - Encrypted Content", func(t *testing.T) {
		original := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED.Number()),
			ContentField:     []byte(`{"ciphertext":"dGVzdA==","algorithm":"aes-256","session_id":"sess789"}`),
		}

		payload, err := converter.ToProto(original)
		require.NoError(t, err)

		result, err := converter.FromProto(payload)
		require.NoError(t, err)

		contentBytes := result[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "aes-256")
	})
}

func TestPayloadConverter_ModerationContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Moderation Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_MODERATION.Number()),
			ContentField:     []byte(`{"body":"Member added to room","actor_subscription_id":"sub123","target_subscription_ids":["sub456","sub789"]}`),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_MODERATION, payload.GetType())

		moderation := payload.GetModeration()
		require.NotNil(t, moderation)
		assert.Equal(t, "Member added to room", moderation.GetBody())
		assert.Equal(t, "sub123", moderation.GetActorSubscriptionId())
		assert.Equal(t, []string{"sub456", "sub789"}, moderation.GetTargetSubscriptionIds())
	})

	t.Run("FromProto - Moderation Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_MODERATION)
		payload.SetModeration(&chatv1.ModerationContent{
			Body:                  "Room deleted",
			ActorSubscriptionId:   "admin123",
			TargetSubscriptionIds: []string{"member456"},
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_MODERATION.Number(), content[PayloadTypeField])
		contentBytes := content[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "Room deleted")
		assert.Contains(t, string(contentBytes), "admin123")
	})
}

func TestPayloadConverter_MotionContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Motion Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_MOTION.Number()),
			ContentField:     []byte(`{"title":"Proposal","description":"Test proposal"}`),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_MOTION, payload.GetType())
	})

	t.Run("FromProto - Motion Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_MOTION)
		payload.SetMotion(&chatv1.MotionContent{
			Id:          "motion123",
			Title:       "Test Motion",
			Description: "A test motion description",
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_MOTION.Number(), content[PayloadTypeField])
		contentBytes := content[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "Test Motion")
	})
}

func TestPayloadConverter_VoteContent(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("ToProto - Vote Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_VOTE.Number()),
			ContentField:     []byte(`{"motion_id":"motion123","choice_id":"yes"}`),
		}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_VOTE, payload.GetType())

		vote := payload.GetVote()
		require.NotNil(t, vote)
		assert.Equal(t, "motion123", vote.GetMotionId())
		assert.Equal(t, "yes", vote.GetChoiceId())
	})

	t.Run("FromProto - Vote Content", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_VOTE)
		payload.SetVote(&chatv1.VoteCast{
			MotionId: "motion456",
			ChoiceId: "no",
		})

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_VOTE.Number(), content[PayloadTypeField])
		contentBytes := content[ContentField].([]byte)
		assert.Contains(t, string(contentBytes), "motion456")
		assert.Contains(t, string(contentBytes), "no")
	})
}

func TestPayloadConverter_EdgeCases(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("Nil Event", func(t *testing.T) {
		_, err := converter.ToProto(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")

		_, err = converter.FromProto(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("Empty Content", func(t *testing.T) {
		content := data.JSONMap{}

		payload, err := converter.ToProto(content)
		require.NoError(t, err)
		require.NotNil(t, payload)
		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED, payload.GetType())
	})

	t.Run("Missing Content in Proto", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_TEXT)
		// No SetText call - content is nil

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)
		assert.Equal(t, chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number(), content[PayloadTypeField])
	})

	t.Run("Invalid JSON Content", func(t *testing.T) {
		content := data.JSONMap{
			PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number()),
			ContentField:     []byte(`{invalid json`),
		}

		_, err := converter.ToProto(content)
		require.Error(t, err) // Should fail to unmarshal
	})

	t.Run("Unspecified Payload Type", func(t *testing.T) {
		payload := &chatv1.Payload{}
		payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED)

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)
	})

	t.Run("Unknown Payload Type - Fallback to Default", func(t *testing.T) {
		payload := &chatv1.Payload{}
		// Use a high number that might not exist
		payload.SetType(chatv1.PayloadType(999))

		content, err := converter.FromProto(payload)
		require.NoError(t, err)
		require.NotNil(t, content)
	})
}

func TestPayloadConverter_Concurrency(t *testing.T) {
	converter := models.NewPayloadConverter()

	t.Run("Concurrent ToProto", func(t *testing.T) {
		// PayloadConverter should be stateless and safe for concurrent use
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				content := data.JSONMap{
					PayloadTypeField: float64(chatv1.PayloadType_PAYLOAD_TYPE_TEXT.Number()),
					ContentField:     []byte(`{"body":"concurrent test"}`),
				}
				payload, err := converter.ToProto(content)
				assert.NoError(t, err)
				assert.NotNil(t, payload)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("Concurrent FromProto", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				payload := &chatv1.Payload{}
				payload.SetType(chatv1.PayloadType_PAYLOAD_TYPE_TEXT)
				payload.SetText(&chatv1.TextContent{Body: "concurrent test"})

				content, err := converter.FromProto(payload)
				assert.NoError(t, err)
				assert.NotNil(t, content)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
