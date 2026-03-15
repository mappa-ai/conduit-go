package conduit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WebhooksResource verifies and parses webhook deliveries.
type WebhooksResource struct{}

// VerifySignature verifies the conduit-signature webhook header.
func (r *WebhooksResource) VerifySignature(payload []byte, headers http.Header, secret string, tolerance time.Duration) error {
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	values := headers.Values("Conduit-Signature")
	if len(values) == 0 {
		return &WebhookVerificationError{ConduitError: newConduitError("missing conduit-signature header", "webhook_signature_missing", "", nil)}
	}
	if len(values) != 1 {
		return &WebhookVerificationError{ConduitError: newConduitError("duplicate conduit-signature header", "webhook_signature_invalid", "", nil)}
	}
	timestamp, signature, err := parseSignatureHeader(values[0])
	if err != nil {
		return err
	}
	if delta := time.Since(time.Unix(timestamp, 0)); delta > tolerance || delta < -tolerance {
		return &WebhookVerificationError{ConduitError: newConduitError("signature timestamp outside tolerance", "webhook_signature_stale", "", nil)}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return &WebhookVerificationError{ConduitError: newConduitError("invalid signature", "webhook_signature_invalid", "", nil)}
	}
	return nil
}

// ParseEvent parses and validates a webhook payload.
func (r *WebhooksResource) ParseEvent(payload []byte) (*WebhookEvent, error) {
	event, err := parseWebhookEvent(payload)
	if err != nil {
		return nil, &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: invalid JSON", Cause: err}
	}
	if err := requireFields(parseISO8601(event.CreatedAt, "createdAt"), parseISO8601(event.Timestamp, "timestamp")); err != nil {
		return nil, err
	}
	switch event.Type {
	case "report.completed":
		if err := validateCompletedData(event.Data, "reportId"); err != nil {
			return nil, err
		}
	case "report.failed", "matching.failed":
		if err := validateFailedData(event.Data); err != nil {
			return nil, err
		}
	case "matching.completed":
		if err := validateCompletedData(event.Data, "matchingId"); err != nil {
			return nil, err
		}
	}
	return event, nil
}

func parseSignatureHeader(value string) (int64, string, error) {
	fields := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		key, rawValue, ok := strings.Cut(part, "=")
		if !ok {
			return 0, "", &WebhookVerificationError{ConduitError: newConduitError("malformed conduit-signature header", "webhook_signature_invalid", "", nil)}
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if key == "" || rawValue == "" || (key != "t" && key != "v1") {
			return 0, "", &WebhookVerificationError{ConduitError: newConduitError("malformed conduit-signature header", "webhook_signature_invalid", "", nil)}
		}
		if _, exists := fields[key]; exists {
			return 0, "", &WebhookVerificationError{ConduitError: newConduitError("malformed conduit-signature header", "webhook_signature_invalid", "", nil)}
		}
		fields[key] = rawValue
	}
	timestampValue, okTimestamp := fields["t"]
	signature, okSignature := fields["v1"]
	if !okTimestamp || !okSignature {
		return 0, "", &WebhookVerificationError{ConduitError: newConduitError("malformed conduit-signature header", "webhook_signature_invalid", "", nil)}
	}
	timestamp, err := strconv.ParseInt(timestampValue, 10, 64)
	if err != nil {
		return 0, "", &WebhookVerificationError{ConduitError: newConduitError("invalid signature timestamp", "webhook_signature_invalid", "", err)}
	}
	return timestamp, signature, nil
}

func validateCompletedData(data any, resourceKey string) error {
	value, ok := data.(map[string]any)
	if !ok {
		return &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: data must be an object"}
	}
	if _, err := requireNonEmpty(stringFromAny(value["jobId"]), "webhook.data.jobId"); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: err.Error()}
	}
	if _, err := requireNonEmpty(stringFromAny(value[resourceKey]), "webhook.data."+resourceKey); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: err.Error()}
	}
	if stringFromAny(value["status"]) != "succeeded" {
		return &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: status must be succeeded"}
	}
	return nil
}

func validateFailedData(data any) error {
	value, ok := data.(map[string]any)
	if !ok {
		return &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: data must be an object"}
	}
	if _, err := requireNonEmpty(stringFromAny(value["jobId"]), "webhook.data.jobId"); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: err.Error()}
	}
	if stringFromAny(value["status"]) != "failed" {
		return &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: status must be failed"}
	}
	errorValue, ok := value["error"].(map[string]any)
	if !ok {
		return &ConduitError{Code: "invalid_webhook_payload", Message: "invalid webhook payload: error must be an object"}
	}
	if _, err := requireNonEmpty(stringFromAny(errorValue["code"]), "webhook.data.error.code"); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: err.Error()}
	}
	if _, err := requireNonEmpty(stringFromAny(errorValue["message"]), "webhook.data.error.message"); err != nil {
		return &ConduitError{Code: "invalid_webhook_payload", Message: err.Error()}
	}
	return nil
}

func stringFromAny(value any) string {
	stringValue, _ := value.(string)
	return stringValue
}
