package deployment

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"connectrpc.com/connect"
	"github.com/pitabwire/util"
)

// MessageTypesTestSuite returns comprehensive tests for all message types.
func MessageTypesTestSuite() *TestSuite {
	return &TestSuite{
		Name:        "Message Types",
		Description: "Tests for all message content types (attachment, encrypted, call, motion, room change)",
		Tests: []TestCase{
			// Attachment tests
			&SendAttachmentMessageTest{},
			&SendAttachmentWithCaptionTest{},

			// Encrypted message tests
			&SendEncryptedMessageTest{},

			// Call signaling tests
			&SendCallOfferTest{},
			&SendCallEndTest{},

			// Motion/voting tests (approval workflow)
			&CreateMotionTest{},
			&CastVoteTest{},
			&MotionApprovalWorkflowTest{},

			// Room change event tests
			&RoomChangeEventTest{},
		},
	}
}

// SendAttachmentMessageTest tests sending an attachment message.
type SendAttachmentMessageTest struct{ BaseTestCase }

func (t *SendAttachmentMessageTest) Name() string        { return "SendEvent_Attachment" }
func (t *SendAttachmentMessageTest) Description() string { return "Send an attachment message" }
func (t *SendAttachmentMessageTest) Tags() []string      { return []string{"message", "attachment"} }

