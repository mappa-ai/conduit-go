package conduit

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type transport struct {
	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
	maxRetries int
	telemetry  *Telemetry
	timeout    time.Duration
	userAgent  string
}

type requestOptions struct {
	body           []byte
	contentType    string
	headers        map[string]string
	idempotencyKey string
	query          url.Values
	requestID      string
	retryable      bool
	timeout        time.Duration
}

type transportResponse struct {
	body      []byte
	headers   http.Header
	requestID string
	status    int
}

func newTransport(apiKey string, config clientConfig) *transport {
	httpClient := config.httpClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	baseURL, _ := url.Parse(config.baseURL)
	userAgent := defaultUserAgent
	if config.userAgent != "" {
		userAgent += " " + config.userAgent
	}
	return &transport{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: config.maxRetries,
		telemetry:  config.telemetry,
		timeout:    config.timeout,
		userAgent:  userAgent,
	}
}

func (t *transport) closeIdleConnections() {
	if t == nil || t.httpClient == nil {
		return
	}
	t.httpClient.CloseIdleConnections()
}

func (t *transport) request(ctx context.Context, method string, path string, options requestOptions) (*transportResponse, error) {
	requestID := options.requestID
	if requestID == "" {
		requestID = randomID("req")
	}
	attempts := 1
	if options.retryable {
		attempts += t.maxRetries
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		response, err := t.requestOnce(ctx, method, path, options, requestID)
		if err == nil {
			return response, nil
		}
		if attempt == attempts || !shouldRetry(err) {
			return nil, err
		}
		if sleepErr := sleepWithContext(ctx, retryDelay(err, attempt)); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return nil, &ConduitError{Code: "transport_error", Message: "unexpected transport exit", RequestID: requestID}
}

func (t *transport) requestOnce(ctx context.Context, method string, path string, options requestOptions, requestID string) (*transportResponse, error) {
	timeout := options.timeout
	if timeout <= 0 {
		timeout = t.timeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	requestURL := t.baseURL.ResolveReference(&url.URL{Path: path})
	if len(options.query) > 0 {
		requestURL.RawQuery = options.query.Encode()
	}
	startedAt := time.Now()

	request, err := http.NewRequestWithContext(requestCtx, method, requestURL.String(), bytes.NewReader(options.body))
	if err != nil {
		return nil, &ConduitError{Code: "transport_error", Message: "failed to create request", RequestID: requestID, Cause: err}
	}
	request.Header = t.defaultHeaders(requestID)
	request.Header.Set("Accept", "application/json")
	if options.idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", options.idempotencyKey)
	}
	if options.contentType != "" {
		request.Header.Set("Content-Type", options.contentType)
	}
	for key, value := range options.headers {
		request.Header.Set(key, value)
	}
	if t.telemetry != nil && t.telemetry.OnRequest != nil {
		t.telemetry.OnRequest(RequestTelemetry{Method: method, RequestID: requestID, URL: requestURL.String()})
	}

	response, err := t.httpClient.Do(request)
	if err != nil {
		if t.telemetry != nil && t.telemetry.OnError != nil {
			t.telemetry.OnError(ErrorTelemetry{Duration: time.Since(startedAt), Error: err, Method: method, RequestID: requestID, URL: requestURL.String()})
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &RequestAbortedError{ConduitError: newConduitError("request aborted by caller", "request_aborted", requestID, err)}
		}
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return nil, &TimeoutError{ConduitError: newConduitError(fmt.Sprintf("request timed out after %dms", timeout.Milliseconds()), "timeout", requestID, err)}
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, &TimeoutError{ConduitError: newConduitError(fmt.Sprintf("request timed out after %dms", timeout.Milliseconds()), "timeout", requestID, err)}
		}
		return nil, &ConduitError{Code: "transport_error", Message: "request failed", RequestID: requestID, Cause: err}
	}
	defer response.Body.Close()
	if t.telemetry != nil && t.telemetry.OnResponse != nil {
		t.telemetry.OnResponse(ResponseTelemetry{Duration: time.Since(startedAt), Method: method, RequestID: requestID, Status: response.StatusCode, URL: requestURL.String()})
	}

	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		if t.telemetry != nil && t.telemetry.OnError != nil {
			t.telemetry.OnError(ErrorTelemetry{Duration: time.Since(startedAt), Error: readErr, Method: method, RequestID: requestID, URL: requestURL.String()})
		}
		return nil, &ConduitError{Code: "transport_error", Message: "failed to read response", RequestID: requestID, Cause: readErr}
	}
	serverRequestID := response.Header.Get("X-Request-Id")
	if serverRequestID == "" {
		serverRequestID = requestID
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return &transportResponse{body: body, headers: response.Header.Clone(), requestID: serverRequestID, status: response.StatusCode}, nil
	}

	apiErr := parseAPIError(response.StatusCode, body, response.Header.Clone(), serverRequestID)
	return nil, apiErr
}

