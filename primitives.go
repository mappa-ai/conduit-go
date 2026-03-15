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

type PrimitivesResource struct {
	Entities *EntitiesResource
	Jobs     *JobsResource
	Media    *MediaResource
}

type StreamOptions struct {
	OnEvent      func(JobEvent)
	PollInterval time.Duration
	Timeout      time.Duration
}

type WaitOptions = StreamOptions

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
	IdempotencyKey string
	RequestID      string
	Source         Source
}

type CancelJobRequest struct {
	IdempotencyKey string
	JobID          string
	RequestID      string
}

type DeleteMediaRequest struct {
	IdempotencyKey string
	MediaID        string
	RequestID      string
}

type SetRetentionLockRequest struct {
	Locked    bool
	MediaID   string
	RequestID string
}

type UpdateEntityRequest struct {
	EntityID  string
	Label     *string
	RequestID string
}

type uploadMaterialization struct {
	contentType string
	fileName    string
	label       string
	payload     []byte
}

type JobsResource struct {
	pollInterval time.Duration
	transport    *transport
}

type EntitiesResource struct {
	transport *transport
}

type MediaResource struct {
	maxSourceBytes int64
	timeout        time.Duration
	transport      *transport
}

func SourceBytes(fileName string, data []byte) Source {
	return Source{kind: sourceKindBytes, fileName: fileName, data: data}
}

func SourceMediaID(mediaID string) Source {
	return Source{kind: sourceKindMediaID, mediaID: mediaID}
}

func SourcePath(path string) Source {
	return Source{kind: sourceKindPath, path: path}
}

func SourceReader(fileName string, reader io.Reader) Source {
	return Source{kind: sourceKindReader, fileName: fileName, reader: reader}
}

func SourceURL(rawURL string) Source {
	return Source{kind: sourceKindURL, rawURL: rawURL}
}

func (s Source) WithLabel(label string) Source {
	s.label = label
	return s
}

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

func (r *JobsResource) stream(ctx context.Context, jobID string, options *StreamOptions) (*Job, error) {
	validatedJobID, err := requireNonEmpty(jobID, "jobID")
	if err != nil {
		return nil, err
	}
	pollInterval := r.pollInterval
	if options != nil && options.PollInterval > 0 {
		pollInterval = options.PollInterval
	}
	streamTimeout := 5 * time.Minute
	if options != nil && options.Timeout > 0 {
		streamTimeout = options.Timeout
	}
	streamCtx, cancel := context.WithTimeout(ctx, streamTimeout)
	defer cancel()

	lastStatus := ""
	lastStage := ""
	for {
		job, err := r.Get(streamCtx, validatedJobID)
		if err != nil {
			return nil, &StreamError{ConduitError: newConduitError("failed to fetch job status", "stream_error", "", err), JobID: validatedJobID}
		}
		if job.Status != lastStatus {
			r.emit(options, JobEvent{Job: job, Type: "status"})
			lastStatus = job.Status
		}
		if job.Stage != "" && job.Stage != lastStage {
			r.emit(options, JobEvent{Job: job, Progress: job.Progress, Stage: job.Stage, Type: "stage"})
			lastStage = job.Stage
		}
		if isTerminalStatus(job.Status) {
			r.emit(options, JobEvent{Job: job, Type: "terminal"})
			return job, nil
		}
		if err := sleepWithContext(streamCtx, pollInterval); err != nil {
			var aborted *RequestAbortedError
			if errors.As(err, &aborted) && errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
				return nil, &TimeoutError{ConduitError: newConduitError(fmt.Sprintf("timed out waiting for job %s after %dms", validatedJobID, streamTimeout.Milliseconds()), "timeout", "", streamCtx.Err())}
			}
			return nil, err
		}
	}
}

func (r *JobsResource) emit(options *StreamOptions, event JobEvent) {
	if options == nil || options.OnEvent == nil {
		return
	}
	options.OnEvent(event)
}

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
	downloadClient := &http.Client{
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("redirect limit exceeded")
			}
			return nil
		},
	}
	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, &RemoteFetchError{SourceError: SourceError{ConduitError: newConduitError("remote fetch failed", "remote_fetch_failed", "", err), URL: parsed.String()}}
	}
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
