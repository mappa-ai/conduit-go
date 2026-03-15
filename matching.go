package conduit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const MatchingContextHiringTeamFit = "hiring_team_fit"

type MatchingSubject struct {
	kind     matchingSubjectKind
	entityID string
	mediaID  string
	selector *TargetSelector
}

type matchingSubjectKind string

const (
	matchingSubjectKindEntity matchingSubjectKind = "entity_id"
	matchingSubjectKindMedia  matchingSubjectKind = "media_target"
)

type CreateMatchingRequest struct {
	Context        string
	Group          []MatchingSubject
	IdempotencyKey string
	RequestID      string
	Target         MatchingSubject
	Webhook        *Webhook
}

type MatchingJobReceipt struct {
	EstimatedWaitSec *float64
	Handle           *MatchingRunHandle
	JobID            string
	Stage            string
	Status           string
}

type MatchingRunHandle struct {
	jobID    string
	jobs     *JobsResource
	matching *MatchingResource
}

type MatchingResource struct {
	jobs      *JobsResource
	transport *transport
}

func MatchingEntity(entityID string) MatchingSubject {
	return MatchingSubject{kind: matchingSubjectKindEntity, entityID: entityID}
}

func MatchingMedia(mediaID string, selector TargetSelector) MatchingSubject {
	return MatchingSubject{kind: matchingSubjectKindMedia, mediaID: mediaID, selector: &selector}
}

func (r *MatchingResource) Create(ctx context.Context, request CreateMatchingRequest) (*MatchingJobReceipt, error) {
	contextValue, err := normalizeMatchingContext(request.Context)
	if err != nil {
		return nil, err
	}
	target, err := normalizeMatchingSubject(request.Target)
	if err != nil {
		return nil, err
	}
	group, err := normalizeMatchingSubjects(request.Group)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"context": contextValue,
		"group":   group,
		"target":  target,
	}
	if err := ensureUniqueEntityIDs(target, group); err != nil {
		return nil, err
	}
	webhook, err := normalizeWebhook(request.Webhook)
	if err != nil {
		return nil, err
	}
	if webhook != nil {
		body["webhook"] = webhook
	}
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return nil, &ConduitError{Code: "invalid_request", Message: "failed to encode matching request", Cause: err}
	}
	response, err := r.transport.request(ctx, http.MethodPost, "/v1/matching/jobs", requestOptions{
		body:           encodedBody,
		contentType:    "application/json",
		idempotencyKey: defaultString(request.IdempotencyKey, randomID("idem")),
		requestID:      request.RequestID,
		retryable:      true,
	})
	if err != nil {
		return nil, err
	}
	receipt, err := parseJobReceipt(response.body)
	if err != nil {
		return nil, err
	}
	return &MatchingJobReceipt{EstimatedWaitSec: receipt.EstimatedWaitSec, Handle: &MatchingRunHandle{jobID: receipt.JobID, jobs: r.jobs, matching: r}, JobID: receipt.JobID, Stage: receipt.Stage, Status: receipt.Status}, nil
}

func (r *MatchingResource) Get(ctx context.Context, matchingID string) (*MatchingAnalysisResponse, error) {
	validated, err := requireNonEmpty(matchingID, "matchingID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/matching/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	return parseMatching(response.body)
}

func (r *MatchingResource) getByJob(ctx context.Context, jobID string) (*MatchingAnalysisResponse, error) {
	validated, err := requireNonEmpty(jobID, "jobID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/matching/by-job/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	if len(response.body) == 0 || strings.TrimSpace(string(response.body)) == "null" {
		return nil, nil
	}
	return parseMatching(response.body)
}

func (h *MatchingRunHandle) Stream(ctx context.Context, options *StreamOptions) error {
	_, err := h.jobs.stream(ctx, h.jobID, options)
	return err
}

func (h *MatchingRunHandle) Wait(ctx context.Context, options *WaitOptions) (*MatchingAnalysisResponse, error) {
	job, err := h.jobs.stream(ctx, h.jobID, options)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case "succeeded":
		if job.MatchingID == "" {
			return nil, &ConduitError{Code: "invalid_response", Message: fmt.Sprintf("job %s succeeded but no matchingId was returned", h.jobID)}
		}
		return h.matching.Get(ctx, job.MatchingID)
	case "failed":
		if job.Error != nil {
			return nil, wrapJobFailed(h.jobID, job.RequestID, job.Error.Code, job.Error.Message)
		}
		return nil, wrapJobFailed(h.jobID, job.RequestID, "job_failed", "job failed")
	default:
		return nil, wrapJobCanceled(h.jobID, job.RequestID)
	}
}

func (h *MatchingRunHandle) Cancel(ctx context.Context) (*Job, error) {
	return h.jobs.Cancel(ctx, CancelJobRequest{JobID: h.jobID})
}

func (h *MatchingRunHandle) Job(ctx context.Context) (*Job, error) {
	return h.jobs.Get(ctx, h.jobID)
}

func (h *MatchingRunHandle) Matching(ctx context.Context) (*MatchingAnalysisResponse, error) {
	return h.matching.getByJob(ctx, h.jobID)
}

func normalizeMatchingContext(context string) (string, error) {
	if strings.TrimSpace(context) != MatchingContextHiringTeamFit {
		return "", &ConduitError{Code: "invalid_request", Message: "context must be hiring_team_fit"}
	}
	return context, nil
}

func normalizeMatchingSubject(subject MatchingSubject) (map[string]any, error) {
	switch subject.kind {
	case matchingSubjectKindEntity:
		validated, err := requireNonEmpty(subject.entityID, "subject.entityId")
		if err != nil {
			return nil, err
		}
		return map[string]any{"entityId": validated, "type": string(matchingSubjectKindEntity)}, nil
	case matchingSubjectKindMedia:
		if subject.selector == nil {
			return nil, &ConduitError{Code: "invalid_request", Message: "subject.selector is required for mediaId refs"}
		}
		validated, err := requireNonEmpty(subject.mediaID, "subject.mediaId")
		if err != nil {
			return nil, err
		}
		selector, err := normalizeTargetSelector(*subject.selector)
		if err != nil {
			return nil, err
		}
		return map[string]any{"mediaId": validated, "selector": selector, "type": string(matchingSubjectKindMedia)}, nil
	default:
		return nil, &ConduitError{Code: "invalid_request", Message: "subject must include entityId or mediaId with selector"}
	}
}

func normalizeMatchingSubjects(subjects []MatchingSubject) ([]map[string]any, error) {
	if len(subjects) == 0 {
		return nil, &ConduitError{Code: "invalid_request", Message: "group must contain at least one subject"}
	}
	resolved := make([]map[string]any, 0, len(subjects))
	for _, subject := range subjects {
		normalized, err := normalizeMatchingSubject(subject)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, normalized)
	}
	return resolved, nil
}

func ensureUniqueEntityIDs(target map[string]any, group []map[string]any) error {
	seen := map[string]struct{}{}
	for _, subject := range append([]map[string]any{target}, group...) {
		if subject["type"] != string(matchingSubjectKindEntity) {
			continue
		}
		entityID, _ := subject["entityId"].(string)
		if _, exists := seen[entityID]; exists {
			return &ConduitError{Code: "invalid_request", Message: "target and group must reference different direct entity IDs"}
		}
		seen[entityID] = struct{}{}
	}
	return nil
}
