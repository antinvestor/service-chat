package models

import (
	"encoding/json"
	"errors"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/data"
)

// PayloadConverter handles conversion between JSONMap payloads and typed RoomEvent content.
type PayloadConverter struct{}

// NewPayloadConverter creates a new PayloadConverter instance.
func NewPayloadConverter() *PayloadConverter {
	return &PayloadConverter{}
}

// ToProtoRoomEvent converts a domain RoomEvent to a protobuf RoomEvent with typed content.
func (c *PayloadConverter) ToProtoRoomEvent(content data.JSONMap) (*chatv1.Payload, error) {
	if content == nil {
		return nil, errors.New("content cannot be nil")
	}

	// Create base proto event
	protoPayload := &chatv1.Payload{}

	// Convert content based on event type
	if err := c.setTypedContent(protoPayload, content); err != nil {
		return nil, fmt.Errorf("failed to set typed content: %w", err)
	}

	return protoPayload, nil
}

// FromProtoRoomEvent converts a protobuf RoomEvent to a domain RoomEvent with JSONMap content.
func (c *PayloadConverter) FromProtoRoomEvent(protoEvent *chatv1.Payload) (data.JSONMap, error) {
	if protoEvent == nil {
		return nil, errors.New("proto event cannot be nil")
	}

	// Extract content from typed fields
	return c.extractTypedContent(protoEvent)
}

// setTypedContent sets the appropriate typed content field on the proto event.
// It detects the type from the content keys.
func (c *PayloadConverter) setTypedContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	if len(content) == 0 {
		return nil
	}

	// Detect content type based on keys present
	if _, hasText := content["text"]; hasText {
		return c.setTextContent(protoEvent, content)
	}

	if _, hasAttachment := content["attachmentId"]; hasAttachment {
		return c.setAttachmentContent(protoEvent, content)
	}

	if _, hasEmoji := content["emoji"]; hasEmoji {
		return c.setReactionContent(protoEvent, content)
	}

	if _, hasCiphertext := content["ciphertext"]; hasCiphertext {
		return c.setEncryptedContent(protoEvent, content)
	}

	if _, hasSDP := content["sdp"]; hasSDP {
		return c.setCallContent(protoEvent, content)
	}

	if _, hasMotionID := content["id"]; hasMotionID {
		// Check if it's a motion (has title/description) vs other content with "id"
		if _, hasTitle := content["title"]; hasTitle {
			return c.setMotionContent(protoEvent, content)
		}
	}

	if _, hasMotionID := content["motionId"]; hasMotionID {
		return c.setVoteContent(protoEvent, content)
	}

	// Unknown/unhandled payload type - store in default field
	return c.setDefaultContent(protoEvent, content)
}

// setTextContent converts JSONMap to TextContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setTextContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	text, ok := content["text"].(string)
	if !ok {
		return errors.New("text content must have 'text' field as string")
	}

	textContent := &chatv1.TextContent{
		Body: text,
	}

	// Optional: formatted text (markdown, HTML, etc.)
	if format, ok := content["format"].(string); ok {
		textContent.Format = format
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_TEXT)
	protoEvent.SetText(textContent)
	return nil
}

// setAttachmentContent converts JSONMap to AttachmentContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setAttachmentContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	attachmentID, ok := content["attachmentId"].(string)
	if !ok {
		return errors.New("attachment content must have 'attachmentId' field")
	}

	attachment := &chatv1.AttachmentContent{
		AttachmentId: attachmentID,
	}

	// Optional fields
	if filename, ok := content["fileName"].(string); ok {
		attachment.Filename = filename
	}
	if mimeType, ok := content["mimeType"].(string); ok {
		attachment.MimeType = mimeType
	}
	if size, ok := content["size"].(float64); ok {
		attachment.SizeBytes = int64(size)
	} else if size, ok := content["size"].(int64); ok {
		attachment.SizeBytes = size
	} else if size, ok := content["size"].(int); ok {
		attachment.SizeBytes = int64(size)
	}

	// Caption (if present) - TextContent structure
	if captionText, ok := content["caption"].(string); ok {
		attachment.Caption = &chatv1.TextContent{Body: captionText}
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT)
	protoEvent.SetAttachment(attachment)
	return nil
}

