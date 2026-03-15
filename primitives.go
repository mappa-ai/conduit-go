package conduit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var labelSuffixPattern = regexp.MustCompile(`(\.[^.]+)+$`)

// PrimitivesResource exposes the stable advanced SDK surface.
type PrimitivesResource struct {
	// Entities exposes stable entity lookup and update helpers.
	Entities *EntitiesResource
	// Jobs exposes stable job inspection and cancellation helpers.
	Jobs *JobsResource
	// Media exposes stable upload and media management helpers.
	Media *MediaResource
}

// StreamOptions configures handle stream resumption.
type StreamOptions struct {
	// LastEventID resumes an SSE stream from a previously seen event ID.
	LastEventID string
}

// WaitOptions configures handle wait helpers.
type WaitOptions struct {
	// LastEventID resumes the underlying SSE stream from a previously seen event ID.
	LastEventID string
	// OnEvent is invoked for each streamed job event before Wait returns.
	OnEvent func(JobEvent)
	// Timeout applies an SDK-side deadline to the wait operation.
	Timeout time.Duration
}

// Source identifies a report or media upload source.
type Source struct {
	kind     sourceKind
	fileName string
	label    string
	mediaID  string
	path     string
	reader   io.Reader
	rawURL   string
	data     []byte
}

type sourceKind string

const (
	sourceKindBytes   sourceKind = "bytes"
	sourceKindMediaID sourceKind = "media_id"
	sourceKindPath    sourceKind = "path"
	sourceKindReader  sourceKind = "reader"
	sourceKindURL     sourceKind = "url"
)

type MediaUploadRequest struct {
	// IdempotencyKey scopes retries and duplicate submission protection for upload.
	IdempotencyKey string
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
	// Source identifies the media to upload.
	Source Source
}

// CancelJobRequest requests cancellation for a running job.
type CancelJobRequest struct {
	// IdempotencyKey scopes retries and duplicate submission protection for cancel.
	IdempotencyKey string
	// JobID is the job to cancel.
	JobID string
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
}

// DeleteMediaRequest requests media deletion.
type DeleteMediaRequest struct {
	// IdempotencyKey scopes retries and duplicate submission protection for delete.
	IdempotencyKey string
	// MediaID is the media object to delete.
	MediaID string
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
}

// SetRetentionLockRequest updates retention lock state for media.
type SetRetentionLockRequest struct {
	// Locked toggles retention lock state.
	Locked bool
	// MediaID is the media object to update.
	MediaID string
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
}

// UpdateEntityRequest updates stable entity metadata.
type UpdateEntityRequest struct {
	// EntityID is the entity to update.
	EntityID string
	// Label sets or clears the optional entity label.
	Label *string
	// RequestID overrides the SDK-generated request identifier.
	RequestID string
}

type uploadMaterialization struct {
	contentType string
	fileName    string
	label       string
	payload     []byte
}

// JobsResource exposes low-level job inspection helpers.
type JobsResource struct {
	transport *transport
}

// EntitiesResource exposes low-level entity helpers.
type EntitiesResource struct {
	transport *transport
}

// MediaResource exposes low-level media helpers.
type MediaResource struct {
	maxSourceBytes int64
	timeout        time.Duration
	transport      *transport
}

// SourceBytes creates an in-memory source using the provided file name and bytes.
func SourceBytes(fileName string, data []byte) Source {
	return Source{kind: sourceKindBytes, fileName: fileName, data: data}
}

// SourceMediaID references previously uploaded media.
func SourceMediaID(mediaID string) Source {
	return Source{kind: sourceKindMediaID, mediaID: mediaID}
}

// SourcePath creates a local filesystem source. Relative paths resolve from the
// current working directory.
func SourcePath(path string) Source {
	return Source{kind: sourceKindPath, path: path}
}

// SourceReader creates a reader-backed source. The SDK may still materialize the
// reader in memory before upload.
func SourceReader(fileName string, reader io.Reader) Source {
	return Source{kind: sourceKindReader, fileName: fileName, reader: reader}
}

// SourceURL creates a remote HTTP(S) source fetched by the SDK host runtime
// before upload.
func SourceURL(rawURL string) Source {
	return Source{kind: sourceKindURL, rawURL: rawURL}
}

