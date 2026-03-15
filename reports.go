package conduit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	// ReportTemplateGeneralReport requests the default general-purpose report template.
	ReportTemplateGeneralReport = "general_report"
	// ReportTemplateSalesPlaybook requests the sales-focused report template.
	ReportTemplateSalesPlaybook = "sales_playbook"
	// OnMissError preserves strict target selection and fails when no match is found.
	OnMissError = "error"
	// OnMissFallbackDominant falls back to the dominant speaker when targeting misses.
	OnMissFallbackDominant = "fallback_dominant"
	// TargetStrategyDominant selects the dominant speaker.
	TargetStrategyDominant = "dominant"
	// TargetStrategyEntityID selects a previously identified entity.
	TargetStrategyEntityID = "entity_id"
	// TargetStrategyMagicHint selects a best-effort natural-language hint.
	TargetStrategyMagicHint = "magic_hint"
	// TargetStrategyTimeRange selects a speaker by time bounds.
	TargetStrategyTimeRange = "timerange"
)

// Webhook configures delivery for terminal job notifications.
type Webhook struct {
	// Headers are optional static headers attached to webhook delivery.
	Headers map[string]string
	// URL is the destination that receives webhook events.
	URL string
}

// ReportOutputRequest configures report output rendering.
type ReportOutputRequest struct {
	// Template is one of the stable report template identifiers.
	Template string
	// TemplateParams carries template-specific rendering inputs.
	TemplateParams map[string]any
}

// TimeRange selects a target by time bounds.
type TimeRange struct {
	// EndSeconds is the exclusive upper bound in seconds.
	EndSeconds *float64
	// StartSeconds is the inclusive lower bound in seconds.
	StartSeconds *float64
}

// TargetSelector selects the speaker to analyze.
type TargetSelector struct {
	// EntityID is required when Strategy is TargetStrategyEntityID.
	EntityID string
	// Hint is required when Strategy is TargetStrategyMagicHint.
	Hint string
	// OnMiss controls fallback behavior when the requested target cannot be resolved.
	OnMiss string
	// Strategy selects the target resolution mode.
	Strategy string
	// TimeRange is required when Strategy is TargetStrategyTimeRange.
	TimeRange *TimeRange
}

// CreateReportRequest creates a report generation job.
type CreateReportRequest struct {
	// IdempotencyKey scopes retries and duplicate submission protection for create.
	IdempotencyKey string
	// Output selects the report template and optional template params.
	Output ReportOutputRequest
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
	// Source identifies the media to analyze.
	Source Source
	// Target selects the speaker to analyze.
	Target TargetSelector
	// Webhook configures async completion delivery.
	Webhook *Webhook
}

// ReportJobReceipt acknowledges accepted report work.
type ReportJobReceipt struct {
	// EstimatedWaitSec is the API-provided advisory wait estimate when available.
	EstimatedWaitSec *float64
	// Handle provides secondary wait, stream, cancel, and fetch helpers.
	Handle *ReportRunHandle
	// JobID is the accepted report job identifier.
	JobID string
	// MediaID is the resolved uploaded media identifier when known.
	MediaID string
	// Stage is the current advisory stage when provided by the API.
	Stage string
	// Status is the accepted job status, typically queued or running.
	Status string
}

// ReportRunHandle provides secondary synchronous helpers for a report job.
type ReportRunHandle struct {
	jobID   string
	jobs    *JobsResource
	reports *ReportsResource
}

// ReportsResource exposes report workflows.
type ReportsResource struct {
	jobs      *JobsResource
	media     *MediaResource
	transport *transport
}

// TargetDominant selects the dominant speaker.
func TargetDominant() TargetSelector {
	return TargetSelector{Strategy: TargetStrategyDominant}
}

// TargetEntityID selects a known entity.
func TargetEntityID(entityID string) TargetSelector {
	return TargetSelector{EntityID: entityID, Strategy: TargetStrategyEntityID}
}

// TargetMagicHint selects a best-effort natural language hint.
func TargetMagicHint(hint string) TargetSelector {
	return TargetSelector{Hint: hint, Strategy: TargetStrategyMagicHint}
}

// TargetTimeRange selects a speaker by time bounds.
func TargetTimeRange(startSeconds *float64, endSeconds *float64) TargetSelector {
	return TargetSelector{Strategy: TargetStrategyTimeRange, TimeRange: &TimeRange{EndSeconds: endSeconds, StartSeconds: startSeconds}}
}

// WithOnMiss configures target fallback behavior.
func (t TargetSelector) WithOnMiss(onMiss string) TargetSelector {
	t.OnMiss = onMiss
	return t
}

