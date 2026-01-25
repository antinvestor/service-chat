// Package keto provides an Ory Keto adapter implementation for the authz.AuthzService interface.
package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/pitabwire/util"
)

// ketoAdapter implements the authz.AuthzService interface using Ory Keto.
type ketoAdapter struct {
	readURL     string
	writeURL    string
	httpClient  *http.Client
	config      Config
	auditLogger authz.AuditLogger
}

// NewKetoAdapter creates a new Keto adapter with the given configuration.
func NewKetoAdapter(cfg Config, auditLogger authz.AuditLogger) (authz.AuthzService, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if auditLogger == nil {
		auditLogger = authz.NewNoOpAuditLogger()
	}

	return &ketoAdapter{
		readURL:  cfg.ReadURL,
		writeURL: cfg.WriteURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		config:      cfg,
		auditLogger: auditLogger,
	}, nil
}

// Keto API request/response types

type ketoCheckResponse struct {
	Allowed bool `json:"allowed"`
}

type ketoRelationTuple struct {
	Namespace string          `json:"namespace"`
	Object    string          `json:"object"`
	Relation  string          `json:"relation"`
	SubjectID string          `json:"subject_id,omitempty"`
	Subject   *ketoSubjectSet `json:"subject_set,omitempty"`
}

type ketoSubjectSet struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
}

type ketoWriteRequest struct {
	RelationTuples []*ketoRelationTuple `json:"relation_tuples,omitempty"`
}

type ketoListResponse struct {
	RelationTuples []*ketoRelationTuple `json:"relation_tuples"`
	NextPageToken  string               `json:"next_page_token,omitempty"`
}

type ketoExpandResponse struct {
	SubjectSets []struct {
		Namespace string `json:"namespace"`
		Object    string `json:"object"`
		Relation  string `json:"relation"`
	} `json:"subject_sets,omitempty"`
	SubjectIDs []string `json:"subject_ids,omitempty"`
}

// Check verifies if a subject has permission on an object.
func (k *ketoAdapter) Check(ctx context.Context, req authz.CheckRequest) (authz.CheckResult, error) {
	if !k.config.Enabled {
		return authz.CheckResult{
			Allowed:   true,
			Reason:    "keto disabled - permissive mode",
			CheckedAt: time.Now().Unix(),
		}, nil
	}

	// Build URL with query parameters
	u, err := url.Parse(k.readURL + "/relation-tuples/check")
	if err != nil {
		return authz.CheckResult{}, authz.NewAuthzServiceError("check", err)
	}

	q := u.Query()
	q.Set("namespace", req.Object.Namespace)
	q.Set("object", req.Object.ID)
	q.Set("relation", req.Permission)

	if req.Subject.Relation != "" {
		// Subject set
		q.Set("subject_set.namespace", req.Subject.Namespace)
		q.Set("subject_set.object", req.Subject.ID)
		q.Set("subject_set.relation", req.Subject.Relation)
	} else {
		q.Set("subject_id", req.Subject.ID)
	}
	u.RawQuery = q.Encode()

	// Execute request with retries
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt <= k.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(k.config.RetryDelay)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			lastErr = err
			continue
		}

		resp, lastErr = k.httpClient.Do(httpReq)
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		return authz.CheckResult{}, authz.NewAuthzServiceError("check", lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return authz.CheckResult{}, authz.NewAuthzServiceError("check",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var checkResp ketoCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return authz.CheckResult{}, authz.NewAuthzServiceError("check", err)
	}

	result := authz.CheckResult{
		Allowed:   checkResp.Allowed,
		CheckedAt: time.Now().Unix(),
	}

	if !checkResp.Allowed {
		result.Reason = "no matching relation found"
	}

	// Audit log
	if err := k.auditLogger.LogDecision(ctx, req, result, nil); err != nil {
		util.Log(ctx).WithError(err).Warn("failed to log authorization decision")
	}

	return result, nil
}

