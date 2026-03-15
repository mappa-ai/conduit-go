package conduit

import (
	"fmt"
	"time"
)

type ConduitError struct {
	Cause     error
	Code      string
	Message   string
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

type InitializationError struct{ ConduitError }

type UnsupportedRuntimeError struct{ ConduitError }

type WebhookVerificationError struct{ ConduitError }

type SourceError struct {
	ConduitError
	Status int
	URL    string
}

type InvalidSourceError struct{ SourceError }

type RemoteFetchError struct{ SourceError }

type RemoteFetchTimeoutError struct{ RemoteFetchError }

type RemoteFetchTooLargeError struct{ RemoteFetchError }

type ApiError struct {
	ConduitError
	Details any
	Status  int
}

type AuthError struct{ ApiError }

type ValidationError struct{ ApiError }

type RateLimitError struct {
	ApiError
	RetryAfter time.Duration
}

type InsufficientCreditsError struct {
	ApiError
	Available float64
	Required  float64
}

type JobFailedError struct {
	ConduitError
	JobID string
}

type JobCanceledError struct {
	ConduitError
	JobID string
}

type TimeoutError struct{ ConduitError }

type RequestAbortedError struct{ ConduitError }

type StreamError struct {
	ConduitError
	JobID       string
	LastEventID string
	RetryCount  int
}

func newConduitError(message string, code string, requestID string, cause error) ConduitError {
	return ConduitError{Cause: cause, Code: code, Message: message, RequestID: requestID}
}

func newAPIError(status int, requestID string, message string, code string, details any) error {
	base := ApiError{
		ConduitError: newConduitError(message, code, requestID, nil),
		Details:      details,
		Status:       status,
	}
	switch status {
	case 401, 403:
		return &AuthError{ApiError: base}
	case 402:
		required, available := readCreditValues(details)
		return &InsufficientCreditsError{ApiError: base, Available: available, Required: required}
	case 422:
		return &ValidationError{ApiError: base}
	case 429:
		return &RateLimitError{ApiError: base}
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
