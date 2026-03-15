package conduit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	internalwire "github.com/mappa-ai/conduit-go/internal/wire"
)

// JobErrorData describes terminal job failure details returned by the API.
type JobErrorData struct {
	// Code is the stable job error code.
	Code string
	// Details carries any structured failure metadata returned by the API.
	Details any
	// Message is the human-readable failure message.
	Message string
	// Retryable reports whether the job failure may succeed on retry when known.
	Retryable *bool
}

// Usage describes job usage and billing metadata.
type Usage struct {
	// CreditsDiscounted is the discounted credit amount when the API reports it.
	CreditsDiscounted *float64
	// CreditsNetUsed is the net credit usage charged for the job.
	CreditsNetUsed float64
	// CreditsUsed is the raw credit usage before discounts.
	CreditsUsed float64
	// DurationMS is the job duration in milliseconds when reported.
	DurationMS *float64
	// ModelVersion is the backend model version when reported.
	ModelVersion string
}

// JobCreditReservation describes reserved-credit lifecycle details.
type JobCreditReservation struct {
	// ReservedCredits is the reserved credit amount when present.
	ReservedCredits *float64
	// ReservationStatus is one of active, released, or applied when present.
	ReservationStatus string
}

// Job is the canonical async job shape returned by jobs endpoints and handles.
type Job struct {
	// Credits contains reserved-credit metadata when available.
	Credits *JobCreditReservation
	// CreatedAt is the ISO8601 job creation timestamp.
	CreatedAt string
	// Error contains structured terminal failure details for failed jobs.
	Error *JobErrorData
	// ID is the stable job identifier.
	ID string
	// MatchingID is set for successful matching jobs.
	MatchingID string
	// Progress is advisory progress between 0 and 1 when reported.
	Progress *float64
	// ReleasedCredits is the released credit amount when reported.
	ReleasedCredits *float64
	// ReportID is set for successful report jobs.
	ReportID string
	// RequestID is the originating API request identifier when available.
	RequestID string
	// Stage is the current advisory processing stage when present.
	Stage string
	// Status is one of queued, running, succeeded, failed, or canceled.
	Status string
	// Type is one of report.generate or matching.generate.
	Type string
	// UpdatedAt is the ISO8601 timestamp for the last job update.
	UpdatedAt string
	// Usage contains job usage metadata when available.
	Usage *Usage
}

// JobEvent is emitted by JobStream and Wait callbacks.
type JobEvent struct {
	// Job is the latest job snapshot attached to the event.
	Job *Job
	// Progress is advisory progress for stage events when reported.
	Progress *float64
	// Stage is the current stage for stage events.
	Stage string
	// Type is one of status, stage, or terminal.
	Type string
}

// ReportOutput contains rendered report representations.
type ReportOutput struct {
	// JSON contains the structured report payload when available.
	JSON map[string]any
	// Markdown contains the markdown rendering when available.
	Markdown string
	// ReportURL contains a report URL when available.
	ReportURL string
	// Template is the stable template identifier used to produce the report.
	Template string
}

// Report is the completed report resource.
type Report struct {
	// CreatedAt is the ISO8601 report creation timestamp.
	CreatedAt string
	// EntityID is the resolved entity identifier when available.
	EntityID string
	// EntityLabel is the resolved entity label when available.
	EntityLabel string
	// ID is the stable report identifier.
	ID string
	// JobID is the originating job identifier when available.
	JobID string
	// Label is the report label when available.
	Label string
	// MediaID is the source media identifier when available.
	MediaID string
	// Output contains rendered report content.
	Output ReportOutput
}

// MatchingResolvedSubject describes a resolved matching subject in the result.
type MatchingResolvedSubject struct {
	// EntityID is the resolved entity identifier when available.
	EntityID string
	// ResolvedLabel is the resolved display label when available.
	ResolvedLabel string
	// Source contains source resolution metadata when provided by the API.
	Source map[string]any
}

// MatchingOutput contains rendered matching result representations.
type MatchingOutput struct {
	// JSON contains the structured matching payload when available.
	JSON map[string]any
	// Markdown contains the markdown rendering when available.
	Markdown string
}

// MatchingAnalysisResponse is the completed matching result resource.
type MatchingAnalysisResponse struct {
	// Context is the stable matching context identifier.
	Context string
	// CreatedAt is the ISO8601 matching creation timestamp.
	CreatedAt string
	// Group contains resolved comparison subjects.
	Group []MatchingResolvedSubject
	// ID is the stable matching identifier.
	ID string
	// JobID is the originating job identifier when available.
	JobID string
	// Label is the matching label when available.
	Label string
	// Output contains rendered matching content.
	Output MatchingOutput
	// Target contains the resolved target subject when available.
	Target *MatchingResolvedSubject
}

// MediaObject is returned immediately after upload.
type MediaObject struct {
	// ContentType is the uploaded media content type.
	ContentType string
	// CreatedAt is the ISO8601 media creation timestamp.
	CreatedAt string
	// DurationSeconds is the detected media duration when available.
	DurationSeconds *float64
	// Label is the media label.
	Label string
	// MediaID is the stable uploaded media identifier.
	MediaID string
	// SizeBytes is the media size in bytes when available.
	SizeBytes *int64
}

