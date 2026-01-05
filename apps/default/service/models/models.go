package models

import (
	"context"
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
func (re *RoomEvent) ToAPI(_ context.Context, converter *PayloadConverter) *chatv1.RoomEvent {
	if re == nil {
		return nil
	}

	// Use PayloadConverter for complete conversion with typed content
	protoEvent := &chatv1.RoomEvent{
		Id:     re.ID,
		RoomId: re.RoomID,
		Source: &commonv1.ContactLink{
			ProfileId: re.SenderID,
		},
		Type: chatv1.RoomEventType(re.EventType),
	}

	// Set timestamp if available
	if !re.CreatedAt.IsZero() {
		protoEvent.SentAt = timestamppb.New(re.CreatedAt)
	}

	// Set parent ID if present
	if re.ParentID != "" {
		parentID := re.ParentID
		protoEvent.ParentId = &parentID
	}

	var err error
	protoEvent.Payload, err = converter.ToProto(re.Content)
	if err != nil {
		return nil
	}

	return protoEvent
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
	ContactID           string `gorm:"type:varchar(50)"`
	Role                string
	SubscriptionState   RoomSubscriptionState `gorm:"index:idx_roomsubscription_room_id_subscription_state"`
	LastReadEventID     string                `gorm:"type:varchar(50)"` // ID of the last read event (naturally time-sorted)
	LastReadAt          int64
	DisableNotification bool
	Properties          data.JSONMap
}

func (rs *RoomSubscription) ToLink() *commonv1.ContactLink {
	return &commonv1.ContactLink{
		ProfileId: rs.ProfileID,
		ContactId: rs.ContactID,
	}
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
		Id:         rs.GetID(),
		RoomId:     rs.RoomID,
		Member:     rs.ToLink(),
		Roles:      strings.Split(rs.Role, ","),
		JoinedAt:   timestamppb.New(rs.CreatedAt),
		LastActive: lastActive,
	}
}

func (rs *RoomSubscription) IsActive() bool {
	return RoomSubscriptionStateActive == rs.SubscriptionState
}

func (rs *RoomSubscription) Matches(contactLink *commonv1.ContactLink) bool {
	if contactLink == nil {
		return false
	}

	if rs.ProfileID != "" && contactLink.GetProfileId() != "" &&
		rs.ProfileID != contactLink.GetProfileId() {
		return false
	}

	if rs.ContactID != "" && contactLink.GetContactId() != "" &&
		rs.ContactID != contactLink.GetContactId() {
		return false
	}

	return true
}
