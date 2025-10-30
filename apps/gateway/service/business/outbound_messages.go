package business

import (
	"context"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/util"
	"gocloud.dev/pubsub"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// handleOutboundRequests processes messages from rooms within server.
func (cm *connectionManager) handleOutboundRequests(
	ctx context.Context,
	conn *Connection,
	req *eventsv1.EventDelivery,
) error {

	evt := req.GetEvent()
	target := req.GetTarget()

	payload := &chatv1.ServerEvent{
		Id:        target.GetTargetId(),
		Timestamp: timestamppb.Now(),
		Payload: &chatv1.ServerEvent_Message{
			Message: &chatv1.RoomEvent{
				Id:       evt.GetEventId(),
				Type:     chatv1.RoomEventType(req.Event.GetEventType().Number()),
				Payload:  req.GetPayload(),
				SentAt:   evt.GetCreatedAt(),
				Edited:   false,
				Redacted: false,
			},
		},
	}

	err := conn.stream.Send(payload)
	if err != nil {
		util.Log(ctx).WithError(err).Info("Failed to deliver an event")
		return err
	}
	return nil
}

func toEventDelivery(req *pubsub.Message) (*eventsv1.EventDelivery, error) {

	EventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(req.Body, EventDelivery)
	if err != nil {
		return nil, err
	}

	return EventDelivery, nil
}
