package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/pitabwire/frame"
	"google.golang.org/protobuf/proto"
)

const (
	bearerScheme     = "Bearer"
	bearerTokenParts = 2
	grpcAuthHeader   = "authorization"
)

// AuthInterceptor implements connect.Interceptor for authentication.
type AuthInterceptor struct {
	svc      *frame.Service
	audience string
	issuer   string
}

// NewAuthInterceptor creates a new authentication interceptor.
func NewAuthInterceptor(svc *frame.Service, audience string, issuer string) *AuthInterceptor {
	return &AuthInterceptor{
		svc:      svc,
		audience: audience,
		issuer:   issuer,
	}
}

// WrapUnary implements the unary interceptor for authentication.
func (a *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return connect.UnaryFunc(func(
		ctx context.Context,
		req connect.AnyRequest,
	) (connect.AnyResponse, error) {
		rawConfig := a.svc.Config()
		runsSecurely := true
		config, ok := rawConfig.(frame.ConfigurationSecurity)
		if ok {
			runsSecurely = config.IsRunSecurely()
		}

		if !runsSecurely {
			return next(ctx, req)
		}

		authorizationHeader := req.Header().Get("Authorization")

		logger := a.svc.Log(ctx).WithField("authorization_header", authorizationHeader)

		if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "Bearer ") {
			logger.WithField("available_headers", req.Header()).
				Debug(" AuthenticationMiddleware -- could not authenticate missing token")
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("an authorization header is required"),
			)
		}

		extractedJwtToken := strings.Split(authorizationHeader, " ")

		if len(extractedJwtToken) != bearerTokenParts {
			logger.Debug(" AuthenticationMiddleware -- token format is not valid")
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("malformed authorization header supplied"),
			)
		}

		jwtToken := strings.TrimSpace(extractedJwtToken[1])

		ctx, err := a.svc.Authenticate(ctx, jwtToken, a.audience, a.issuer)

		if err != nil {
			logger.WithError(err).Info(" AuthenticationMiddleware -- could not authenticate token")
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("authorization header is invalid"),
			)
		}

		return next(ctx, req)
	})
}

// WrapStreamingClient implements the streaming client interceptor (pass-through for server-side).
func (a *AuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements the streaming handler interceptor for authentication.
func (a *AuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(
		ctx context.Context,
		conn connect.StreamingHandlerConn,
	) error {
		rawConfig := a.svc.Config()
		runsSecurely := true
		config, ok := rawConfig.(frame.ConfigurationSecurity)
		if ok {
			runsSecurely = config.IsRunSecurely()
		}

		if !runsSecurely {
			return next(ctx, conn)
		}

		authorizationHeader := conn.RequestHeader().Get("Authorization")

		logger := a.svc.Log(ctx).WithField("authorization_header", authorizationHeader)

		if authorizationHeader == "" || !strings.HasPrefix(authorizationHeader, "Bearer ") {
			logger.WithField("available_headers", conn.RequestHeader()).
				Debug(" StreamAuthenticationMiddleware -- could not authenticate missing token")
			return connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("an authorization header is required"),
			)
		}

		extractedJwtToken := strings.Split(authorizationHeader, " ")

		if len(extractedJwtToken) != bearerTokenParts {
			logger.Debug(" StreamAuthenticationMiddleware -- token format is not valid")
			return connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("malformed authorization header supplied"),
			)
		}

		jwtToken := strings.TrimSpace(extractedJwtToken[1])

		ctx, err := a.svc.Authenticate(ctx, jwtToken, a.audience, a.issuer)

		if err != nil {
			logger.WithError(err).Info(" StreamAuthenticationMiddleware -- could not authenticate token")
			return connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("authorization header is invalid"),
			)
		}

		return next(ctx, conn)
	})
}

// ValidationInterceptor implements connect.Interceptor for protovalidate validation.
type ValidationInterceptor struct {
	validator protovalidate.Validator
}

// NewValidationInterceptor creates a new validation interceptor.
func NewValidationInterceptor() (*ValidationInterceptor, error) {
	validator, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return &ValidationInterceptor{
		validator: validator,
	}, nil
}

// WrapUnary validates unary requests and responses.
func (v *ValidationInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return connect.UnaryFunc(func(
		ctx context.Context,
		req connect.AnyRequest,
	) (connect.AnyResponse, error) {
		// Validate request if it's a proto message
		if msg, ok := req.Any().(proto.Message); ok {
			if err := v.validator.Validate(msg); err != nil {
				return nil, connect.NewError(
					connect.CodeInvalidArgument,
					fmt.Errorf("request validation failed: %w", err),
				)
			}
		}

		// Call the handler
		resp, err := next(ctx, req)
		if err != nil {
			return nil, err
		}

		// Validate response if it's a proto message
		if msg, ok := resp.Any().(proto.Message); ok {
			if err := v.validator.Validate(msg); err != nil {
				return nil, connect.NewError(
					connect.CodeInternal,
					fmt.Errorf("response validation failed: %w", err),
				)
			}
		}

		return resp, nil
	})
}

// WrapStreamingClient validates streaming client messages.
func (v *ValidationInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next // Client-side validation not needed on server
}

// WrapStreamingHandler validates streaming messages.
func (v *ValidationInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(
		ctx context.Context,
		conn connect.StreamingHandlerConn,
	) error {
		// Wrap the connection to intercept Receive calls
		wrappedConn := &validatingStreamConn{
			StreamingHandlerConn: conn,
			validator:            v.validator,
		}

		return next(ctx, wrappedConn)
	})
}

// validatingStreamConn wraps a StreamingHandlerConn to validate messages.
type validatingStreamConn struct {
	connect.StreamingHandlerConn
	validator protovalidate.Validator
}

// Receive validates incoming stream messages.
func (v *validatingStreamConn) Receive(msg any) error {
	if err := v.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}

	// Validate the received message
	if protoMsg, ok := msg.(proto.Message); ok {
		if err := v.validator.Validate(protoMsg); err != nil {
			return connect.NewError(
				connect.CodeInvalidArgument,
				fmt.Errorf("stream message validation failed: %w", err),
			)
		}
	}

	return nil
}

// Send validates outgoing stream messages.
func (v *validatingStreamConn) Send(msg any) error {
	// Validate before sending
	if protoMsg, ok := msg.(proto.Message); ok {
		if err := v.validator.Validate(protoMsg); err != nil {
			return connect.NewError(
				connect.CodeInternal,
				fmt.Errorf("stream response validation failed: %w", err),
			)
		}
	}

	return v.StreamingHandlerConn.Send(msg)
}
