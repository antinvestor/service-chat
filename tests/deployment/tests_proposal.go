package deployment

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"connectrpc.com/connect"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/structpb"
)

// ProposalTestSuite returns tests for the proposal/approval workflow system.
// Proposals are pending changes that require approval before execution.
// This applies to rooms with RequiresApproval=true.
func ProposalTestSuite() *TestSuite {
	return &TestSuite{
		Name:        "Proposal Operations",
		Description: "Tests for proposal/approval workflows on protected rooms",
		Tests: []TestCase{
			// Room with approval tests
			&CreateRoomWithApprovalFlagTest{},
			&RoomMetadataApprovalFlagTest{},

			// Proposal API tests
			&ListProposalsEmptyTest{},
			&ListProposalsRequiresRoomIDTest{},

			// Motion-based approval workflow (available via message API)
			&MotionBasedApprovalTest{},
			&MotionWithPercentageRuleTest{},
			&MotionWithDeadlineTest{},

			// Multi-approver workflow tests
			&MultiApproverMotionTest{},
			&VoteChangeNotAllowedTest{},
		},
	}
}

// CreateRoomWithApprovalFlagTest tests creating a room with the requires_approval metadata flag.
type CreateRoomWithApprovalFlagTest struct{ BaseTestCase }

func (t *CreateRoomWithApprovalFlagTest) Name() string { return "CreateRoom_WithApprovalFlag" }
func (t *CreateRoomWithApprovalFlagTest) Description() string {
	return "Create a room with requires_approval metadata flag"
}
func (t *CreateRoomWithApprovalFlagTest) Tags() []string {
	return []string{"proposal", "room", "approval"}
}

func (t *CreateRoomWithApprovalFlagTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	// Create metadata with requires_approval flag
	metadata, err := structpb.NewStruct(map[string]any{
		"requires_approval": true,
		"approval_type":     "owner_only",
	})
	if err != nil {
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	roomID := util.IDString()
	resp, err := client.Chat().CreateRoom(ctx, connect.NewRequest(&chatv1.CreateRoomRequest{
		Id:          roomID,
		Name:        client.config.TestDataPrefix + "approval-room",
		Description: "Room that requires approval for changes",
		IsPrivate:   true,
		Metadata:    metadata,
	}))
	if err := a.NoError(err, "CreateRoom with approval flag should succeed"); err != nil {
		return err
	}

	room := resp.Msg.GetRoom()
	client.TrackRoom(room.GetId())

	if err := a.NotNil(room, "Room should be created"); err != nil {
		return err
	}

	// Check if metadata was stored (the service may or may not honor the flag)
	roomMeta := room.GetMetadata()
	if roomMeta != nil {
		// If metadata is returned, verify the flag is present
		if reqApproval := roomMeta.GetFields()["requires_approval"]; reqApproval != nil {
			// Flag was accepted
			if err := a.True(reqApproval.GetBoolValue(), "requires_approval should be true"); err != nil {
				return err
			}
		}
	}

	return nil
}

// RoomMetadataApprovalFlagTest tests that room metadata can indicate approval requirements.
type RoomMetadataApprovalFlagTest struct{ BaseTestCase }

func (t *RoomMetadataApprovalFlagTest) Name() string { return "Room_MetadataApprovalFlag" }
func (t *RoomMetadataApprovalFlagTest) Description() string {
	return "Verify room metadata can contain approval configuration"
}
func (t *RoomMetadataApprovalFlagTest) Tags() []string {
	return []string{"proposal", "room", "metadata"}
}

func (t *RoomMetadataApprovalFlagTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	// Create a room with comprehensive approval metadata
	metadata, _ := structpb.NewStruct(map[string]any{
		"requires_approval": true,
		"approval_config": map[string]any{
			"min_approvers":    2,
			"approval_roles":   []any{"admin", "owner"},
			"expiry_hours":     72,
			"notify_on_create": true,
		},
	})

	resp, err := client.Chat().CreateRoom(ctx, connect.NewRequest(&chatv1.CreateRoomRequest{
		Id:          util.IDString(),
		Name:        client.config.TestDataPrefix + "approval-config-room",
		Description: "Room with detailed approval configuration",
		IsPrivate:   true,
		Metadata:    metadata,
	}))
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	room := resp.Msg.GetRoom()
	client.TrackRoom(room.GetId())

	// Verify the room was created
	if err := a.NotEmpty(room.GetId(), "Room ID should not be empty"); err != nil {
		return err
	}

	return nil
}

// ListProposalsEmptyTest tests that listing proposals on a room with none returns empty.
type ListProposalsEmptyTest struct{ BaseTestCase }

func (t *ListProposalsEmptyTest) Name() string { return "ListProposals_Empty" }
func (t *ListProposalsEmptyTest) Description() string {
	return "List proposals on a room with no pending proposals"
}
func (t *ListProposalsEmptyTest) Tags() []string { return []string{"proposal", "api"} }

func (t *ListProposalsEmptyTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	// Create a regular room (no approval requirement)
	room, err := client.CreateTestRoom(ctx, "list-proposals-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	// List proposals - should be empty
	resp, err := client.Chat().ListProposals(ctx, connect.NewRequest(&chatv1.ListProposalsRequest{
		RoomId: room.GetId(),
	}))
	if err := a.NoError(err, "ListProposals should succeed"); err != nil {
		return err
	}

	proposals := resp.Msg.GetProposals()
	if err := a.Equal(len(proposals), 0, "Should have no proposals"); err != nil {
		return err
	}

	return nil
}

// ListProposalsRequiresRoomIDTest tests that listing proposals requires a room ID.
type ListProposalsRequiresRoomIDTest struct{ BaseTestCase }

func (t *ListProposalsRequiresRoomIDTest) Name() string { return "ListProposals_RequiresRoomID" }
func (t *ListProposalsRequiresRoomIDTest) Description() string {
	return "Verify ListProposals validates room_id is provided"
}
func (t *ListProposalsRequiresRoomIDTest) Tags() []string { return []string{"proposal", "api", "validation"} }

func (t *ListProposalsRequiresRoomIDTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	// Try to list proposals without room_id
	_, err := client.Chat().ListProposals(ctx, connect.NewRequest(&chatv1.ListProposalsRequest{
		RoomId: "", // Empty room ID
	}))

	if err := a.Error(err, "ListProposals without room_id should fail"); err != nil {
		return err
	}

	// Verify it's a validation error
	if err := a.True(
		strings.Contains(err.Error(), "room_id") || strings.Contains(err.Error(), "invalid"),
		"Error should mention room_id or validation",
	); err != nil {
		return err
	}

	return nil
}

// MotionBasedApprovalTest tests creating a motion for approval decisions.
type MotionBasedApprovalTest struct{ BaseTestCase }

func (t *MotionBasedApprovalTest) Name() string { return "Motion_ApprovalDecision" }
func (t *MotionBasedApprovalTest) Description() string {
	return "Use motion for group approval decision"
}
func (t *MotionBasedApprovalTest) Tags() []string { return []string{"proposal", "motion", "approval"} }

func (t *MotionBasedApprovalTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "motion-approval-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()

	// Create a motion representing a proposal that needs approval
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "Proposal: Add new feature to project",
						Description:   "This proposal requires approval from project administrators before implementation can proceed.",
						EligibleRoles: []string{"admin", "owner"},
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Absolute{Absolute: 1},
							PassingChoiceId: "approve",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "approve", Name: "Approve", Description: ptr("Approve this proposal")},
							{Id: "reject", Name: "Reject", Description: ptr("Reject this proposal")},
							{Id: "defer", Name: "Defer", Description: ptr("Defer decision to later")},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Creating motion should succeed"); err != nil {
		return err
	}

	// Cast an approval vote
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
	if err := a.NoError(err, "Casting vote should succeed"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	// Verify the motion and vote are in history
	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	motionFound := false
	voteFound := false
	for _, e := range history.GetEvents() {
		if motion := e.GetPayload().GetMotion(); motion != nil && motion.GetId() == motionID {
			motionFound = true
		}
		if vote := e.GetPayload().GetVote(); vote != nil && vote.GetMotionId() == motionID {
			voteFound = true
		}
	}

	if err := a.True(motionFound, "Motion should be in history"); err != nil {
		return err
	}
	if err := a.True(voteFound, "Vote should be in history"); err != nil {
		return err
	}

	return nil
}

// MotionWithPercentageRuleTest tests motion with percentage-based passing rule.
type MotionWithPercentageRuleTest struct{ BaseTestCase }

func (t *MotionWithPercentageRuleTest) Name() string { return "Motion_PercentageRule" }
func (t *MotionWithPercentageRuleTest) Description() string {
	return "Motion with percentage-based approval threshold"
}
func (t *MotionWithPercentageRuleTest) Tags() []string {
	return []string{"proposal", "motion", "voting"}
}

func (t *MotionWithPercentageRuleTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "percentage-vote-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()
	eligibleVotes := uint32(10)

	// Create a motion requiring 60% approval
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "Budget Allocation Proposal",
						Description:   "Requires 60% approval from all eligible voters",
						EligibleRoles: []string{"member", "admin", "owner"},
						EligibleVotes: &eligibleVotes,
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Percentage{Percentage: 60},
							PassingChoiceId: "yes",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "yes", Name: "Yes"},
							{Id: "no", Name: "No"},
							{Id: "abstain", Name: "Abstain"},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Creating motion with percentage rule should succeed"); err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)

	// Verify motion was created
	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	motionFound := false
	for _, e := range history.GetEvents() {
		if motion := e.GetPayload().GetMotion(); motion != nil && motion.GetId() == motionID {
			motionFound = true
			// Verify the percentage rule was set
			if rule := motion.GetPassingRule(); rule != nil {
				if err := a.Equal(int(rule.GetPercentage()), 60, "Percentage should be 60"); err != nil {
					return err
				}
			}
			break
		}
	}

	if err := a.True(motionFound, "Motion should be in history"); err != nil {
		return err
	}

	return nil
}