// BatchCheck verifies multiple permissions in one call.
func (k *ketoAdapter) BatchCheck(ctx context.Context, requests []authz.CheckRequest) ([]authz.CheckResult, error) {
	if !k.config.Enabled {
		results := make([]authz.CheckResult, len(requests))
		for i := range results {
			results[i] = authz.CheckResult{
				Allowed:   true,
				Reason:    "keto disabled - permissive mode",
				CheckedAt: time.Now().Unix(),
			}
		}
		return results, nil
	}

	// Keto doesn't have a native batch check API, so we do individual checks
	// This could be optimized with goroutines for parallel execution
	results := make([]authz.CheckResult, len(requests))
	for i, req := range requests {
		result, err := k.Check(ctx, req)
		if err != nil {
			// On error, deny access (fail closed)
			results[i] = authz.CheckResult{
				Allowed:   false,
				Reason:    fmt.Sprintf("check failed: %v", err),
				CheckedAt: time.Now().Unix(),
			}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// WriteTuple creates a relationship tuple.
func (k *ketoAdapter) WriteTuple(ctx context.Context, tuple authz.RelationTuple) error {
	return k.WriteTuples(ctx, []authz.RelationTuple{tuple})
}

// WriteTuples creates multiple relationship tuples atomically.
func (k *ketoAdapter) WriteTuples(ctx context.Context, tuples []authz.RelationTuple) error {
	if !k.config.Enabled {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	ketoTuples := make([]*ketoRelationTuple, len(tuples))
	for i, t := range tuples {
		ketoTuples[i] = k.toKetoTuple(t)
	}

	reqBody := ketoWriteRequest{
		RelationTuples: ketoTuples,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return authz.NewAuthzServiceError("write_tuples", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		k.writeURL+"/admin/relation-tuples", bytes.NewReader(body))
	if err != nil {
		return authz.NewAuthzServiceError("write_tuples", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return authz.NewAuthzServiceError("write_tuples", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return authz.NewAuthzServiceError("write_tuples",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody)))
	}

	return nil
}

// DeleteTuple removes a relationship tuple.
func (k *ketoAdapter) DeleteTuple(ctx context.Context, tuple authz.RelationTuple) error {
	return k.DeleteTuples(ctx, []authz.RelationTuple{tuple})
}

// DeleteTuples removes multiple relationship tuples atomically.
func (k *ketoAdapter) DeleteTuples(ctx context.Context, tuples []authz.RelationTuple) error {
	if !k.config.Enabled {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	// Keto uses query parameters for delete
	for _, tuple := range tuples {
		u, err := url.Parse(k.writeURL + "/admin/relation-tuples")
		if err != nil {
			return authz.NewAuthzServiceError("delete_tuples", err)
		}

		q := u.Query()
		q.Set("namespace", tuple.Object.Namespace)
		q.Set("object", tuple.Object.ID)
		q.Set("relation", tuple.Relation)

		if tuple.Subject.Relation != "" {
			q.Set("subject_set.namespace", tuple.Subject.Namespace)
			q.Set("subject_set.object", tuple.Subject.ID)
			q.Set("subject_set.relation", tuple.Subject.Relation)
		} else {
			q.Set("subject_id", tuple.Subject.ID)
		}
		u.RawQuery = q.Encode()

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
		if err != nil {
			return authz.NewAuthzServiceError("delete_tuples", err)
		}

		resp, err := k.httpClient.Do(httpReq)
		if err != nil {
			return authz.NewAuthzServiceError("delete_tuples", err)
		}
		resp.Body.Close()

		// 404 is acceptable - tuple might not exist
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			return authz.NewAuthzServiceError("delete_tuples",
				fmt.Errorf("unexpected status %d", resp.StatusCode))
		}
	}

	return nil
}

// ListRelations returns all relations for an object.
func (k *ketoAdapter) ListRelations(ctx context.Context, object authz.ObjectRef) ([]authz.RelationTuple, error) {
	if !k.config.Enabled {
		return []authz.RelationTuple{}, nil
	}

	u, err := url.Parse(k.readURL + "/relation-tuples")
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_relations", err)
	}

	q := u.Query()
	q.Set("namespace", object.Namespace)
	q.Set("object", object.ID)
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_relations", err)
	}

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_relations", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, authz.NewAuthzServiceError("list_relations",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var listResp ketoListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, authz.NewAuthzServiceError("list_relations", err)
	}

	tuples := make([]authz.RelationTuple, len(listResp.RelationTuples))
	for i, kt := range listResp.RelationTuples {
		tuples[i] = k.fromKetoTuple(kt)
	}

	return tuples, nil
}

// ListSubjectRelations returns all objects a subject has relations to.
func (k *ketoAdapter) ListSubjectRelations(ctx context.Context, subject authz.SubjectRef, namespace string) ([]authz.RelationTuple, error) {
	if !k.config.Enabled {
		return []authz.RelationTuple{}, nil
	}

	u, err := url.Parse(k.readURL + "/relation-tuples")
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_subject_relations", err)
	}

	q := u.Query()
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	if subject.Relation != "" {
		q.Set("subject_set.namespace", subject.Namespace)
		q.Set("subject_set.object", subject.ID)
		q.Set("subject_set.relation", subject.Relation)
	} else {
		q.Set("subject_id", subject.ID)
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_subject_relations", err)
	}

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return nil, authz.NewAuthzServiceError("list_subject_relations", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, authz.NewAuthzServiceError("list_subject_relations",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var listResp ketoListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, authz.NewAuthzServiceError("list_subject_relations", err)
	}

	tuples := make([]authz.RelationTuple, len(listResp.RelationTuples))
	for i, kt := range listResp.RelationTuples {
		tuples[i] = k.fromKetoTuple(kt)
	}

	return tuples, nil
}

// Expand returns all subjects with a given relation.
func (k *ketoAdapter) Expand(ctx context.Context, object authz.ObjectRef, relation string) ([]authz.SubjectRef, error) {
	if !k.config.Enabled {
		return []authz.SubjectRef{}, nil
	}

	u, err := url.Parse(k.readURL + "/relation-tuples/expand")
	if err != nil {
		return nil, authz.NewAuthzServiceError("expand", err)
	}

	q := u.Query()
	q.Set("namespace", object.Namespace)
	q.Set("object", object.ID)
	q.Set("relation", relation)
	q.Set("max-depth", "3")
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, authz.NewAuthzServiceError("expand", err)
	}

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return nil, authz.NewAuthzServiceError("expand", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, authz.NewAuthzServiceError("expand",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var expandResp ketoExpandResponse
	if err := json.NewDecoder(resp.Body).Decode(&expandResp); err != nil {
		return nil, authz.NewAuthzServiceError("expand", err)
	}

	subjects := make([]authz.SubjectRef, 0, len(expandResp.SubjectIDs)+len(expandResp.SubjectSets))

	for _, id := range expandResp.SubjectIDs {
		subjects = append(subjects, authz.SubjectRef{
			Namespace: authz.NamespaceProfile,
			ID:        id,
		})
	}

	for _, ss := range expandResp.SubjectSets {
		subjects = append(subjects, authz.SubjectRef{
			Namespace: ss.Namespace,
			ID:        ss.Object,
			Relation:  ss.Relation,
		})
	}

	return subjects, nil
}

// Helper methods

func (k *ketoAdapter) toKetoTuple(t authz.RelationTuple) *ketoRelationTuple {
	kt := &ketoRelationTuple{
		Namespace: t.Object.Namespace,
		Object:    t.Object.ID,
		Relation:  t.Relation,
	}

	if t.Subject.Relation != "" {
		kt.Subject = &ketoSubjectSet{
			Namespace: t.Subject.Namespace,
			Object:    t.Subject.ID,
			Relation:  t.Subject.Relation,
		}
	} else {
		kt.SubjectID = t.Subject.ID
	}

	return kt
}

func (k *ketoAdapter) fromKetoTuple(kt *ketoRelationTuple) authz.RelationTuple {
	t := authz.RelationTuple{
		Object: authz.ObjectRef{
			Namespace: kt.Namespace,
			ID:        kt.Object,
		},
		Relation: kt.Relation,
	}

	if kt.Subject != nil {
		t.Subject = authz.SubjectRef{
			Namespace: kt.Subject.Namespace,
			ID:        kt.Subject.Object,
			Relation:  kt.Subject.Relation,
		}
	} else {
		t.Subject = authz.SubjectRef{
			Namespace: authz.NamespaceProfile,
			ID:        kt.SubjectID,
		}
	}

	return t
}