// setReactionContent converts JSONMap to ReactionContent.
func (c *PayloadConverter) setReactionContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	emoji, ok := content["emoji"].(string)
	if !ok {
		return errors.New("reaction content must have 'emoji' field")
	}

	reaction := &chatv1.ReactionContent{
		Reaction: emoji,
		Add:      true, // Default to adding reaction
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_REACTION)
	protoEvent.SetReaction(reaction)
	return nil
}

// setEncryptedContent converts JSONMap to EncryptedContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setEncryptedContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	ciphertext, ok := content["ciphertext"].(string)
	if !ok {
		return errors.New("encrypted content must have 'ciphertext' field")
	}

	encrypted := &chatv1.EncryptedContent{
		Ciphertext: []byte(ciphertext),
	}

	// Optional fields
	if algorithm, ok := content["algorithm"].(string); ok {
		encrypted.Algorithm = algorithm
	}
	if sessionID, ok := content["sessionId"].(string); ok {
		encrypted.SessionId = &sessionID
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED)
	protoEvent.SetEncrypted(encrypted)
	return nil
}

// setCallContent converts JSONMap to CallContent.
func (c *PayloadConverter) setCallContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	// Call content will use the type field and potentially additional data
	// For now, just create a minimal call content
	call := &chatv1.CallContent{}

	// WebRTC signaling data
	if sdp, ok := content["sdp"].(string); ok {
		call.Sdp = &sdp
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_CALL)
	protoEvent.SetCall(call)
	return nil
}

// setMotionContent converts JSONMap to MotionContent.
//

func (c *PayloadConverter) setMotionContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	motion := &chatv1.MotionContent{}

	// Required fields
	if id, ok := content["id"].(string); ok {
		motion.Id = id
	}
	if title, ok := content["title"].(string); ok {
		motion.Title = title
	}
	if description, ok := content["description"].(string); ok {
		motion.Description = description
	}

	// Optional fields - for complex types like PassingRule and Choices,
	// we'll need the full proto structures. For now, store basic fields.
	// TODO: Add full support for PassingRule, VoteChoice when needed

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_MOTION)
	protoEvent.SetMotion(motion)
	return nil
}

// setVoteContent converts JSONMap to VoteCast.
//

func (c *PayloadConverter) setVoteContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	vote := &chatv1.VoteCast{}

	// Vote content structure
	if motionID, ok := content["motionId"].(string); ok {
		vote.MotionId = motionID
	}
	if choiceID, ok := content["choiceId"].(string); ok {
		vote.ChoiceId = choiceID
	}

	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_VOTE)
	protoEvent.SetVote(vote)
	return nil
}

// setDefaultContent converts JSONMap to generic Struct for unknown payload types.
func (c *PayloadConverter) setDefaultContent(protoEvent *chatv1.Payload, content data.JSONMap) error {
	if len(content) == 0 {
		return nil
	}

	structData := content.ToProtoStruct()
	protoEvent.SetType(chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED)
	protoEvent.SetDefault(structData)
	return nil
}