func (t *transport) defaultHeaders(requestID string) http.Header {
	headers := http.Header{}
	headers.Set("Mappa-Api-Key", t.apiKey)
	headers.Set("X-Request-Id", requestID)
	if t.userAgent != "" {
		headers.Set("User-Agent", t.userAgent)
	}
	return headers
}

func parseAPIError(status int, body []byte, headers http.Header, requestID string) error {
	message := "Request failed"
	code := "api_error"
	var details any

	var payload map[string]any
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if nested, ok := payload["error"].(map[string]any); ok {
			if value, ok := nested["message"].(string); ok && strings.TrimSpace(value) != "" {
				message = value
			}
			if value, ok := nested["code"].(string); ok && strings.TrimSpace(value) != "" {
				code = value
			}
			details = nested["details"]
		} else {
			if value, ok := payload["message"].(string); ok && strings.TrimSpace(value) != "" {
				message = value
			}
			if value, ok := payload["code"].(string); ok && strings.TrimSpace(value) != "" {
				code = value
			}
			details = payload
		}
	} else if len(body) > 0 {
		message = strings.TrimSpace(string(body))
	}

	err := newAPIError(status, requestID, message, code, details)
	if rateLimitErr, ok := err.(*RateLimitError); ok {
		rateLimitErr.RetryAfter = parseRetryAfter(headers.Get("Retry-After"))
	}
	return err
}

func shouldRetry(err error) bool {
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500
	}
	var timeoutErr *TimeoutError
	if errors.As(err, &timeoutErr) {
		return true
	}
	var conduitErr *ConduitError
	if errors.As(err, &conduitErr) {
		return conduitErr.Code == "transport_error"
	}
	return false
}

func retryDelay(err error, attempt int) time.Duration {
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		return rateLimitErr.RetryAfter
	}
	base := time.Duration(500*(1<<attempt)) * time.Millisecond
	if base > 4*time.Second {
		base = 4 * time.Second
	}
	randomBytes := make([]byte, 2)
	if _, err := crand.Read(randomBytes); err != nil {
		return base
	}
	jitter := time.Duration(int(randomBytes[0])%500) * time.Millisecond
	return base + jitter
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return &RequestAbortedError{ConduitError: newConduitError("request aborted by caller", "request_aborted", "", ctx.Err())}
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(trimmed); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(trimmed); err == nil {
		return time.Until(retryAt)
	}
	return 0
}

func parseHTTPURL(value string, name string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, &InvalidSourceError{SourceError: SourceError{ConduitError: newConduitError(fmt.Sprintf("%s must be an http or https URL", name), "invalid_source", "", err)}}
	}
	return parsed, nil
}

func randomID(prefix string) string {
	raw := make([]byte, 8)
	if _, err := crand.Read(raw); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func contentTypeFromName(name string) string {
	value := mime.TypeByExtension(strings.ToLower(fileExtension(name)))
	return stripContentType(value)
}

func fileExtension(name string) string {
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return ""
	}
	return name[lastDot:]
}

func stripContentType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(trimmed, ";", 2)[0])
}

func toFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	}
	return 0
}
