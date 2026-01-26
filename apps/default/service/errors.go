package service

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// PartialBatchError represents an error where some items in a batch
// succeeded while others failed. This allows callers to distinguish
// between total failures and partial successes.
type PartialBatchError struct {
	Succeeded int
	Failed    int
	Errors    []ItemError
}

// ItemError describes a failure for a specific item in a batch operation.
type ItemError struct {
	Index   int
	ItemID  string
	Message string
}

func (e *PartialBatchError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "partial batch failure: %d succeeded, %d failed", e.Succeeded, e.Failed)
	for _, ie := range e.Errors {
		fmt.Fprintf(&sb, "; item %d (%s): %s", ie.Index, ie.ItemID, ie.Message)
	}
	return sb.String()
}

// IsPartialBatchError checks whether err is a PartialBatchError.
func IsPartialBatchError(err error) (*PartialBatchError, bool) {
	var pbe *PartialBatchError
	if errors.As(err, &pbe) {
		return pbe, true
	}
	return nil, false
}

var (
	ErrUnspecifiedID      = connect.NewError(connect.CodeInvalidArgument, errors.New("no id was supplied"))
	ErrEmptyValueSupplied = connect.NewError(connect.CodeInvalidArgument, errors.New("empty value supplied"))
	ErrItemExist          = connect.NewError(connect.CodeAlreadyExists, errors.New("specified item already exists"))
	ErrItemDoesNotExist   = connect.NewError(connect.CodeNotFound, errors.New("specified item does not exist"))

	// ErrRoomNameRequired is returned when room name is not provided.
	ErrRoomNameRequired    = connect.NewError(connect.CodeInvalidArgument, errors.New("room name is required"))
	ErrRoomIDRequired      = connect.NewError(connect.CodeInvalidArgument, errors.New("room ID is required"))
	ErrRoomMembersRequired = connect.NewError(
		connect.CodeInvalidArgument,
		errors.New("at least one member is required"),
	)
	ErrRoomNotFound     = connect.NewError(connect.CodeNotFound, errors.New("room not found"))
	ErrRoomAccessDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have access to this room"),
	)
	ErrRoomUpdateDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("only room admins can update the room"),
	)
	ErrRoomDeleteDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("only room owners can delete the room"),
	)
	ErrRoomAddMembersDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to add members to this room"),
	)
	ErrRoomRemoveMembersDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to remove members from this room"),
	)
	ErrRoomUpdateRoleDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to update member roles"),
	)
	ErrRoomMemberNotFound = connect.NewError(connect.CodeNotFound, errors.New("member not found in room"))

	// ErrMessageRoomIDRequired is returned when message room ID is not provided.
	ErrMessageRoomIDRequired  = connect.NewError(connect.CodeInvalidArgument, errors.New("room ID is required"))
	ErrMessageContentRequired = connect.NewError(connect.CodeInvalidArgument, errors.New("message content is required"))
	ErrMessageNotFound        = connect.NewError(connect.CodeNotFound, errors.New("message not found"))
	ErrMessageAccessDenied    = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have access to this message"),
	)
	ErrMessageDeleteDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to delete this message"),
	)
	ErrMessageSendDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to send messages to this room"),
	)

	// ErrProfileIDsRequired is returned when profile IDs are not provided.
	ErrProfileIDsRequired = connect.NewError(
		connect.CodeInvalidArgument,
		errors.New("at least one profile ID is required"),
	)
	ErrRoleRequired         = connect.NewError(connect.CodeInvalidArgument, errors.New("subscription Role is required"))
	ErrSubscriptionNotFound = connect.NewError(connect.CodeNotFound, errors.New("subscription not found"))

	// ErrProposalNotFound is returned when a proposal is not found.
	ErrProposalNotFound   = connect.NewError(connect.CodeNotFound, errors.New("proposal not found"))
	ErrProposalNotPending = connect.NewError(
		connect.CodeFailedPrecondition,
		errors.New("proposal is not in pending state"),
	)
	ErrProposalExpired = connect.NewError(
		connect.CodeFailedPrecondition,
		errors.New("proposal has expired"),
	)
	ErrProposalApprovalDenied = connect.NewError(
		connect.CodePermissionDenied,
		errors.New("you don't have permission to approve or reject proposals"),
	)
	ErrProposalRequired = connect.NewError(
		connect.CodeFailedPrecondition,
		errors.New("this operation requires approval; a proposal has been created"),
	)
)
