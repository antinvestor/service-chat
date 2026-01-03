package models

import (
	"encoding/json"
	"errors"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/data"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PayloadConverter handles conversion between JSONMap payloads and typed RoomEvent content.
type PayloadConverter struct{}

// NewPayloadConverter creates a new PayloadConverter instance.
func NewPayloadConverter() *PayloadConverter {
	return &PayloadConverter{}
}

// ToProtoRoomEvent converts a domain RoomEvent to a protobuf RoomEvent with typed content.
func (c *PayloadConverter) ToProtoRoomEvent(event *RoomEvent) (*chatv1.RoomEvent, error) {
	if event == nil {
		return nil, errors.New("event cannot be nil")
	}

	// Create base proto event
	protoEvent := &chatv1.RoomEvent{
		Id:       event.ID,
		RoomId:   event.RoomID,
		SenderId: event.SenderID,
		Type:     chatv1.RoomEventType(event.EventType),
	}

	// Set timestamp if available
	if !event.CreatedAt.IsZero() {
		protoEvent.SentAt = timestamppb.New(event.CreatedAt)
	}

	// Set parent ID if present
	if event.ParentID != "" {
		parentID := event.ParentID
		protoEvent.ParentId = &parentID
	}

	// Convert content based on event type
	if err := c.setTypedContent(protoEvent, event.Content); err != nil {
		return nil, fmt.Errorf("failed to set typed content: %w", err)
	}

	return protoEvent, nil
}

// FromProtoRoomEvent converts a protobuf RoomEvent to a domain RoomEvent with JSONMap content.
func (c *PayloadConverter) FromProtoRoomEvent(protoEvent *chatv1.RoomEvent) (*RoomEvent, error) {
	if protoEvent == nil {
		return nil, errors.New("proto event cannot be nil")
	}

	// Create base domain event
	event := &RoomEvent{
		BaseModel: data.BaseModel{
			ID: protoEvent.GetId(),
		},
		RoomID:    protoEvent.GetRoomId(),
		SenderID:  protoEvent.GetSenderId(),
		EventType: int32(protoEvent.GetType()),
	}

	// Set parent ID if present
	if protoEvent.ParentId != nil {
		event.ParentID = protoEvent.GetParentId()
	}

	// Set timestamp
	if protoEvent.GetSentAt() != nil {
		event.CreatedAt = protoEvent.GetSentAt().AsTime()
	}

	// Extract content from typed fields
	content, err := c.extractTypedContent(protoEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to extract typed content: %w", err)
	}
	event.Content = content

	return event, nil
}

// setTypedContent sets the appropriate typed content field on the proto event.
func (c *PayloadConverter) setTypedContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
	if len(content) == 0 {
		return nil
	}

	switch protoEvent.GetType() {
	case chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT:
		return c.setTextContent(protoEvent, content)

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT:
		return c.setAttachmentContent(protoEvent, content)

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION:
		return c.setReactionContent(protoEvent, content)

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED:
		return c.setEncryptedContent(protoEvent, content)

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL:
		return c.setCallContent(protoEvent, content)

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_UNSPECIFIED,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_EDIT,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_REDACTION:
		// For unknown/unhandled types, no typed content
		// This maintains forward compatibility
		return nil

	default:
		// Unknown event type
		return nil
	}
}

// setTextContent converts JSONMap to TextContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setTextContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
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

	protoEvent.SetText(textContent)
	return nil
}

// setAttachmentContent converts JSONMap to AttachmentContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setAttachmentContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
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

	protoEvent.SetAttachment(attachment)
	return nil
}

// setReactionContent converts JSONMap to ReactionContent.
func (c *PayloadConverter) setReactionContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
	emoji, ok := content["emoji"].(string)
	if !ok {
		return errors.New("reaction content must have 'emoji' field")
	}

	// Reaction requires a parent event (the message being reacted to)
	if protoEvent.ParentId == nil || protoEvent.GetParentId() == "" {
		return errors.New("reaction must have parentId set")
	}

	reaction := &chatv1.ReactionContent{
		Reaction: emoji,
		Add:      true, // Default to adding reaction
	}

	protoEvent.SetReaction(reaction)
	return nil
}

// setEncryptedContent converts JSONMap to EncryptedContent.
//
//nolint:govet // Multiple type assertions with 'ok' variable is idiomatic Go
func (c *PayloadConverter) setEncryptedContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
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

	protoEvent.SetEncrypted(encrypted)
	return nil
}

// setCallContent converts JSONMap to CallContent.
func (c *PayloadConverter) setCallContent(protoEvent *chatv1.RoomEvent, content data.JSONMap) error {
	// Call content will use the type field and potentially additional data
	// For now, just create a minimal call content
	call := &chatv1.CallContent{}

	// WebRTC signaling data
	if sdp, ok := content["sdp"].(string); ok {
		call.Sdp = &sdp
	}

	protoEvent.SetCall(call)
	return nil
}

// extractTypedContent extracts content from typed proto fields into JSONMap.
func (c *PayloadConverter) extractTypedContent(protoEvent *chatv1.RoomEvent) (data.JSONMap, error) {
	content := make(data.JSONMap)

	switch protoEvent.GetType() {
	case chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT:
		text := protoEvent.GetText()
		if text != nil {
			content["text"] = text.GetBody()
			if text.GetFormat() != "" {
				content["format"] = text.GetFormat()
			}
		}

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT:
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

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION:
		reaction := protoEvent.GetReaction()
		if reaction != nil {
			content["emoji"] = reaction.GetReaction()
		}

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED:
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

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL:
		call := protoEvent.GetCall()
		if call != nil {
			if call.Sdp != nil {
				content["sdp"] = call.GetSdp()
			}
		}

	case chatv1.RoomEventType_ROOM_EVENT_TYPE_UNSPECIFIED,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_EDIT,
		chatv1.RoomEventType_ROOM_EVENT_TYPE_REDACTION:
		// No typed content for these event types
		// Empty content map is returned
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
