package models

import (
	"strings"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/pitabwire/frame/data"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Room represents a chat room entity.
type Room struct {
	data.BaseModel
	RoomType    string `json:"room_type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Properties  data.JSONMap
	IsPublic    bool
}

// ToAPI converts Room model to API representation.
func (r *Room) ToAPI() *chatv1.Room {
	if r == nil {
		return nil
	}

	var metadata *structpb.Struct
	if r.Properties != nil {
		metadata, _ = structpb.NewStruct(r.Properties)
	}

	return &chatv1.Room{
		Id:          r.GetID(),
		Name:        r.Name,
		Description: r.Description,
		IsPrivate:   !r.IsPublic,
		Metadata:    metadata,
		CreatedAt:   timestamppb.New(r.CreatedAt),
	}
}

// RoomCall represents a call session in a room.
type RoomCall struct {
	data.BaseModel
	RoomID    string `gorm:"type:varchar(50)"`
	CallID    string `gorm:"type:varchar(50)"`
	SFUNodeID string `gorm:"type:varchar(250)"`
	Status    string // ringing, active, ended
	StartedAt time.Time
	EndedAt   time.Time
	Metadata  data.JSONMap
}

// RoomEvent represents a message or event in a room.
// The ID field (from BaseModel) is naturally time-sorted and used for ordering.
type RoomEvent struct {
	data.BaseModel
	RoomID     string `gorm:"type:varchar(50);index:idx_room_id"`
	SenderID   string `gorm:"type:varchar(50)"`
	ParentID   string `gorm:"type:varchar(50)"`
	EventType  int32
	Content    data.JSONMap
	Properties data.JSONMap
}

// ToAPI converts RoomEvent model to API RoomEvent representation.
func (re *RoomEvent) ToAPI() *chatv1.RoomEvent {
	if re == nil {
		return nil
	}

	// Map message type to RoomEventType
	eventType := chatv1.RoomEventType(re.EventType)

	roomEvent := &chatv1.RoomEvent{
		Id:       re.GetID(),
		RoomId:   re.RoomID,
		SenderId: re.SenderID,
		Type:     eventType,
		SentAt:   timestamppb.New(re.CreatedAt),
		Edited:   false,
		Redacted: false,
	}

	// Populate payload based on event type
	// Note: This is a simplified conversion - actual implementation may need
	// to properly marshal/unmarshal the Content/Properties to specific types
	if re.Properties != nil {
		switch eventType {
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT:
			// Extract text content if available
			if text, ok := re.Properties["text"].(string); ok {
				roomEvent.Payload = &chatv1.RoomEvent_Text{
					Text: &chatv1.TextContent{Body: text},
				}
			}
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_ATTACHMENT:
			// TODO: Properly convert attachment data
			roomEvent.Payload = &chatv1.RoomEvent_Attachment{
				Attachment: &chatv1.AttachmentContent{},
			}
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_REACTION:
			// TODO: Properly convert reaction data
			roomEvent.Payload = &chatv1.RoomEvent_Reaction{
				Reaction: &chatv1.ReactionContent{},
			}
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_ENCRYPTED:
			// TODO: Properly convert encrypted data
			roomEvent.Payload = &chatv1.RoomEvent_Encrypted{
				Encrypted: &chatv1.EncryptedContent{},
			}
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL:
			// TODO: Properly convert call data
			roomEvent.Payload = &chatv1.RoomEvent_Call{
				Call: &chatv1.CallContent{},
			}
		}
	}

	return roomEvent
}

type RoomSubscriptionState int

const (
	RoomSubscriptionStateProposed RoomSubscriptionState = iota
	RoomSubscriptionStateActive
	RoomSubscriptionStateBlocked
)

// RoomSubscription represents a user's subscription to a room.
type RoomSubscription struct {
	data.BaseModel
	RoomID              string `gorm:"type:varchar(50);index:idx_roomsubscription_room_id_subscription_state"`
	ProfileID           string `gorm:"type:varchar(50)"`
	Role                string
	SubscriptionState   RoomSubscriptionState `gorm:"index:idx_roomsubscription_room_id_subscription_state"`
	LastReadEventID     string                `gorm:"type:varchar(50)"` // ID of the last read event (naturally time-sorted)
	LastReadAt          int64
	DisableNotification bool
	Properties          data.JSONMap
}

// ToAPI converts RoomSubscription model to API representation.
func (rs *RoomSubscription) ToAPI() *chatv1.RoomSubscription {
	if rs == nil {
		return nil
	}

	var lastActive *timestamppb.Timestamp
	if rs.LastReadAt > 0 {
		lastActive = timestamppb.New(time.Unix(rs.LastReadAt, 0))
	}

	return &chatv1.RoomSubscription{
		RoomId: rs.RoomID,
		// Use ContactLink with profileId
		// TODO: Update database schema to store profile_name, profile_type, etc.
		Member: &commonv1.ContactLink{
			ProfileId: rs.ProfileID,
			// TODO: Add profile_name, profile_type from database when available
		},
		Roles:      strings.Split(rs.Role, ","),
		JoinedAt:   timestamppb.New(rs.CreatedAt),
		LastActive: lastActive,
	}
}

func (rs *RoomSubscription) IsActive() bool {
	return RoomSubscriptionStateActive == rs.SubscriptionState
}
