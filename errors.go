package conduit

import (
	"fmt"
	"time"
)

// ConduitError is the base error type for SDK failures.
type ConduitError struct {
	// Cause is the wrapped underlying error, when available.
	Cause error
	// Code is the stable SDK error code for programmatic branching.
	Code string
	// Message is the human-readable error message.
	Message string
	// RequestID is the API request identifier when one is available.
	RequestID string
}

func (e ConduitError) Error() string {
	if e.Message == "" {
		return "conduit error"
	}
	return e.Message
}

func (e ConduitError) Unwrap() error {
	return e.Cause
}

// InitializationError reports invalid client configuration.
type InitializationError struct{ ConduitError }

// UnsupportedRuntimeError reports behavior unavailable in the current runtime.
type UnsupportedRuntimeError struct{ ConduitError }

// WebhookVerificationError reports invalid webhook signatures or headers.
type WebhookVerificationError struct{ ConduitError }

// SourceError reports failures while resolving or uploading a source.
type SourceError struct {
	ConduitError
	// Status is the remote HTTP status code when the failure came from SourceURL.
	Status int
	// URL is the offending source URL when the failure came from SourceURL.
	URL string
}

// InvalidSourceError reports invalid local or remote source input.
type InvalidSourceError struct{ SourceError }

// RemoteFetchError reports failures while fetching a remote source URL.
type RemoteFetchError struct{ SourceError }

// RemoteFetchTimeoutError reports remote source fetch timeout failures.
type RemoteFetchTimeoutError struct{ RemoteFetchError }

// RemoteFetchTooLargeError reports remote sources exceeding the upload limit.
type RemoteFetchTooLargeError struct{ RemoteFetchError }

// APIError reports a non-2xx API response.
type APIError struct {
	ConduitError
	// Details carries any structured error details returned by the API.
	Details any
	// Status is the HTTP status code returned by the API.
	Status int
}

// AuthError reports authentication or authorization failures.
type AuthError struct{ APIError }

// ValidationError reports invalid request payloads rejected by the API.
type ValidationError struct{ APIError }

// RateLimitError reports rate-limited API requests.
type RateLimitError struct {
	APIError
	// RetryAfter is the server-provided retry delay when available.
	RetryAfter time.Duration
}

// InsufficientCreditsError reports insufficient credit balance.
type InsufficientCreditsError struct {
	APIError
	// Available is the available credit balance reported by the API.
	Available float64
	// Required is the required credit amount reported by the API.
	Required float64
}

// JobFailedError reports terminal job failures.
type JobFailedError struct {
	ConduitError
	// JobID is the failed job identifier.
	JobID string
}

// JobCanceledError reports terminal canceled jobs.
type JobCanceledError struct {
	ConduitError
	// JobID is the canceled job identifier.
	JobID string
}

// TimeoutError reports SDK-enforced request or wait timeouts.
type TimeoutError struct{ ConduitError }

// RequestAbortedError reports caller-triggered cancellation.
type RequestAbortedError struct{ ConduitError }

// StreamError reports SSE stream failures after retries are exhausted.
type StreamError struct {
	ConduitError
	// JobID is the job whose stream failed.
	JobID string
	// LastEventID is the last successfully processed SSE event ID.
	LastEventID string
	// RetryCount is the number of reconnect attempts performed by the SDK.
	RetryCount int
}

func newConduitError(message string, code string, requestID string, cause error) ConduitError {
	return ConduitError{Cause: cause, Code: code, Message: message, RequestID: requestID}
}

func newAPIError(status int, requestID string, message string, code string, details any) error {
	base := APIError{
		ConduitError: newConduitError(message, code, requestID, nil),
		Details:      details,
		Status:       status,
	}
	switch status {
	case 401, 403:
		return &AuthError{APIError: base}
	case 402:
		required, available := readCreditValues(details)
		return &InsufficientCreditsError{APIError: base, Available: available, Required: required}
	case 422:
		return &ValidationError{APIError: base}
	case 429:
		return &RateLimitError{APIError: base}
	default:
		return &base
	}
}

func readCreditValues(details any) (float64, float64) {
	values, ok := details.(map[string]any)
	if !ok {
		return 0, 0
	}
	return toFloat(values["required"]), toFloat(values["available"])
}

func wrapJobFailed(jobID string, requestID string, code string, message string) error {
	if code == "" {
		code = "job_failed"
	}
	if message == "" {
		message = fmt.Sprintf("job %s failed", jobID)
	}
	return &JobFailedError{ConduitError: newConduitError(message, code, requestID, nil), JobID: jobID}
}

func wrapJobCanceled(jobID string, requestID string) error {
	return &JobCanceledError{ConduitError: newConduitError(fmt.Sprintf("job %s canceled", jobID), "job_canceled", requestID, nil), JobID: jobID}
}