// WithLabel overrides the inferred media label used for upload.
func (s Source) WithLabel(label string) Source {
	s.label = label
	return s
}

// Get fetches the canonical job state by ID.
func (r *JobsResource) Get(ctx context.Context, jobID string) (*Job, error) {
	validated, err := requireNonEmpty(jobID, "jobID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	return parseJob(response.body)
}

// Cancel requests cancellation for an in-flight job.
func (r *JobsResource) Cancel(ctx context.Context, request CancelJobRequest) (*Job, error) {
	validated, err := requireNonEmpty(request.JobID, "jobID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(validated)+"/cancel", requestOptions{
		idempotencyKey: request.IdempotencyKey,
		requestID:      request.RequestID,
		retryable:      true,
	})
	if err != nil {
		return nil, err
	}
	return parseJob(response.body)
}

// Get fetches a stable entity by ID.
func (r *EntitiesResource) Get(ctx context.Context, entityID string) (*Entity, error) {
	validated, err := requireNonEmpty(entityID, "entityID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/entities/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	return parseEntity(response.body)
}

// List fetches entities using cursor pagination. When limit is zero or negative,
// the SDK defaults to 20.
func (r *EntitiesResource) List(ctx context.Context, limit int, cursor string) (*ListEntitiesResponse, error) {
	query := url.Values{}
	if limit <= 0 {
		limit = 20
	}
	query.Set("limit", fmt.Sprintf("%d", limit))
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", cursor)
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/entities", requestOptions{query: query, retryable: true})
	if err != nil {
		return nil, err
	}
	return parseListEntities(response.body)
}

// Update sets or clears the optional entity label.
func (r *EntitiesResource) Update(ctx context.Context, request UpdateEntityRequest) (*Entity, error) {
	validated, err := requireNonEmpty(request.EntityID, "entityID")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"label": request.Label})
	if err != nil {
		return nil, &ConduitError{Code: "invalid_request", Message: "failed to encode entity update", Cause: err}
	}
	response, err := r.transport.request(ctx, http.MethodPatch, "/v1/entities/"+url.PathEscape(validated), requestOptions{
		body:        body,
		contentType: "application/json",
		requestID:   request.RequestID,
		retryable:   true,
	})
	if err != nil {
		return nil, err
	}
	return parseEntity(response.body)
}

// Upload materializes the provided source and uploads it as media.
func (r *MediaResource) Upload(ctx context.Context, request MediaUploadRequest) (*MediaObject, error) {
	materialized, err := r.materializeSource(ctx, request.Source)
	if err != nil {
		return nil, err
	}
	body, contentType, err := buildMultipartBody(map[string]string{"label": materialized.label}, materialized)
	if err != nil {
		return nil, &ConduitError{Code: "invalid_request", Message: "failed to build upload request", Cause: err}
	}
	response, err := r.transport.request(ctx, http.MethodPost, "/v1/files", requestOptions{
		body:           body,
		contentType:    contentType,
		idempotencyKey: request.IdempotencyKey,
		requestID:      request.RequestID,
		retryable:      true,
	})
	if err != nil {
		return nil, err
	}
	return parseMediaObject(response.body)
}

// Get fetches one uploaded media object by ID.
func (r *MediaResource) Get(ctx context.Context, mediaID string) (*MediaFile, error) {
	validated, err := requireNonEmpty(mediaID, "mediaID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/files/"+url.PathEscape(validated), requestOptions{retryable: true})
	if err != nil {
		return nil, err
	}
	return parseMediaFile(response.body)
}

// List fetches uploaded media using cursor pagination. When limit is zero or
// negative, the SDK defaults to 20.
func (r *MediaResource) List(ctx context.Context, limit int, cursor string, includeDeleted bool) (*ListFilesResponse, error) {
	query := url.Values{}
	if limit <= 0 {
		limit = 20
	}
	query.Set("includeDeleted", fmt.Sprintf("%t", includeDeleted))
	query.Set("limit", fmt.Sprintf("%d", limit))
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", cursor)
	}
	response, err := r.transport.request(ctx, http.MethodGet, "/v1/files", requestOptions{query: query, retryable: true})
	if err != nil {
		return nil, err
	}
	return parseListFiles(response.body)
}

