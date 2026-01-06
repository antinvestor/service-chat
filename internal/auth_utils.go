package internal

import (
	"context"
	"errors"
	"slices"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"connectrpc.com/connect"
	"github.com/pitabwire/frame/security"
)

// AuthContactLink extracts contact link from validated authentication claims.
func AuthContactLink(ctx context.Context, roles ...string) (*commonv1.ContactLink, error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("request needs to be authenticated"),
		)
	}

	for _, item := range roles {
		if !slices.Contains(authClaims.GetRoles(), item) {
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("request not authorized"),
			)
		}
	}

	profileID, err := authClaims.GetSubject()
	if err != nil || profileID == "" {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("invalid authentication claims"),
		)
	}

	contactID := authClaims.GetContactID()
	if contactID == "" {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("invalid authentication claims"),
		)
	}

	return &commonv1.ContactLink{
		ProfileId: profileID,
		ContactId: contactID,
	}, nil
}

func IsValidContactLink(contactLink *commonv1.ContactLink) error {
	if contactLink == nil {
		return connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("no contact link specified"),
		)
	}

	// Contact is the primary identifier and should be sufficient
	if contactLink.GetContactId() == "" && contactLink.GetDetail() == "" {
		return connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("no contact specified"),
		)
	}

	return nil
}
