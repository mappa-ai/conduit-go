package conduit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type JobErrorData struct {
	Code      string
	Details   any
	Message   string
	Retryable *bool
}

type Usage struct {
	CreditsDiscounted *float64
	CreditsNetUsed    float64
	CreditsUsed       float64
	DurationMS        *float64
	ModelVersion      string
}

type JobCreditReservation struct {
	ReservedCredits   *float64
	ReservationStatus string
}

type Job struct {
	Credits         *JobCreditReservation
	CreatedAt       string
	Error           *JobErrorData
	ID              string
	MatchingID      string
	Progress        *float64
	ReleasedCredits *float64
	ReportID        string
	RequestID       string
	Stage           string
	Status          string
	Type            string
	UpdatedAt       string
	Usage           *Usage
}

type JobEvent struct {
	Job      *Job
	Progress *float64
	Stage    string
	Type     string
}

type ReportOutput struct {
	JSON      map[string]any
	Markdown  string
	ReportURL string
	Template  string
}

type Report struct {
	CreatedAt   string
	EntityID    string
	EntityLabel string
	ID          string
	JobID       string
	Label       string
	MediaID     string
	Output      ReportOutput
}

type MatchingResolvedSubject struct {
	EntityID      string
	ResolvedLabel string
	Source        map[string]any
}

type MatchingOutput struct {
	JSON     map[string]any
	Markdown string
}

type MatchingAnalysisResponse struct {
	Context   string
	CreatedAt string
	Group     []MatchingResolvedSubject
	ID        string
	JobID     string
	Label     string
	Output    MatchingOutput
	Target    *MatchingResolvedSubject
}

type MediaObject struct {
	ContentType     string
	CreatedAt       string
	DurationSeconds *float64
	Label           string
	MediaID         string
	SizeBytes       *int64
}

type MediaRetention struct {
	DaysRemaining *int64
	ExpiresAt     string
	Locked        bool
}

type MediaFile struct {
	ContentType      string
	CreatedAt        string
	DurationSeconds  *float64
	Label            string
	LastUsedAt       string
	MediaID          string
	ProcessingStatus string
	Retention        MediaRetention
	SizeBytes        *int64
}

type FileDeleteReceipt struct {
	Deleted bool
	MediaID string
}

type RetentionLockResult struct {
	MediaID       string
	Message       string
	RetentionLock bool
}

type ListFilesResponse struct {
	Files      []MediaFile
	HasMore    bool
	NextCursor string
}

type Entity struct {
	CreatedAt  string
	ID         string
	Label      string
	LastSeenAt string
	MediaCount float64
}

type ListEntitiesResponse struct {
	Cursor   string
	Entities []Entity
	HasMore  bool
}

type WebhookEvent struct {
	CreatedAt string
	Data      any
	ID        string
	Timestamp string
	Type      string
}

type reportJobReceipt struct {
	EstimatedWaitSec *float64 `json:"estimatedWaitSec"`
	JobID            string   `json:"jobId"`
	Stage            string   `json:"stage"`
	Status           string   `json:"status"`
}