// Delete deletes media by ID.
func (r *MediaResource) Delete(ctx context.Context, request DeleteMediaRequest) (*FileDeleteReceipt, error) {
	validated, err := requireNonEmpty(request.MediaID, "mediaID")
	if err != nil {
		return nil, err
	}
	response, err := r.transport.request(ctx, http.MethodDelete, "/v1/files/"+url.PathEscape(validated), requestOptions{
		idempotencyKey: request.IdempotencyKey,
		requestID:      request.RequestID,
		retryable:      true,
	})
	if err != nil {
		return nil, err
	}
	return parseDeleteReceipt(response.body)
}

// SetRetentionLock updates retention lock state for media.
func (r *MediaResource) SetRetentionLock(ctx context.Context, request SetRetentionLockRequest) (*RetentionLockResult, error) {
	validated, err := requireNonEmpty(request.MediaID, "mediaID")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]bool{"lock": request.Locked})
	if err != nil {
		return nil, &ConduitError{Code: "invalid_request", Message: "failed to encode retention request", Cause: err}
	}
	response, err := r.transport.request(ctx, http.MethodPatch, "/v1/files/"+url.PathEscape(validated)+"/retention", requestOptions{
		body:        body,
		contentType: "application/json",
		requestID:   request.RequestID,
		retryable:   true,
	})
	if err != nil {
		return nil, err
	}
	return parseRetentionLock(response.body)
}

func (r *MediaResource) resolveReportSource(ctx context.Context, source Source, idempotencyKey string, requestID string) (string, error) {
	if source.kind == sourceKindMediaID {
		return requireNonEmpty(source.mediaID, "source.mediaId")
	}
	media, err := r.Upload(ctx, MediaUploadRequest{IdempotencyKey: idempotencyKey, RequestID: requestID, Source: source})
	if err != nil {
		return "", err
	}
	return media.MediaID, nil
}

func (r *MediaResource) materializeSource(ctx context.Context, source Source) (*uploadMaterialization, error) {
	switch source.kind {
	case sourceKindPath:
		return r.materializePath(source)
	case sourceKindURL:
		return r.materializeURL(ctx, source)
	case sourceKindBytes:
		return r.materializeBytes(source)
	case sourceKindReader:
		return r.materializeReader(source)
	case sourceKindMediaID:
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("upload source cannot use mediaId", "invalid_source", "", nil)}}
	default:
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source must be file, url, path, or mediaId", "invalid_source", "", nil)}}
	}
}

func (r *MediaResource) materializePath(source Source) (*uploadMaterialization, error) {
	validatedPath, err := requireNonEmpty(source.path, "source.path")
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(validatedPath)
	if err != nil {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError(fmt.Sprintf("file not found: %s", validatedPath), "invalid_source", "", err)}}
	}
	if info.IsDir() {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError(fmt.Sprintf("path is a directory: %s", validatedPath), "invalid_source", "", nil)}}
	}
	if info.Size() > r.maxSourceBytes {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source.path exceeds upload size limit", "source_too_large", "", nil)}}
	}
	payload, err := os.ReadFile(validatedPath)
	if err != nil {
		return nil, &SourceError{ConduitError: newConduitError("failed to read source.path", "source_error", "", err)}
	}
	name := filepath.Base(validatedPath)
	return &uploadMaterialization{contentType: contentTypeFromName(name), fileName: name, label: resolveLabel(source.label, name, "upload"), payload: payload}, nil
}

func (r *MediaResource) materializeBytes(source Source) (*uploadMaterialization, error) {
	if len(source.data) == 0 {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source.file is required", "invalid_source", "", nil)}}
	}
	if int64(len(source.data)) > r.maxSourceBytes {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source.file exceeds upload size limit", "source_too_large", "", nil)}}
	}
	name := source.fileName
	if strings.TrimSpace(name) == "" {
		name = "upload.bin"
	}
	return &uploadMaterialization{contentType: contentTypeFromName(name), fileName: name, label: resolveLabel(source.label, name, "upload"), payload: source.data}, nil
}

