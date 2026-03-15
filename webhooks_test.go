package conduit_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	conduit "github.com/mappa-ai/conduit-go"
)

const reportCompletedPayload = "{" +
	`"id":"evt_1",` +
	`"type":"report.completed",` +
	`"createdAt":"2026-03-15T00:00:00Z",` +
	`"timestamp":"2026-03-15T00:00:00Z",` +
	`"data":{"jobId":"job_1","reportId":"rep_1","status":"succeeded"}` +
	"}"

const matchingCompletedPayload = "{" +
	`"id":"evt_1",` +
	`"type":"matching.completed",` +
	`"createdAt":"2026-03-15T00:00:00Z",` +
	`"timestamp":"2026-03-15T00:00:00Z",` +
	`"data":{"jobId":"job_1","matchingId":"mat_1","status":"succeeded"}` +
	"}"

const invalidReportCompletedPayload = "{" +
	`"id":"evt_1",` +
	`"type":"report.completed",` +
	`"createdAt":"2026-03-15T00:00:00Z",` +
	`"timestamp":"2026-03-15T00:00:00Z",` +
	`"data":{"jobId":"job_1","status":"succeeded"}` +
	"}"

func TestVerifySignatureAcceptsValidPayload(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	secret := "whsec_test"
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, reportCompletedPayload)))
	signature := hex.EncodeToString(mac.Sum(nil))
	headers := http.Header{"Conduit-Signature": []string{fmt.Sprintf("t=%d,v1=%s", timestamp, signature)}}

	if err := client.Webhooks.VerifySignature([]byte(reportCompletedPayload), headers, secret, 5*time.Minute); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

func TestVerifySignatureRejectsInvalidPayload(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	err = client.Webhooks.VerifySignature([]byte("{}"), http.Header{"Conduit-Signature": []string{"t=1,v1=deadbeef"}}, "whsec_test", time.Hour)
	if err == nil {
		t.Fatal("expected error")
	}
	var verificationErr *conduit.WebhookVerificationError
	if !errors.As(err, &verificationErr) {
		t.Fatalf("expected WebhookVerificationError, got %T", err)
	}
}

func TestParseEventValidatesKnownEventShape(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	event, err := client.Webhooks.ParseEvent([]byte(matchingCompletedPayload))
	if err != nil {
		t.Fatalf("parse event: %v", err)
	}
	if event.Type != "matching.completed" {
		t.Fatalf("expected matching.completed, got %s", event.Type)
	}
}

func TestParseEventRejectsInvalidKnownEventShape(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	_, err = client.Webhooks.ParseEvent([]byte(invalidReportCompletedPayload))
	if err == nil {
		t.Fatal("expected error")
	}
	var conduitErr *conduit.ConduitError
	if !errors.As(err, &conduitErr) {
		t.Fatalf("expected ConduitError, got %T", err)
	}
}