// MotionWithDeadlineTest tests motion with expiration deadline.
type MotionWithDeadlineTest struct{ BaseTestCase }

func (t *MotionWithDeadlineTest) Name() string        { return "Motion_WithDeadline" }
func (t *MotionWithDeadlineTest) Description() string { return "Motion with voting deadline" }
func (t *MotionWithDeadlineTest) Tags() []string      { return []string{"proposal", "motion", "deadline"} }

func (t *MotionWithDeadlineTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "deadline-vote-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()
	// Set deadline to 24 hours from now
	closesAt := time.Now().Add(24 * time.Hour).Unix()

	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "Time-Limited Proposal",
						Description:   "This proposal must be decided within 24 hours",
						EligibleRoles: []string{"member"},
						ClosesAt:      &closesAt,
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
	if err := a.NoError(err, "Creating motion with deadline should succeed"); err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)

	// Verify motion was created with deadline
	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	motionFound := false
	for _, e := range history.GetEvents() {
		if motion := e.GetPayload().GetMotion(); motion != nil && motion.GetId() == motionID {
			motionFound = true
			if err := a.True(motion.HasClosesAt(), "Motion should have closes_at set"); err != nil {
				return err
			}
			break
		}
	}

	if err := a.True(motionFound, "Motion should be in history"); err != nil {
		return err
	}

	return nil
}

// MultiApproverMotionTest tests motion requiring multiple approvers.
type MultiApproverMotionTest struct{ BaseTestCase }

func (t *MultiApproverMotionTest) Name() string        { return "Motion_MultipleApprovers" }
func (t *MultiApproverMotionTest) Description() string { return "Motion requiring multiple approvals" }
func (t *MultiApproverMotionTest) Tags() []string {
	return []string{"proposal", "motion", "multi-approval"}
}