type reportWire struct {
	CreatedAt string `json:"createdAt"`
	Entity    struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"entity"`
	ID       string         `json:"id"`
	JobID    string         `json:"jobId"`
	JSON     map[string]any `json:"json"`
	Label    string         `json:"label"`
	Markdown string         `json:"markdown"`
	Media    struct {
		MediaID string `json:"mediaId"`
		URL     string `json:"url"`
	} `json:"media"`
	Output struct {
		Template string `json:"template"`
	} `json:"output"`
}

type matchingSubjectWire struct {
	EntityID      string         `json:"entityId"`
	ResolvedLabel string         `json:"resolvedLabel"`
	Source        map[string]any `json:"source"`
}

type matchingWire struct {
	Context   string                `json:"context"`
	CreatedAt string                `json:"createdAt"`
	Group     []matchingSubjectWire `json:"group"`
	ID        string                `json:"id"`
	JobID     string                `json:"jobId"`
	JSON      map[string]any        `json:"json"`
	Label     string                `json:"label"`
	Markdown  string                `json:"markdown"`
	Target    *matchingSubjectWire  `json:"target"`
}

type mediaObjectWire struct {
	ContentType     string   `json:"contentType"`
	CreatedAt       string   `json:"createdAt"`
	DurationSeconds *float64 `json:"durationSeconds"`
	Label           string   `json:"label"`
	MediaID         string   `json:"mediaId"`
	SizeBytes       *int64   `json:"sizeBytes"`
}

type mediaFileWire struct {
	ContentType      string   `json:"contentType"`
	CreatedAt        string   `json:"createdAt"`
	DurationSeconds  *float64 `json:"durationSeconds"`
	Label            string   `json:"label"`
	LastUsedAt       string   `json:"lastUsedAt"`
	MediaID          string   `json:"mediaId"`
	ProcessingStatus string   `json:"processingStatus"`
	Retention        struct {
		DaysRemaining *int64 `json:"daysRemaining"`
		ExpiresAt     string `json:"expiresAt"`
		Locked        bool   `json:"locked"`
	} `json:"retention"`
	SizeBytes *int64 `json:"sizeBytes"`
}

type entityWire struct {
	CreatedAt  string  `json:"createdAt"`
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	LastSeenAt string  `json:"lastSeenAt"`
	MediaCount float64 `json:"mediaCount"`
}

type jobWire struct {
	CreatedAt string `json:"createdAt"`
	Credits   *struct {
		ReservedCredits   *float64 `json:"reservedCredits"`
		ReservationStatus string   `json:"reservationStatus"`
	} `json:"credits"`
	Error *struct {
		Code      string `json:"code"`
		Details   any    `json:"details"`
		Message   string `json:"message"`
		Retryable *bool  `json:"retryable"`
	} `json:"error"`
	ID              string   `json:"id"`
	MatchingID      string   `json:"matchingId"`
	Progress        *float64 `json:"progress"`
	ReleasedCredits *float64 `json:"releasedCredits"`
	ReportID        string   `json:"reportId"`
	RequestID       string   `json:"requestId"`
	Stage           string   `json:"stage"`
	Status          string   `json:"status"`
	Type            string   `json:"type"`
	UpdatedAt       string   `json:"updatedAt"`
	Usage           *struct {
		CreditsDiscounted *float64 `json:"creditsDiscounted"`
		CreditsNetUsed    float64  `json:"creditsNetUsed"`
		CreditsUsed       float64  `json:"creditsUsed"`
		DurationMS        *float64 `json:"durationMs"`
		ModelVersion      string   `json:"modelVersion"`
	} `json:"usage"`
}

func parseJob(data []byte) (*Job, error) {
	var wire jobWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.ID, "job.id"),
		requireString(wire.Type, "job.type"),
		requireString(wire.Status, "job.status"),
		requireString(wire.CreatedAt, "job.createdAt"),
		requireString(wire.UpdatedAt, "job.updatedAt"),
	); err != nil {
		return nil, err
	}
	job := &Job{
		CreatedAt:       wire.CreatedAt,
		ID:              wire.ID,
		MatchingID:      wire.MatchingID,
		Progress:        wire.Progress,
		ReleasedCredits: wire.ReleasedCredits,
		ReportID:        wire.ReportID,
		RequestID:       wire.RequestID,
		Stage:           wire.Stage,
		Status:          wire.Status,
		Type:            wire.Type,
		UpdatedAt:       wire.UpdatedAt,
	}
	if wire.Credits != nil {
		job.Credits = &JobCreditReservation{ReservedCredits: wire.Credits.ReservedCredits, ReservationStatus: wire.Credits.ReservationStatus}
	}
	if wire.Error != nil {
		job.Error = &JobErrorData{Code: wire.Error.Code, Details: wire.Error.Details, Message: wire.Error.Message, Retryable: wire.Error.Retryable}
	}
	if wire.Usage != nil {
		job.Usage = &Usage{
			CreditsDiscounted: wire.Usage.CreditsDiscounted,
			CreditsNetUsed:    wire.Usage.CreditsNetUsed,
			CreditsUsed:       wire.Usage.CreditsUsed,
			DurationMS:        wire.Usage.DurationMS,
			ModelVersion:      wire.Usage.ModelVersion,
		}
	}
	return job, nil
}

func parseJobReceipt(data []byte) (*reportJobReceipt, error) {
	var wire reportJobReceipt
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(requireString(wire.JobID, "jobReceipt.jobId"), requireString(wire.Status, "jobReceipt.status")); err != nil {
		return nil, err
	}
	return &wire, nil
}

func parseReport(data []byte) (*Report, error) {
	var wire reportWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.ID, "report.id"),
		requireString(wire.CreatedAt, "report.createdAt"),
		requireString(wire.Output.Template, "report.output.template"),
	); err != nil {
		return nil, err
	}
	return &Report{
		CreatedAt:   wire.CreatedAt,
		EntityID:    wire.Entity.ID,
		EntityLabel: wire.Entity.Label,
		ID:          wire.ID,
		JobID:       wire.JobID,
		Label:       wire.Label,
		MediaID:     wire.Media.MediaID,
		Output: ReportOutput{
			JSON:      wire.JSON,
			Markdown:  wire.Markdown,
			ReportURL: wire.Media.URL,
			Template:  wire.Output.Template,
		},
	}, nil
}

func parseMatching(data []byte) (*MatchingAnalysisResponse, error) {
	var wire matchingWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.ID, "matching.id"),
		requireString(wire.CreatedAt, "matching.createdAt"),
		requireString(wire.Context, "matching.context"),
	); err != nil {
		return nil, err
	}
	group := make([]MatchingResolvedSubject, 0, len(wire.Group))
	for _, item := range wire.Group {
		group = append(group, MatchingResolvedSubject{EntityID: item.EntityID, ResolvedLabel: item.ResolvedLabel, Source: item.Source})
	}
	var target *MatchingResolvedSubject
	if wire.Target != nil {
		target = &MatchingResolvedSubject{EntityID: wire.Target.EntityID, ResolvedLabel: wire.Target.ResolvedLabel, Source: wire.Target.Source}
	}
	return &MatchingAnalysisResponse{
		Context:   wire.Context,
		CreatedAt: wire.CreatedAt,
		Group:     group,
		ID:        wire.ID,
		JobID:     wire.JobID,
		Label:     wire.Label,
		Output: MatchingOutput{
			JSON:     wire.JSON,
			Markdown: wire.Markdown,
		},
		Target: target,
	}, nil
}

func parseMediaObject(data []byte) (*MediaObject, error) {
	var wire mediaObjectWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.MediaID, "media.mediaId"),
		requireString(wire.CreatedAt, "media.createdAt"),
		requireString(wire.ContentType, "media.contentType"),
		requireString(wire.Label, "media.label"),
	); err != nil {
		return nil, err
	}
	return &MediaObject{ContentType: wire.ContentType, CreatedAt: wire.CreatedAt, DurationSeconds: wire.DurationSeconds, Label: wire.Label, MediaID: wire.MediaID, SizeBytes: wire.SizeBytes}, nil
}

func parseMediaFile(data []byte) (*MediaFile, error) {
	var wire mediaFileWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.MediaID, "media.mediaId"),
		requireString(wire.CreatedAt, "media.createdAt"),
		requireString(wire.ContentType, "media.contentType"),
		requireString(wire.Label, "media.label"),
		requireString(wire.ProcessingStatus, "media.processingStatus"),
	); err != nil {
		return nil, err
	}
	return &MediaFile{
		ContentType:      wire.ContentType,
		CreatedAt:        wire.CreatedAt,
		DurationSeconds:  wire.DurationSeconds,
		Label:            wire.Label,
		LastUsedAt:       wire.LastUsedAt,
		MediaID:          wire.MediaID,
		ProcessingStatus: wire.ProcessingStatus,
		Retention: MediaRetention{
			DaysRemaining: wire.Retention.DaysRemaining,
			ExpiresAt:     wire.Retention.ExpiresAt,
			Locked:        wire.Retention.Locked,
		},
		SizeBytes: wire.SizeBytes,
	}, nil
}

func parseListFiles(data []byte) (*ListFilesResponse, error) {
	var payload struct {
		Files      []mediaFileWire `json:"files"`
		HasMore    bool            `json:"hasMore"`
		NextCursor string          `json:"nextCursor"`
	}
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	files := make([]MediaFile, 0, len(payload.Files))
	for _, item := range payload.Files {
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, &ConduitError{Code: "invalid_response", Message: "invalid files response", Cause: err}
		}
		file, err := parseMediaFile(encoded)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	return &ListFilesResponse{Files: files, HasMore: payload.HasMore, NextCursor: payload.NextCursor}, nil
}

func parseDeleteReceipt(data []byte) (*FileDeleteReceipt, error) {
	var payload FileDeleteReceipt
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	if err := requireFields(requireString(payload.MediaID, "delete.mediaId")); err != nil {
		return nil, err
	}
	return &payload, nil
}

func parseRetentionLock(data []byte) (*RetentionLockResult, error) {
	var payload RetentionLockResult
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(payload.MediaID, "retention.mediaId"),
		requireString(payload.Message, "retention.message"),
	); err != nil {
		return nil, err
	}
	return &payload, nil
}

func parseEntity(data []byte) (*Entity, error) {
	var wire entityWire
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(wire.ID, "entity.id"),
		requireString(wire.CreatedAt, "entity.createdAt"),
	); err != nil {
		return nil, err
	}
	return &Entity{CreatedAt: wire.CreatedAt, ID: wire.ID, Label: wire.Label, LastSeenAt: wire.LastSeenAt, MediaCount: wire.MediaCount}, nil
}

func parseListEntities(data []byte) (*ListEntitiesResponse, error) {
	var payload struct {
		Cursor   string       `json:"cursor"`
		Entities []entityWire `json:"entities"`
		HasMore  bool         `json:"hasMore"`
	}
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	entities := make([]Entity, 0, len(payload.Entities))
	for _, item := range payload.Entities {
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, &ConduitError{Code: "invalid_response", Message: "invalid entities response", Cause: err}
		}
		entity, err := parseEntity(encoded)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *entity)
	}
	return &ListEntitiesResponse{Cursor: payload.Cursor, Entities: entities, HasMore: payload.HasMore}, nil
}

func parseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := decodeJSON(payload, &event); err != nil {
		return nil, err
	}
	if err := requireFields(
		requireString(event.ID, "webhook.id"),
		requireString(event.Type, "webhook.type"),
		requireString(event.CreatedAt, "webhook.createdAt"),
		requireString(event.Timestamp, "webhook.timestamp"),
	); err != nil {
		return nil, err
	}
	return &event, nil
}

func decodeJSON(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return &ConduitError{Code: "invalid_response", Message: "invalid JSON response", Cause: err}
	}
	return nil
}

func parseISO8601(value string, name string) error {
	if strings.TrimSpace(value) == "" {
		return &ConduitError{Code: "invalid_webhook_payload", Message: fmt.Sprintf("%s must be an ISO8601 string", name)}
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: fmt.Sprintf("%s must be an ISO8601 string", name), Cause: err}
	}
	return nil
}

func requireFields(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func requireString(value string, name string) error {
	if strings.TrimSpace(value) == "" {
		return &ConduitError{Code: "invalid_response", Message: fmt.Sprintf("invalid %s: expected string", name)}
	}
	return nil
}