func (t *SendAttachmentMessageTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "attachment-test-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	// Send an attachment message
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
				Data: &chatv1.Payload_Attachment{
					Attachment: &chatv1.AttachmentContent{
						AttachmentId: util.IDString(),
						Filename:     "test-document.pdf",
						MimeType:     "application/pdf",
						SizeBytes:    1024,
						Uri:          "https://storage.example.com/attachments/test-document.pdf",
						Encrypted:    false,
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent attachment should succeed"); err != nil {
		return err
	}

	// Verify attachment appears in history
	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Check if attachment message is in history
	found := false
	for _, e := range history.GetEvents() {
		if attachment := e.GetPayload().GetAttachment(); attachment != nil {
			if attachment.GetFilename() == "test-document.pdf" {
				found = true
				break
			}
		}
	}

	if err := a.True(found, "Attachment message should appear in history"); err != nil {
		return err
	}

	return nil
}

// SendAttachmentWithCaptionTest tests sending an attachment with a caption.
type SendAttachmentWithCaptionTest struct{ BaseTestCase }

func (t *SendAttachmentWithCaptionTest) Name() string { return "SendEvent_AttachmentWithCaption" }
func (t *SendAttachmentWithCaptionTest) Description() string {
	return "Send an attachment with a text caption"
}
func (t *SendAttachmentWithCaptionTest) Tags() []string { return []string{"message", "attachment"} }

func (t *SendAttachmentWithCaptionTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "attachment-caption-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	captionText := "Check out this image!"
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
				Data: &chatv1.Payload_Attachment{
					Attachment: &chatv1.AttachmentContent{
						AttachmentId: util.IDString(),
						Filename:     "sunset-photo.jpg",
						MimeType:     "image/jpeg",
						SizeBytes:    2048576, // 2MB
						Uri:          "https://storage.example.com/images/sunset.jpg",
						Caption: &chatv1.TextContent{
							Body:   captionText,
							Format: "plain",
						},
						Previews: []*chatv1.AttachmentPreview{
							{
								Uri:      "https://storage.example.com/images/sunset-thumb.jpg",
								MimeType: "image/jpeg",
								Width:    200,
								Height:   150,
							},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent attachment with caption should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify attachment with caption
	found := false
	for _, e := range history.GetEvents() {
		if attachment := e.GetPayload().GetAttachment(); attachment != nil {
			if attachment.GetFilename() == "sunset-photo.jpg" && attachment.GetCaption().GetBody() == captionText {
				found = true
				break
			}
		}
	}

	if err := a.True(found, "Attachment with caption should appear in history"); err != nil {
		return err
	}

	return nil
}

// SendEncryptedMessageTest tests sending an encrypted message.
type SendEncryptedMessageTest struct{ BaseTestCase }

func (t *SendEncryptedMessageTest) Name() string { return "SendEvent_Encrypted" }
func (t *SendEncryptedMessageTest) Description() string {
	return "Send an end-to-end encrypted message"
}
func (t *SendEncryptedMessageTest) Tags() []string { return []string{"message", "encrypted", "e2ee"} }

func (t *SendEncryptedMessageTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "encrypted-test-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	// Simulate encrypted content (just a placeholder - real implementation would use actual encryption)
	plaintext := "This is a secret message"
	ciphertext := base64.StdEncoding.EncodeToString([]byte(plaintext))
	nonce := []byte("random-nonce-12b")

	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED,
				Data: &chatv1.Payload_Encrypted{
					Encrypted: &chatv1.EncryptedContent{
						Algorithm:       "x25519-aesgcm",
						Ciphertext:      []byte(ciphertext),
						Nonce:           nonce,
						RecipientKeyIds: []string{"recipient-key-001", "recipient-key-002"},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent encrypted should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify encrypted message is in history
	found := false
	for _, e := range history.GetEvents() {
		if encrypted := e.GetPayload().GetEncrypted(); encrypted != nil {
			if encrypted.GetAlgorithm() == "x25519-aesgcm" {
				found = true
				break
			}
		}
	}

	if err := a.True(found, "Encrypted message should appear in history"); err != nil {
		return err
	}

	return nil
}

// SendCallOfferTest tests sending a call offer event.
type SendCallOfferTest struct{ BaseTestCase }

func (t *SendCallOfferTest) Name() string        { return "SendEvent_CallOffer" }
func (t *SendCallOfferTest) Description() string { return "Send a WebRTC call offer" }
func (t *SendCallOfferTest) Tags() []string      { return []string{"message", "call", "webrtc"} }

func (t *SendCallOfferTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "call-test-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	callID := util.IDString()
	sdpOffer := "v=0\r\no=- 1234567890 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE audio\r\n"

	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED, // Call uses UNSPECIFIED
				Data: &chatv1.Payload_Call{
					Call: &chatv1.CallContent{
						CallId: callID,
						Type:   chatv1.CallContent_CALL_TYPE_AUDIO,
						Action: chatv1.CallContent_CALL_ACTION_OFFER,
						Sdp:    &sdpOffer,
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent call offer should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify call event is in history
	found := false
	for _, e := range history.GetEvents() {
		if call := e.GetPayload().GetCall(); call != nil {
			if call.GetCallId() == callID && call.GetAction() == chatv1.CallContent_CALL_ACTION_OFFER {
				found = true
				break
			}
		}
	}

	if err := a.True(found, "Call offer should appear in history"); err != nil {
		return err
	}

	return nil
}

// SendCallEndTest tests ending a call.
type SendCallEndTest struct{ BaseTestCase }

func (t *SendCallEndTest) Name() string        { return "SendEvent_CallEnd" }
func (t *SendCallEndTest) Description() string { return "End a WebRTC call" }
func (t *SendCallEndTest) Tags() []string      { return []string{"message", "call", "webrtc"} }

func (t *SendCallEndTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "call-end-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	callID := util.IDString()

	// First send an offer to start the call
	sdpOffer := "v=0\r\no=- 1234567890 1 IN IP4 127.0.0.1\r\n"
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Call{
					Call: &chatv1.CallContent{
						CallId: callID,
						Type:   chatv1.CallContent_CALL_TYPE_VIDEO,
						Action: chatv1.CallContent_CALL_ACTION_OFFER,
						Sdp:    &sdpOffer,
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent call offer should succeed"); err != nil {
		return err
	}

	// Now end the call
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_CALL,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Call{
					Call: &chatv1.CallContent{
						CallId: callID,
						Type:   chatv1.CallContent_CALL_TYPE_VIDEO,
						Action: chatv1.CallContent_CALL_ACTION_END,
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent call end should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify both call events are in history
	offerFound := false
	endFound := false
	for _, e := range history.GetEvents() {
		if call := e.GetPayload().GetCall(); call != nil {
			if call.GetCallId() == callID {
				if call.GetAction() == chatv1.CallContent_CALL_ACTION_OFFER {
					offerFound = true
				}
				if call.GetAction() == chatv1.CallContent_CALL_ACTION_END {
					endFound = true
				}
			}
		}
	}

	if err := a.True(offerFound, "Call offer should be in history"); err != nil {
		return err
	}
	if err := a.True(endFound, "Call end should be in history"); err != nil {
		return err
	}

	return nil
}

// CreateMotionTest tests creating a motion for voting.
type CreateMotionTest struct{ BaseTestCase }

func (t *CreateMotionTest) Name() string        { return "SendEvent_Motion" }
func (t *CreateMotionTest) Description() string { return "Create a motion for voting/approval" }
func (t *CreateMotionTest) Tags() []string      { return []string{"message", "motion", "voting"} }

func (t *CreateMotionTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "motion-test-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()
	targetVotes := uint32(2)

	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "Approve expense report",
						Description:   "Vote to approve the Q1 2026 expense report for marketing team",
						EligibleRoles: []string{"admin", "moderator"},
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Absolute{Absolute: targetVotes},
							PassingChoiceId: "yes",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "yes", Name: "Yes", Description: ptr("Approve the expense report")},
							{Id: "no", Name: "No", Description: ptr("Reject the expense report")},
							{Id: "abstain", Name: "Abstain", Description: ptr("Neither approve nor reject")},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent motion should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify motion is in history
	found := false
	for _, e := range history.GetEvents() {
		if motion := e.GetPayload().GetMotion(); motion != nil {
			if motion.GetId() == motionID && motion.GetTitle() == "Approve expense report" {
				found = true
				// Verify choices
				if len(motion.GetChoices()) != 3 {
					return &AssertionError{Message: fmt.Sprintf("Expected 3 choices, got %d", len(motion.GetChoices()))}
				}
				break
			}
		}
	}

	if err := a.True(found, "Motion should appear in history"); err != nil {
		return err
	}

	return nil
}

// CastVoteTest tests casting a vote on a motion.
type CastVoteTest struct{ BaseTestCase }

func (t *CastVoteTest) Name() string        { return "SendEvent_Vote" }
func (t *CastVoteTest) Description() string { return "Cast a vote on a motion" }
func (t *CastVoteTest) Tags() []string      { return []string{"message", "motion", "voting"} }

func (t *CastVoteTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "vote-test-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()

	// First create a motion
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:          motionID,
						Title:       "Simple yes/no vote",
						Description: "Test motion for voting",
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Absolute{Absolute: 1},
							PassingChoiceId: "yes",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "yes", Name: "Yes"},
							{Id: "no", Name: "No"},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent motion should succeed"); err != nil {
		return err
	}

	// Cast a vote
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT, // Votes are events
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Vote{
					Vote: &chatv1.VoteCast{
						MotionId: motionID,
						ChoiceId: "yes",
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent vote should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify vote is in history
	voteFound := false
	for _, e := range history.GetEvents() {
		if vote := e.GetPayload().GetVote(); vote != nil {
			if vote.GetMotionId() == motionID && vote.GetChoiceId() == "yes" {
				voteFound = true
				break
			}
		}
	}

	if err := a.True(voteFound, "Vote should appear in history"); err != nil {
		return err
	}

	return nil
}

// MotionApprovalWorkflowTest tests a complete motion approval workflow.
type MotionApprovalWorkflowTest struct{ BaseTestCase }

func (t *MotionApprovalWorkflowTest) Name() string { return "Motion_ApprovalWorkflow" }
func (t *MotionApprovalWorkflowTest) Description() string {
	return "Complete motion approval workflow with multiple votes"
}
func (t *MotionApprovalWorkflowTest) Tags() []string {
	return []string{"message", "motion", "voting", "workflow"}
}

func (t *MotionApprovalWorkflowTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "approval-workflow-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()

	// Step 1: Create a motion that requires 2 yes votes to pass
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "Approve external system action",
						Description:   "This action requires approval from 2 authorized users before execution",
						EligibleRoles: []string{"admin", "approver"},
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Absolute{Absolute: 2},
							PassingChoiceId: "approve",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "approve", Name: "Approve", Description: ptr("Approve the action")},
							{Id: "reject", Name: "Reject", Description: ptr("Reject the action")},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent motion should succeed"); err != nil {
		return err
	}

	// Step 2: First approver votes yes
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Vote{
					Vote: &chatv1.VoteCast{
						MotionId: motionID,
						ChoiceId: "approve",
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "First vote should succeed"); err != nil {
		return err
	}

	// Step 3: Second approver votes yes
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Vote{
					Vote: &chatv1.VoteCast{
						MotionId: motionID,
						ChoiceId: "approve",
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Second vote should succeed"); err != nil {
		return err
	}

	// Step 4: Send tally update (simulating what the server might compute)
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_MotionTally{
					MotionTally: &chatv1.MotionTally{
						MotionId:       motionID,
						EligibleVotes:  5, // e.g., 5 total eligible voters
						TotalVotesCast: 2,
						TargetVotes:    2,
						Passed:         true,
						Closed:         true,
						Tallies: []*chatv1.VoteTally{
							{ChoiceId: "approve", Count: 2},
							{ChoiceId: "reject", Count: 0},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Motion tally should succeed"); err != nil {
		return err
	}

	time.Sleep(300 * time.Millisecond)

	// Verify the complete workflow is in history
	history, err := client.GetHistory(ctx, room.GetId(), 20, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	motionFound := false
	voteCount := 0
	tallyFound := false

	for _, e := range history.GetEvents() {
		if motion := e.GetPayload().GetMotion(); motion != nil && motion.GetId() == motionID {
			motionFound = true
		}
		if vote := e.GetPayload().GetVote(); vote != nil && vote.GetMotionId() == motionID {
			voteCount++
		}
		if tally := e.GetPayload().GetMotionTally(); tally != nil && tally.GetMotionId() == motionID {
			tallyFound = true
			if !tally.GetPassed() {
				return &AssertionError{Message: "Tally should show passed=true"}
			}
			if !tally.GetClosed() {
				return &AssertionError{Message: "Tally should show closed=true"}
			}
		}
	}

	if err := a.True(motionFound, "Motion should be in history"); err != nil {
		return err
	}
	if err := a.MinLen(voteCount, 2, "Should have at least 2 votes"); err != nil {
		return err
	}
	if err := a.True(tallyFound, "Motion tally should be in history"); err != nil {
		return err
	}

	return nil
}

// RoomChangeEventTest tests sending room change events.
type RoomChangeEventTest struct{ BaseTestCase }

func (t *RoomChangeEventTest) Name() string        { return "SendEvent_RoomChange" }
func (t *RoomChangeEventTest) Description() string { return "Send room change events" }
func (t *RoomChangeEventTest) Tags() []string      { return []string{"message", "room", "system"} }

func (t *RoomChangeEventTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "room-change-test", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	// Get the creator's subscription ID
	subs, err := client.SearchSubscriptions(ctx, room.GetId())
	if err := a.NoError(err, "SearchSubscriptions should succeed"); err != nil {
		return err
	}
	if len(subs) == 0 {
		return &AssertionError{Message: "Should have at least one subscription"}
	}
	actorSubID := subs[0].GetId()

	// Send a room change event for an update
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_SYSTEM,
			Payload: &chatv1.Payload{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ROOM_CHANGE,
				Data: &chatv1.Payload_RoomChange{
					RoomChange: &chatv1.RoomChangeContent{
						Action:              chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_UPDATED,
						ActorSubscriptionId: actorSubID,
						Body:                "Room settings were updated",
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "SendEvent room change should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	// Verify room change event is in history
	found := false
	for _, e := range history.GetEvents() {
		if rc := e.GetPayload().GetRoomChange(); rc != nil {
			if rc.GetAction() == chatv1.RoomChangeAction_ROOM_CHANGE_ACTION_UPDATED {
				found = true
				break
			}
		}
	}

	if err := a.True(found, "Room change event should appear in history"); err != nil {
		return err
	}

	return nil
}

// Helper function to create a string pointer.
func ptr(s string) *string {
	return &s
}