// Create resolves the source, uploads when needed, creates the job, and returns
// a receipt after the work has been accepted by the API.
func (r *ReportsResource) Create(ctx context.Context, request CreateReportRequest) (*ReportJobReceipt, error) {
	output, err := normalizeReportOutput(request.Output)
	if err != nil {
		return nil, err
	}
	target, err := normalizeTargetSelector(request.Target)
	if err != nil {
		return nil, err
	}
	mediaID, err := r.media.resolveReportSource(ctx, request.Source, request.IdempotencyKey, request.RequestID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"media":  map[string]string{"mediaId": mediaID},
		"output": output,
		"target": target,
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
		return nil, &ConduitError{Code: "invalid_request", Message: "failed to encode report request", Cause: err}
	}
	response, err := r.transport.request(ctx, http.MethodPost, "/v1/reports/jobs", requestOptions{
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
	return &ReportJobReceipt{
		EstimatedWaitSec: receipt.EstimatedWaitSec,
		Handle:           &ReportRunHandle{jobID: receipt.JobID, jobs: r.jobs, reports: r},
		JobID:            receipt.JobID,
		MediaID:          mediaID,
		Stage:            receipt.Stage,
		Status:           receipt.Status,
	}, nil
}

// Get fetches a completed report by ID.
func (r *ReportsResource) Get(ctx context.Context, reportID string) (*Report, error) {
	validated, err := requireNonEmpty(reportID, "reportID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/reports/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	return parseReport(response.body)
}

func (r *ReportsResource) getByJob(ctx context.Context, jobID string) (*Report, error) {
	validated, err := requireNonEmpty(jobID, "jobID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/reports/by-job/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	if len(response.body) == 0 || strings.TrimSpace(string(response.body)) == "null" {
		return nil, nil
	}
	return parseReport(response.body)
}

// Stream opens the SSE job stream for the report run.
func (h *ReportRunHandle) Stream(ctx context.Context, options *StreamOptions) (*JobStream, error) {
	return h.jobs.stream(ctx, h.jobID, options)
}

// Wait blocks until the report job reaches a terminal state and then fetches the
// completed report.
func (h *ReportRunHandle) Wait(ctx context.Context, options *WaitOptions) (*Report, error) {
	job, err := h.jobs.wait(ctx, h.jobID, options)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case "succeeded":
		if job.ReportID == "" {
			return nil, &ConduitError{Code: "invalid_response", Message: fmt.Sprintf("job %s succeeded but no reportId was returned", h.jobID)}
		}
		return h.reports.Get(ctx, job.ReportID)
	case "failed":
		if job.Error != nil {
			return nil, wrapJobFailed(h.jobID, job.RequestID, job.Error.Code, job.Error.Message)
		}
		return nil, wrapJobFailed(h.jobID, job.RequestID, "job_failed", "job failed")
	default:
		return nil, wrapJobCanceled(h.jobID, job.RequestID)
	}
}

// Cancel requests cancellation for the report job.
func (h *ReportRunHandle) Cancel(ctx context.Context) (*Job, error) {
	return h.jobs.Cancel(ctx, CancelJobRequest{JobID: h.jobID})
}

// Job fetches the latest job state.
func (h *ReportRunHandle) Job(ctx context.Context) (*Job, error) {
	return h.jobs.Get(ctx, h.jobID)
}

// Report fetches the completed report by job ID and returns nil when the API has
// not materialized the report yet.
func (h *ReportRunHandle) Report(ctx context.Context) (*Report, error) {
	return h.reports.getByJob(ctx, h.jobID)
}

func normalizeReportOutput(output ReportOutputRequest) (map[string]any, error) {
	if output.Template != ReportTemplateGeneralReport && output.Template != ReportTemplateSalesPlaybook {
		return nil, &ConduitError{Code: "invalid_request", Message: "output.template must be general_report or sales_playbook"}
	}
	payload := map[string]any{"template": output.Template}
	if output.TemplateParams != nil {
		payload["templateParams"] = output.TemplateParams
	}
	return payload, nil
}

func normalizeTargetSelector(target TargetSelector) (map[string]any, error) {
	payload := map[string]any{"strategy": target.Strategy}
	if target.OnMiss != "" {
		if target.OnMiss != OnMissError && target.OnMiss != OnMissFallbackDominant {
			return nil, &ConduitError{Code: "invalid_request", Message: "target.onMiss must be fallback_dominant or error"}
		}
		payload["on_miss"] = target.OnMiss
	}
	switch target.Strategy {
	case TargetStrategyDominant:
		return payload, nil
	case TargetStrategyTimeRange:
		if target.TimeRange == nil {
			return nil, &ConduitError{Code: "invalid_request", Message: "target.timeRange is required for timerange"}
		}
		if target.TimeRange.StartSeconds == nil && target.TimeRange.EndSeconds == nil {
			return nil, &ConduitError{Code: "invalid_request", Message: "target.timeRange must include startSeconds or endSeconds"}
		}
		if target.TimeRange.StartSeconds != nil && target.TimeRange.EndSeconds != nil && *target.TimeRange.StartSeconds >= *target.TimeRange.EndSeconds {
			return nil, &ConduitError{Code: "invalid_request", Message: "target.timeRange.startSeconds must be less than endSeconds"}
		}
		payload["timerange"] = map[string]any{"end_seconds": target.TimeRange.EndSeconds, "start_seconds": target.TimeRange.StartSeconds}
		return payload, nil
	case TargetStrategyEntityID:
		validated, err := requireNonEmpty(target.EntityID, "target.entityId")
		if err != nil {
			return nil, err
		}
		payload["entity_id"] = validated
		return payload, nil
	case TargetStrategyMagicHint:
		validated, err := requireNonEmpty(target.Hint, "target.hint")
		if err != nil {
			return nil, err
		}
		payload["hint"] = validated
		return payload, nil
	default:
		return nil, &ConduitError{Code: "invalid_request", Message: "target.strategy must be dominant, timerange, entity_id, or magic_hint"}
	}
}

func normalizeWebhook(webhook *Webhook) (map[string]any, error) {
	if webhook == nil {
		return nil, nil
	}
	parsed, err := parseHTTPURL(webhook.URL, "webhook.url")
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"url": parsed.String()}
	if webhook.Headers != nil {
		headers := make(map[string]string, len(webhook.Headers))
		for key, value := range webhook.Headers {
			validated, err := requireNonEmpty(value, fmt.Sprintf("webhook.headers[%s]", key))
			if err != nil {
				return nil, err
			}
			headers[key] = validated
		}
		payload["headers"] = headers
	}
	return payload, nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