// MediaRetention describes media retention state.
type MediaRetention struct {
	// DaysRemaining is the remaining retention window when available.
	DaysRemaining *int64
	// ExpiresAt is the ISO8601 retention expiry timestamp when available.
	ExpiresAt string
	// Locked reports whether retention lock is enabled.
	Locked bool
}

// MediaFile is the stored media resource returned by media APIs.
type MediaFile struct {
	// ContentType is the uploaded media content type.
	ContentType string
	// CreatedAt is the ISO8601 media creation timestamp.
	CreatedAt string
	// DurationSeconds is the detected media duration when available.
	DurationSeconds *float64
	// Label is the media label.
	Label string
	// LastUsedAt is the ISO8601 timestamp for the last use when available.
	LastUsedAt string
	// MediaID is the stable media identifier.
	MediaID string
	// ProcessingStatus is the backend processing status.
	ProcessingStatus string
	// Retention contains retention lock and expiry metadata.
	Retention MediaRetention
	// SizeBytes is the media size in bytes when available.
	SizeBytes *int64
}

// FileDeleteReceipt acknowledges media deletion.
type FileDeleteReceipt struct {
	// Deleted reports whether the media was deleted.
	Deleted bool
	// MediaID is the deleted media identifier.
	MediaID string
}

// RetentionLockResult acknowledges a retention lock update.
type RetentionLockResult struct {
	// MediaID is the updated media identifier.
	MediaID string
	// Message is the API acknowledgment message.
	Message string
	// RetentionLock reports the resulting retention lock state.
	RetentionLock bool
}

// ListFilesResponse is the paginated media list response.
type ListFilesResponse struct {
	// Files contains the current page of media files.
	Files []MediaFile
	// HasMore reports whether more results are available.
	HasMore bool
	// NextCursor is the cursor to request the next page.
	NextCursor string
}

// Entity is the stable speaker identity resource.
type Entity struct {
	// CreatedAt is the ISO8601 entity creation timestamp.
	CreatedAt string
	// ID is the stable entity identifier.
	ID string
	// Label is the optional entity label.
	Label string
	// LastSeenAt is the ISO8601 timestamp of the latest related media when available.
	LastSeenAt string
	// MediaCount is the number of associated media records.
	MediaCount float64
}

// ListEntitiesResponse is the paginated entity list response.
type ListEntitiesResponse struct {
	// Cursor is the server cursor for the current page.
	Cursor string
	// Entities contains the current page of entities.
	Entities []Entity
	// HasMore reports whether more results are available.
	HasMore bool
}

// WebhookEvent is the parsed webhook envelope.
type WebhookEvent struct {
	// CreatedAt is the canonical event creation timestamp.
	CreatedAt string
	// Data is the event payload. Known event types are validated but remain untyped.
	Data any
	// ID is the stable event identifier used for deduplication.
	ID string
	// Timestamp duplicates CreatedAt for compatibility with event processors.
	Timestamp string
	// Type is the event type, such as report.completed.
	Type string
}

func parseJob(data []byte) (*Job, error) {
	var wire internalwire.Job
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

func parseJobReceipt(data []byte) (*internalwire.JobReceipt, error) {
	var wire internalwire.JobReceipt
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	if err := requireFields(requireString(wire.JobID, "jobReceipt.jobId"), requireString(wire.Status, "jobReceipt.status")); err != nil {
		return nil, err
	}
	return &wire, nil
}

func parseReport(data []byte) (*Report, error) {
	var wire internalwire.Report
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
	var wire internalwire.Matching
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
	var wire internalwire.MediaObject
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
	var wire internalwire.MediaFile
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
	var payload internalwire.ListFilesResponse
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	files := make([]MediaFile, 0, len(payload.Files))
	for _, item := range payload.Files {
		file, err := mapMediaFile(item)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	return &ListFilesResponse{Files: files, HasMore: payload.HasMore, NextCursor: payload.NextCursor}, nil
}

func mapMediaFile(wire internalwire.MediaFile) (*MediaFile, error) {
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
	var wire internalwire.Entity
	if err := decodeJSON(data, &wire); err != nil {
		return nil, err
	}
	return mapEntity(wire)
}

func mapEntity(wire internalwire.Entity) (*Entity, error) {
	if err := requireFields(
		requireString(wire.ID, "entity.id"),
		requireString(wire.CreatedAt, "entity.createdAt"),
	); err != nil {
		return nil, err
	}
	return &Entity{CreatedAt: wire.CreatedAt, ID: wire.ID, Label: wire.Label, LastSeenAt: wire.LastSeenAt, MediaCount: wire.MediaCount}, nil
}

func parseListEntities(data []byte) (*ListEntitiesResponse, error) {
	var payload internalwire.ListEntitiesResponse
	if err := decodeJSON(data, &payload); err != nil {
		return nil, err
	}
	entities := make([]Entity, 0, len(payload.Entities))
	for _, item := range payload.Entities {
		entity, err := mapEntity(item)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *entity)
	}
	return &ListEntitiesResponse{Cursor: payload.Cursor, Entities: entities, HasMore: payload.HasMore}, nil
}

func parseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var event internalwire.WebhookEvent
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
	return &WebhookEvent{CreatedAt: event.CreatedAt, Data: event.Data, ID: event.ID, Timestamp: event.Timestamp, Type: event.Type}, nil
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