func (r *MediaResource) materializeReader(source Source) (*uploadMaterialization, error) {
	if source.reader == nil {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source.file is required", "invalid_source", "", nil)}}
	}
	limited := io.LimitReader(source.reader, r.maxSourceBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, &SourceError{ConduitError: newConduitError("failed to read source.file", "source_error", "", err)}
	}
	if int64(len(payload)) > r.maxSourceBytes {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError("source.file exceeds upload size limit", "source_too_large", "", nil)}}
	}
	name := source.fileName
	if strings.TrimSpace(name) == "" {
		name = "upload.bin"
	}
	return &uploadMaterialization{contentType: contentTypeFromName(name), fileName: name, label: resolveLabel(source.label, name, "upload"), payload: payload}, nil
}

func (r *MediaResource) materializeURL(ctx context.Context, source Source) (*uploadMaterialization, error) {
	parsed, err := parseHTTPURL(source.rawURL, "source.url")
	if err != nil {
		return nil, err
	}
	const maxRedirects = 5
	downloadClient := *r.transport.httpClient
	originalCheckRedirect := downloadClient.CheckRedirect
	downloadClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("redirect limit exceeded")
		}
		if originalCheckRedirect != nil {
			return originalCheckRedirect(request, via)
		}
		return nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch failed", "remote_fetch_failed", "", err), URL: parsed.String()}}
	}
	request.Header = r.transport.defaultHeaders(randomID("req"))
	response, err := downloadClient.Do(request)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return nil, &RemoteFetchTimeoutError{RemoteFetchError: RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch timed out", "remote_fetch_timeout", "", err), URL: parsed.String()}}}
		}
		if strings.Contains(err.Error(), "redirect limit exceeded") {
			return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch exceeded redirect limit", "remote_fetch_redirects_exhausted", "", err), URL: parsed.String()}}
		}
		return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch failed", "remote_fetch_failed", "", err), URL: parsed.String()}}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError(fmt.Sprintf("remote fetch failed with status %d", response.StatusCode), "remote_fetch_failed", "", nil), Status: response.StatusCode, URL: parsed.String()}}
	}
	if contentLength := response.Header.Get("Content-Length"); contentLength != "" {
		if parsedLength, parseErr := strconv.ParseInt(contentLength, 10, 64); parseErr == nil && parsedLength > r.maxSourceBytes {
			return nil, &RemoteFetchTooLargeError{RemoteFetchError: RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("source.url exceeds upload size limit", "source_too_large", "", nil), Status: response.StatusCode, URL: parsed.String()}}}
		}
	}
	limited := io.LimitReader(response.Body, r.maxSourceBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch failed", "remote_fetch_failed", "", err), Status: response.StatusCode, URL: parsed.String()}}
	}
	if int64(len(payload)) > r.maxSourceBytes {
		return nil, &RemoteFetchTooLargeError{RemoteFetchError: RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("source.url exceeds upload size limit", "source_too_large", "", nil), Status: response.StatusCode, URL: parsed.String()}}}
	}
	fileName := fileNameFromURL(response.Request.URL.String())
	return &uploadMaterialization{contentType: stripContentType(response.Header.Get("Content-Type")), fileName: fileName, label: resolveLabel(source.label, fileName, "remote"), payload: payload}, nil
}

func buildMultipartBody(fields map[string]string, file *uploadMaterialization) ([]byte, string, error) {
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	fileWriter, err := writer.CreateFormFile("file", file.fileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := fileWriter.Write(file.payload); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func requireNonEmpty(value string, name string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", &ConduitError{Code: "invalid_request", Message: fmt.Sprintf("%s must be a non-empty string", name)}
	}
	return trimmed, nil
}

func resolveLabel(label string, fileName string, fallback string) string {
	if strings.TrimSpace(label) != "" {
		return normalizeLabel(label)
	}
	normalized := normalizeLabel(fileName)
	if normalized != "" {
		return normalized
	}
	return fallback
}

func normalizeLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = labelSuffixPattern.ReplaceAllString(trimmed, "")
	return strings.TrimSpace(trimmed)
}

func fileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "remote.bin"
	}
	name := pathpkg.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "remote.bin"
	}
	return name
}

func isTerminalStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "canceled":
		return true
	default:
		return false
	}
}
