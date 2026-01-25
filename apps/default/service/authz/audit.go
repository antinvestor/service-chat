package authz

import (
	"context"
	"math/rand/v2"

	"github.com/pitabwire/util"
)

// auditLogger implements the AuditLogger interface.
type auditLogger struct {
	sampleRate float64
	enabled    bool
}

// AuditLoggerConfig holds configuration for the audit logger.
type AuditLoggerConfig struct {
	// Enabled determines if audit logging is enabled.
	Enabled bool
	// SampleRate is the fraction of decisions to log (0.0 to 1.0).
	SampleRate float64
}

// NewAuditLogger creates a new AuditLogger with the given configuration.
func NewAuditLogger(config AuditLoggerConfig) AuditLogger {
	if config.SampleRate <= 0 {
		config.SampleRate = 1.0
	}
	if config.SampleRate > 1.0 {
		config.SampleRate = 1.0
	}

	return &auditLogger{
		sampleRate: config.SampleRate,
		enabled:    config.Enabled,
	}
}

// LogDecision logs an authorization decision for security audit.
func (a *auditLogger) LogDecision(ctx context.Context, req CheckRequest, result CheckResult, metadata map[string]string) error {
	if !a.enabled {
		return nil
	}

	// Sample rate check
	if a.sampleRate < 1.0 && rand.Float64() > a.sampleRate {
		return nil
	}

	fields := map[string]any{
		"authz_object_namespace": req.Object.Namespace,
		"authz_object_id":        req.Object.ID,
		"authz_permission":       req.Permission,
		"authz_subject_ns":       req.Subject.Namespace,
		"authz_subject_id":       req.Subject.ID,
		"authz_allowed":          result.Allowed,
		"authz_checked_at":       result.CheckedAt,
	}

	if result.Reason != "" {
		fields["authz_reason"] = result.Reason
	}

	if req.Subject.Relation != "" {
		fields["authz_subject_relation"] = req.Subject.Relation
	}

	// Add custom metadata
	for k, v := range metadata {
		fields["authz_meta_"+k] = v
	}

	log := util.Log(ctx).WithFields(fields)

	if result.Allowed {
		log.Debug("authorization decision: allowed")
	} else {
		log.Info("authorization decision: denied")
	}

	return nil
}

// NoOpAuditLogger is an audit logger that does nothing.
type NoOpAuditLogger struct{}

// LogDecision implements AuditLogger but does nothing.
func (n *NoOpAuditLogger) LogDecision(_ context.Context, _ CheckRequest, _ CheckResult, _ map[string]string) error {
	return nil
}

// NewNoOpAuditLogger creates a new no-op audit logger.
func NewNoOpAuditLogger() AuditLogger {
	return &NoOpAuditLogger{}
}
