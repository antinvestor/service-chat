package models

import (
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	"github.com/pitabwire/frame"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Room represents a chat room entity.
type Room struct {
	frame.BaseModel
	RoomType    string `json:"room_type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Metadata    frame.JSONMap
	IsPublic    bool
}

// ToAPI converts Room model to API representation.
func (r *Room) ToAPI() *chatv1.Room {
	if r == nil {
		return nil
	}

	var metadata *structpb.Struct
	if r.Metadata != nil {
		metadata, _ = structpb.NewStruct(r.Metadata)
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
	frame.BaseModel
	RoomID    string `gorm:"type:varchar(50)"`
	CallID    string `gorm:"type:varchar(50)"`
	SFUNodeID string `gorm:"type:varchar(250)"`
	Status    string // ringing, active, ended
	StartedAt time.Time
	EndedAt   time.Time
	Metadata  frame.JSONMap
}

// RoomEvent represents a message or event in a room.
type RoomEvent struct {
	frame.BaseModel
	RoomID      string `gorm:"type:varchar(50)"`
	SenderID    string `gorm:"type:varchar(50)"`
	MessageType string
	Content     frame.JSONMap
	Metadata    frame.JSONMap
	DeletedAt   int64
}

// ToAPI converts RoomEvent model to API RoomEvent representation.
func (re *RoomEvent) ToAPI() *chatv1.RoomEvent {
	if re == nil {
		return nil
	}

	var payload *structpb.Struct
	if re.Metadata != nil {
		payload, _ = structpb.NewStruct(re.Metadata)
	}

	// Map message type to RoomEventType
	eventType := chatv1.RoomEventType_MESSAGE_TYPE_UNSPECIFIED
	if typeVal, ok := chatv1.RoomEventType_value[re.MessageType]; ok {
		eventType = chatv1.RoomEventType(typeVal)
	}

	return &chatv1.RoomEvent{
		Id:       re.GetID(),
		RoomId:   re.RoomID,
		SenderId: re.SenderID,
		Type:     eventType,
		Payload:  payload,
		SentAt:   timestamppb.New(re.CreatedAt),
		Edited:   false,
		Redacted: re.DeletedAt > 0,
	}
}

// RoomOutbox represents an outbox entry for message delivery tracking.
type RoomOutbox struct {
	frame.BaseModel
	RoomID         string `gorm:"type:varchar(50)"`
	SubscriptionID string `gorm:"type:varchar(50);index:idx_subscription_status"`
	EventID        string `gorm:"type:varchar(50)"`
	Status         string `json:"status" gorm:"index:idx_subscription_status"` // pending, sent, failed
	RetryCount     int    `json:"retry_count"`
	ErrorMessage   string `json:"error_message"`
}

// RoomSubscription represents a user's subscription to a room.
type RoomSubscription struct {
	frame.BaseModel
	RoomID               string `gorm:"type:varchar(50)"`
	ProfileID            string `gorm:"type:varchar(50)"`
	Role                 string
	IsActive             bool
	LastReadSequence     string
	LastReadAt           int64
	UnreadCount          int `gorm:"->;type:int;default:(SELECT COUNT(*) FROM room_outboxes WHERE room_outboxes.subscription_id = room_subscriptions.id AND room_outboxes.status = 'pending' AND room_outboxes.deleted_at = 0)"`
	NotificationsEnabled bool
	Metadata             frame.JSONMap
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
		RoomId:     rs.RoomID,
		ProfileId:  rs.ProfileID,
		Roles:      []string{rs.Role},
		JoinedAt:   timestamppb.New(rs.CreatedAt),
		LastActive: lastActive,
	}
}

// Address represents a physical address (from profile service).
type Address struct {
	frame.BaseModel
	Name      string `gorm:"type:varchar(250)"`
	AdminUnit string `gorm:"type:varchar(250)"`
	CountryID string `gorm:"type:varchar(3)"`
	Country   *Country
}

// ToAPI converts Address model to API representation.
func (a *Address) ToAPI() *profilev1.AddressObject {
	if a == nil {
		return nil
	}
	return &profilev1.AddressObject{
		Id:      a.GetID(),
		Name:    a.Name,
		Area:    a.AdminUnit,
		Country: a.CountryID,
	}
}

// ProfileAddress represents the link between a profile and an address.
type ProfileAddress struct {
	frame.BaseModel
	Name      string `gorm:"type:varchar(250)"`
	AddressID string `gorm:"type:varchar(50)"`
	ProfileID string `gorm:"type:varchar(50)"`
	Address   *Address
}

// Country represents a country entity.
type Country struct {
	ISO3 string `gorm:"primaryKey;type:varchar(3)"`
	ISO2 string `gorm:"type:varchar(2)"`
	Name string `gorm:"type:varchar(250)"`
}
