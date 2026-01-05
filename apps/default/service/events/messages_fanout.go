package events

import (
	"context"
	"errors"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const RoomFanoutEventName = "room.message.fanout.event"

type FanoutEventHandler struct {
	cfg           *config.ChatConfig
	eventRepo     repository.RoomEventRepository
	queueMan      queue.Manager
	deliveryTopic queue.Publisher
}

func NewFanoutEventHandler(
	ctx context.Context,
	cfg *config.ChatConfig,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	queueMan queue.Manager,
) *FanoutEventHandler {
	return &FanoutEventHandler{
		cfg:       cfg,
		queueMan:  queueMan,
		eventRepo: repository.NewRoomEventRepository(ctx, dbPool, workMan),
	}
}

func (feh *FanoutEventHandler) getTopic() (queue.Publisher, error) {
	if feh.deliveryTopic != nil {
		return feh.deliveryTopic, nil
	}

	var err error
	feh.deliveryTopic, err = feh.queueMan.GetPublisher(feh.cfg.QueueDeviceEventDeliveryName)
	if err != nil {
		return nil, err
	}
	return feh.deliveryTopic, nil
}

func (feh *FanoutEventHandler) Name() string {
	return RoomFanoutEventName
}

func (feh *FanoutEventHandler) PayloadType() any {
	return &eventsv1.Broadcast{}
}

func (feh *FanoutEventHandler) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.Broadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.Broadcast")
	}
	return nil
}

func (feh *FanoutEventHandler) Execute(ctx context.Context, payload any) error {
	broadcast, ok := payload.(*eventsv1.Broadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.Broadcast{}")
	}

	eventLink := broadcast.GetEvent()
	destinations := broadcast.GetDestinations()

	// Early exit if no destinations
	if len(destinations) == 0 {
		return nil
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id":      eventLink.GetRoomId(),
		"target_count": len(destinations),
	})

	// Get event data to extract payload
	eventLinkData, err := feh.eventRepo.GetByID(ctx, eventLink.GetEventId())
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such chat event exists")
			return nil
		}
		logger.WithError(err).Error("failed to get chat event data")
		return err
	}

	deliveryTopic, err := feh.getTopic()
	if err != nil {
		logger.WithError(err).Error("failed to get topic")
		return err
	}

	// Convert event content to typed payload
	converter := models.NewPayloadConverter()
	eventPayload, payloadErr := converter.ToProto(eventLinkData.Content)
	if payloadErr != nil {
		logger.WithError(payloadErr).Error("failed to convert event content to payload")
		// Continue with nil payload rather than failing entirely
	}

	// Publish all deliveries - continue on individual failures
	var failCount int
	for _, destination := range destinations {
		eventDelivery := &eventsv1.Delivery{
			Event:        eventLink,
			Destination:  destination,
			Source:       broadcast.GetSource(),
			Payload:      eventPayload,
			IsCompressed: false,
			RetryCount:   0,
		}

		if pubErr := deliveryTopic.Publish(ctx, eventDelivery); pubErr != nil {
			failCount++
			logger.WithError(pubErr).
				WithField("destination", destination.GetProfileId()).
				Warn("failed to publish delivery")
		}
	}

	if failCount > 0 {
		logger.WithField("fail_count", failCount).Warn("some deliveries failed")
	}

	return nil
}
