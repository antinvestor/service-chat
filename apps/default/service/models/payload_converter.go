package models

import (
	"encoding/json"
	"errors"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/data"
)

// Payload field constants.
const (
	PayloadTypeField = "p_type"
	ContentField     = "content"
)

// PayloadConverter handles conversion between protobuf payloads and JSON maps.
// It's stateless and can be used concurrently.
type PayloadConverter struct{}

// NewPayloadConverter creates a new PayloadConverter instance.
func NewPayloadConverter() *PayloadConverter {
	return &PayloadConverter{}
}

// ToProto converts a domain JSONMap to a protobuf Payload with typed content.
func (c *PayloadConverter) ToProto(content data.JSONMap) (*chatv1.Payload, error) {
	if content == nil {
		return nil, errors.New("content cannot be nil")
	}

	// Extract payload type and content
	payloadType := chatv1.PayloadType(content.GetFloat(PayloadTypeField))

	// Create base proto event
	protoPayload := &chatv1.Payload{Type: payloadType}

	rawContent, ok := content[ContentField]
	if ok {
		payloadData, dataOk := rawContent.([]byte)
		if dataOk {
			// Convert content based on type
			if err := c.setTypedContent(protoPayload, payloadType, payloadData); err != nil {
				return nil, fmt.Errorf("failed to set typed content: %w", err)
			}
		}
	}

	return protoPayload, nil
}

// FromProto converts a protobuf Payload to a domain JSONMap with content.
func (c *PayloadConverter) FromProto(protoEvent *chatv1.Payload) (data.JSONMap, error) {
	if protoEvent == nil {
		return nil, errors.New("proto event cannot be nil")
	}

	// Extract payload based on type
	var payload interface{}
	switch protoEvent.GetType() {
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		payload = protoEvent.GetText()
	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		payload = protoEvent.GetAttachment()
	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		payload = protoEvent.GetReaction()
	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		payload = protoEvent.GetEncrypted()
	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		payload = protoEvent.GetCall()
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		payload = protoEvent.GetMotion()
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		payload = protoEvent.GetVote()
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION_TALLY:
		payload = protoEvent.GetMotionTally()
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE_TALLY:
		payload = protoEvent.GetVoteTally()
	case chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED:
		payload = protoEvent.GetDefault()
	default: // Covers unknown types
		payload = protoEvent.GetDefault()
	}

	content := data.JSONMap{
		PayloadTypeField: protoEvent.GetType().Number(),
	}

	if payload != nil {
		// Marshal payload to JSON
		jsonContent, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		content[ContentField] = jsonContent
	}

	// Return structured result
	return content, nil
}

// setTypedContent sets the appropriate typed content field on the proto event.
func (c *PayloadConverter) setTypedContent(
	protoEvent *chatv1.Payload,
	payloadType chatv1.PayloadType,
	content []byte,
) error {
	if len(content) == 0 {
		return nil
	}

	switch payloadType {
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		return unmarshalAndSet(content, protoEvent.SetText)
	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		return unmarshalAndSet(content, protoEvent.SetAttachment)
	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		return unmarshalAndSet(content, protoEvent.SetReaction)
	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		return unmarshalAndSet(content, protoEvent.SetEncrypted)
	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		return unmarshalAndSet(content, protoEvent.SetCall)
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		return unmarshalAndSet(content, protoEvent.SetMotion)
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		return unmarshalAndSet(content, protoEvent.SetVote)
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION_TALLY:
		return unmarshalAndSet(content, protoEvent.SetMotionTally)
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE_TALLY:
		return unmarshalAndSet(content, protoEvent.SetVoteTally)
	case chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED:
		return unmarshalAndSet(content, protoEvent.SetDefault)
	default:
		return unmarshalAndSet(content, protoEvent.SetDefault)
	}
}

// unmarshalAndSet is a generic function that unmarshals JSON content into a proto type
// and sets it on the proto event using the provided setter function.
func unmarshalAndSet[T any](
	content []byte,
	setter func(*T),
) error {
	var proto T
	if err := json.Unmarshal(content, &proto); err != nil {
		return err
	}
	setter(&proto)
	return nil
}
