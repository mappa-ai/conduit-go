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
	ReportTemplateGeneralReport = "general_report"
	ReportTemplateSalesPlaybook = "sales_playbook"
	OnMissError                 = "error"
	OnMissFallbackDominant      = "fallback_dominant"
	TargetStrategyDominant      = "dominant"
	TargetStrategyEntityID      = "entity_id"
	TargetStrategyMagicHint     = "magic_hint"
	TargetStrategyTimeRange     = "timerange"
)

type Webhook struct {
	Headers map[string]string
	URL     string
}

type ReportOutputRequest struct {
	Template       string
	TemplateParams map[string]any
}

type TimeRange struct {
	EndSeconds   *float64
	StartSeconds *float64
}

type TargetSelector struct {
	EntityID  string
	Hint      string
	OnMiss    string
	Strategy  string
	TimeRange *TimeRange
}

type CreateReportRequest struct {
	IdempotencyKey string
	Output         ReportOutputRequest
	RequestID      string
	Source         Source
	Target         TargetSelector
	Webhook        *Webhook
}

type ReportJobReceipt struct {
	EstimatedWaitSec *float64
	Handle           *ReportRunHandle
	JobID            string
	MediaID          string
	Stage            string
	Status           string
}

type ReportRunHandle struct {
	jobID   string
	jobs    *JobsResource
	reports *ReportsResource
}

type ReportsResource struct {
	jobs      *JobsResource
	media     *MediaResource
	transport *transport
}

func TargetDominant() TargetSelector {
	return TargetSelector{Strategy: TargetStrategyDominant}
}

func TargetEntityID(entityID string) TargetSelector {
	return TargetSelector{EntityID: entityID, Strategy: TargetStrategyEntityID}
}

func TargetMagicHint(hint string) TargetSelector {
	return TargetSelector{Hint: hint, Strategy: TargetStrategyMagicHint}
}

func TargetTimeRange(startSeconds *float64, endSeconds *float64) TargetSelector {
	return TargetSelector{Strategy: TargetStrategyTimeRange, TimeRange: &TimeRange{EndSeconds: endSeconds, StartSeconds: startSeconds}}
}

func (t TargetSelector) WithOnMiss(onMiss string) TargetSelector {
	t.OnMiss = onMiss
	return t
}

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

func (h *ReportRunHandle) Stream(ctx context.Context, options *StreamOptions) error {
	_, err := h.jobs.stream(ctx, h.jobID, options)
	return err
}

func (h *ReportRunHandle) Wait(ctx context.Context, options *WaitOptions) (*Report, error) {
	job, err := h.jobs.stream(ctx, h.jobID, options)
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

func (h *ReportRunHandle) Cancel(ctx context.Context) (*Job, error) {
	return h.jobs.Cancel(ctx, CancelJobRequest{JobID: h.jobID})
}

func (h *ReportRunHandle) Job(ctx context.Context) (*Job, error) {
	return h.jobs.Get(ctx, h.jobID)
}

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
