package conduit_test

import (
	"context"
	"errors"
	"testing"

	conduit "github.com/mappa-ai/conduit-go"
)

func TestReportsCreateRejectsZeroValueSource(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	_, err = client.Reports.Create(context.Background(), conduit.CreateReportRequest{
		Source: conduit.Source{},
		Output: conduit.ReportOutputRequest{Template: conduit.ReportTemplateGeneralReport},
		Target: conduit.TargetDominant(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var sourceErr *conduit.InvalidSourceError
	if !errors.As(err, &sourceErr) {
		t.Fatalf("expected InvalidSourceError, got %T", err)
	}
}

func TestMatchingCreateRejectsInvalidContext(t *testing.T) {
	client, err := conduit.New("sk_test", conduit.WithBaseURL("http://localhost:8080"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	_, err = client.Matching.Create(context.Background(), conduit.CreateMatchingRequest{
		Context: "freeform",
		Target:  conduit.MatchingEntity("ent_target"),
		Group:   []conduit.MatchingSubject{conduit.MatchingEntity("ent_group")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var conduitErr *conduit.ConduitError
	if !errors.As(err, &conduitErr) {
		t.Fatalf("expected ConduitError, got %T", err)
	}
}