// extractTypedContent extracts content from typed proto fields into JSONMap.
//
//nolint:gocognit // Event type handling requires multiple cases and conditionals
func (c *PayloadConverter) extractTypedContent(protoEvent *chatv1.Payload) (data.JSONMap, error) {
	content := make(data.JSONMap)

	switch protoEvent.GetType() {
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		text := protoEvent.GetText()
		if text != nil {
			content["text"] = text.GetBody()
			if text.GetFormat() != "" {
				content["format"] = text.GetFormat()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		attachment := protoEvent.GetAttachment()
		if attachment != nil {
			content["attachmentId"] = attachment.GetAttachmentId()
			if attachment.GetFilename() != "" {
				content["fileName"] = attachment.GetFilename()
			}
			if attachment.GetMimeType() != "" {
				content["mimeType"] = attachment.GetMimeType()
			}
			if attachment.GetSizeBytes() != 0 {
				content["size"] = attachment.GetSizeBytes()
			}
			if attachment.GetCaption() != nil {
				content["caption"] = attachment.GetCaption().GetBody()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		reaction := protoEvent.GetReaction()
		if reaction != nil {
			content["emoji"] = reaction.GetReaction()
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		encrypted := protoEvent.GetEncrypted()
		if encrypted != nil {
			content["ciphertext"] = string(encrypted.GetCiphertext())
			if encrypted.GetAlgorithm() != "" {
				content["algorithm"] = encrypted.GetAlgorithm()
			}
			if encrypted.SessionId != nil {
				content["sessionId"] = encrypted.GetSessionId()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		call := protoEvent.GetCall()
		if call != nil {
			if call.Sdp != nil {
				content["sdp"] = call.GetSdp()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		motion := protoEvent.GetMotion()
		if motion != nil {
			content["id"] = motion.GetId()
			content["title"] = motion.GetTitle()
			content["description"] = motion.GetDescription()
			// TODO: Add full support for PassingRule, VoteChoice, eligible roles when needed
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		vote := protoEvent.GetVote()
		if vote != nil {
			content["motionId"] = vote.GetMotionId()
			content["choiceId"] = vote.GetChoiceId()
		}

	default:
		dump := protoEvent.GetDefault()
		if dump != nil {
			content.FromProtoStruct(dump)
		}
	}

	return content, nil
}

// ContentToJSON converts JSONMap to JSON string (for logging/debugging).
func (c *PayloadConverter) ContentToJSON(content data.JSONMap) (string, error) {
	if content == nil {
		return "{}", nil
	}

	bytes, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("failed to marshal content: %w", err)
	}

	return string(bytes), nil
}

// ContentFromJSON parses JSON string into JSONMap.
func (c *PayloadConverter) ContentFromJSON(jsonStr string) (data.JSONMap, error) {
	if jsonStr == "" {
		return make(data.JSONMap), nil
	}

	var content data.JSONMap
	if err := json.Unmarshal([]byte(jsonStr), &content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	return content, nil
}

// ValidateTextContent validates that the JSONMap contains valid text content.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) ValidateTextContent(content data.JSONMap) error {
	if content == nil {
		return errors.New("content cannot be nil")
	}

	text, ok := content["text"]
	if !ok {
		return errors.New("text content must have 'text' field")
	}

	if _, ok := text.(string); !ok {
		return errors.New("text field must be a string")
	}

	return nil
}

// ValidateAttachmentContent validates that the JSONMap contains valid attachment content.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) ValidateAttachmentContent(content data.JSONMap) error {
	if content == nil {
		return errors.New("content cannot be nil")
	}

	attachmentID, ok := content["attachmentId"]
	if !ok {
		return errors.New("attachment content must have 'attachmentId' field")
	}

	if _, ok := attachmentID.(string); !ok {
		return errors.New("attachmentId field must be a string")
	}

	return nil
}

// ValidateReactionContent validates that the JSONMap contains valid reaction content.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) ValidateReactionContent(content data.JSONMap) error {
	if content == nil {
		return errors.New("content cannot be nil")
	}

	emoji, ok := content["emoji"]
	if !ok {
		return errors.New("reaction content must have 'emoji' field")
	}

	if _, ok := emoji.(string); !ok {
		return errors.New("emoji field must be a string")
	}

	return nil
}

// ValidateCallContent validates that the JSONMap contains valid call content.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) ValidateCallContent(content data.JSONMap) error {
	if content == nil {
		return errors.New("content cannot be nil")
	}

	callID, ok := content["callId"]
	if !ok {
		return errors.New("call content must have 'callId' field")
	}

	if _, ok := callID.(string); !ok {
		return errors.New("callId field must be a string")
	}

	return nil
}

// CreateTextContent creates a JSONMap for text content.
func (c *PayloadConverter) CreateTextContent(text string) data.JSONMap {
	return data.JSONMap{
		"text": text,
	}
}

// CreateAttachmentContent creates a JSONMap for attachment content.
func (c *PayloadConverter) CreateAttachmentContent(attachmentID, filename, mimeType string, size int64) data.JSONMap {
	return data.JSONMap{
		"attachmentId": attachmentID,
		"fileName":     filename,
		"mimeType":     mimeType,
		"size":         size,
	}
}

// CreateReactionContent creates a JSONMap for reaction content.
func (c *PayloadConverter) CreateReactionContent(emoji string) data.JSONMap {
	return data.JSONMap{
		"emoji": emoji,
	}
}

// CreateCallContent creates a JSONMap for call content.
func (c *PayloadConverter) CreateCallContent(callID, callType, action string) data.JSONMap {
	return data.JSONMap{
		"callId":   callID,
		"callType": callType,
		"action":   action,
	}
}