func (t *MultiApproverMotionTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "multi-approver-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()

	// Create motion requiring 3 approvals
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:            motionID,
						Title:         "High-Impact Change Request",
						Description:   "This change requires approval from at least 3 administrators",
						EligibleRoles: []string{"admin", "owner"},
						PassingRule: &chatv1.PassingRule{
							Rule:            &chatv1.PassingRule_Absolute{Absolute: 3},
							PassingChoiceId: "approve",
						},
						Choices: []*chatv1.VoteChoice{
							{Id: "approve", Name: "Approve"},
							{Id: "reject", Name: "Reject"},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Creating multi-approver motion should succeed"); err != nil {
		return err
	}

	// Cast first vote
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

	// Cast second vote
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

	// Send tally showing 2/3 approvals (not yet passed)
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_MotionTally{
					MotionTally: &chatv1.MotionTally{
						MotionId:       motionID,
						EligibleVotes:  5,
						TotalVotesCast: 2,
						TargetVotes:    3,
						Passed:         false, // Not yet passed
						Closed:         false,
						Tallies: []*chatv1.VoteTally{
							{ChoiceId: "approve", Count: 2},
							{ChoiceId: "reject", Count: 0},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Intermediate tally should succeed"); err != nil {
		return err
	}

	// Cast third vote to reach threshold
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
	if err := a.NoError(err, "Third vote should succeed"); err != nil {
		return err
	}

	// Send final tally showing motion passed
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_MotionTally{
					MotionTally: &chatv1.MotionTally{
						MotionId:       motionID,
						EligibleVotes:  5,
						TotalVotesCast: 3,
						TargetVotes:    3,
						Passed:         true, // Now passed!
						Closed:         true,
						Tallies: []*chatv1.VoteTally{
							{ChoiceId: "approve", Count: 3},
							{ChoiceId: "reject", Count: 0},
						},
					},
				},
			},
		}},
	}))
	if err := a.NoError(err, "Final tally should succeed"); err != nil {
		return err
	}

	time.Sleep(300 * time.Millisecond)

	// Verify complete workflow in history
	history, err := client.GetHistory(ctx, room.GetId(), 20, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	voteCount := 0
	tallyCount := 0
	passedTally := false

	for _, e := range history.GetEvents() {
		if vote := e.GetPayload().GetVote(); vote != nil && vote.GetMotionId() == motionID {
			voteCount++
		}
		if tally := e.GetPayload().GetMotionTally(); tally != nil && tally.GetMotionId() == motionID {
			tallyCount++
			if tally.GetPassed() {
				passedTally = true
			}
		}
	}

	if err := a.MinLen(voteCount, 3, "Should have at least 3 votes"); err != nil {
		return err
	}
	if err := a.MinLen(tallyCount, 1, "Should have at least 1 tally"); err != nil {
		return err
	}
	if err := a.True(passedTally, "Should have a passing tally"); err != nil {
		return err
	}

	return nil
}

// VoteChangeNotAllowedTest tests that vote changes are tracked separately.
type VoteChangeNotAllowedTest struct{ BaseTestCase }

func (t *VoteChangeNotAllowedTest) Name() string        { return "Motion_VoteTracking" }
func (t *VoteChangeNotAllowedTest) Description() string { return "Track multiple votes from same user" }
func (t *VoteChangeNotAllowedTest) Tags() []string      { return []string{"proposal", "motion", "voting"} }

func (t *VoteChangeNotAllowedTest) Run(ctx context.Context, client *Client) error {
	var a Assert

	room, err := client.CreateTestRoom(ctx, "vote-tracking-room", nil)
	if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
		return err
	}

	motionID := util.IDString()

	// Create a motion
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MOTION,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Motion{
					Motion: &chatv1.MotionContent{
						Id:          motionID,
						Title:       "Vote Tracking Test",
						Description: "Testing vote tracking behavior",
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
	if err := a.NoError(err, "Creating motion should succeed"); err != nil {
		return err
	}

	// Cast first vote (yes)
	_, err = client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
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
	if err := a.NoError(err, "First vote should succeed"); err != nil {
		return err
	}

	// Attempt to cast a different vote (no) - behavior may vary by implementation
	// Some systems allow vote changes, others don't
	_, secondVoteErr := client.Chat().SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{{
			Id:     util.IDString(),
			RoomId: room.GetId(),
			Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
			Payload: &chatv1.Payload{
				Data: &chatv1.Payload_Vote{
					Vote: &chatv1.VoteCast{
						MotionId: motionID,
						ChoiceId: "no",
					},
				},
			},
		}},
	}))

	// Either success (vote change allowed) or error (vote change not allowed) is acceptable
	// The key is that the system handles it gracefully
	if secondVoteErr != nil {
		// Vote change was rejected - verify it's a proper error
		errStr := secondVoteErr.Error()
		// Should be a validation error, not a system error
		if strings.Contains(errStr, "internal") {
			return fmt.Errorf("vote change rejection should not be internal error: %w", secondVoteErr)
		}
	}

	time.Sleep(200 * time.Millisecond)

	// Verify votes are tracked in history
	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err := a.NoError(err, "GetHistory should succeed"); err != nil {
		return err
	}

	voteCount := 0
	for _, e := range history.GetEvents() {
		if vote := e.GetPayload().GetVote(); vote != nil && vote.GetMotionId() == motionID {
			voteCount++
		}
	}

	// Should have at least the first vote
	if err := a.MinLen(voteCount, 1, "Should have at least 1 vote in history"); err != nil {
		return err
	}

	return nil
}
