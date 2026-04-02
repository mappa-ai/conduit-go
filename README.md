# Conduit Go SDK

This repository is the official public mirror for the Conduit Go SDK. Releases and issue tracking live here. SDK development happens in Conduit's private upstream monorepo and is mirrored into this repository.

Behavioral intelligence for your app. Webhook-first, receipt-first, and built for long-running jobs.

## Install

```bash
go get github.com/mappa-ai/conduit-go
```

## Scope

Stable SDK scope is intentionally small:

- `client.Reports.Create(...)`
- `client.Reports.Get(...)`
- `client.Matching.Create(...)`
- `client.Matching.Get(...)`
- `client.Webhooks.VerifySignature(...)`
- `client.Webhooks.ParseEvent(...)`

Advanced stable primitives are available under `client.Primitives.Entities`, `client.Primitives.Media`, and `client.Primitives.Jobs`.

Conduit jobs are asynchronous and commonly take around 150 seconds. Use webhooks as the default completion path. `Handle.Wait(...)` and `Handle.Stream(...)` are fallback control paths for local development, scripts, and operator tooling.

## Quickstart

```go
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	conduit "github.com/mappa-ai/conduit-go"
)

func main() {
	client, err := conduit.New(os.Getenv("CONDUIT_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	receipt, err := client.Reports.Create(context.Background(), conduit.CreateReportRequest{
		IdempotencyKey: "signup-call-42",
		Output: conduit.ReportOutputRequest{
			Template: conduit.ReportTemplateGeneralReport,
		},
		Source: conduit.SourceURL("https://storage.example.com/call.wav"),
		Target: conduit.TargetDominant(),
		Webhook: &conduit.Webhook{
			URL: "https://my-app.com/webhooks/conduit",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("accepted report job %s with status %s", receipt.JobID, receipt.Status)
}

func handleConduitWebhook(client *conduit.Client, seen func(string) bool) http.HandlerFunc {
	secret := os.Getenv("CONDUIT_WEBHOOK_SECRET")

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		if err := client.Webhooks.VerifySignature(payload, r.Header, secret, 5*time.Minute); err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		event, err := client.Webhooks.ParseEvent(payload)
		if err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}

		if seen(event.ID) {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch event.Type {
		case "report.completed":
			data := event.Data.(map[string]any)
			reportID := data["reportId"].(string)
			report, err := client.Reports.Get(r.Context(), reportID)
			if err != nil {
				http.Error(w, "failed to fetch report", http.StatusBadGateway)
				return
			}
			_ = report // persist report
		case "report.failed":
			log.Printf("report failed: %+v", event.Data)
		}

		w.WriteHeader(http.StatusOK)
	}
}
```

`Reports.Create(...)` returns only after source resolution, upload, and job acceptance complete successfully.

## Source variants

The SDK supports all canonical report source forms:

- `conduit.SourceMediaID("med_...")`
- `conduit.SourceBytes("call.wav", audioBytes)`
- `conduit.SourceURL("https://example.com/call.wav")`
- `conduit.SourcePath("./call.wav")`
- `conduit.SourceReader("call.wav", reader)`

Use `Source.WithLabel(...)` to override the inferred media label.

Runtime and transport behavior:

- `SourceURL(...)` makes the SDK host runtime fetch the remote media and then upload it to Conduit.
- Remote fetch follows redirects up to 5 hops.
- The client timeout budget applies to remote fetch, upload, and API create unless you provide a narrower `context.Context` deadline.
- `SourcePath(...)`, `SourceBytes(...)`, `SourceReader(...)`, and `SourceURL(...)` may materialize upload payloads in memory before submission.
- `Reports.Create(...)` returns a receipt only after upload succeeds and the job is accepted.

## Matching

Matching follows the same receipt-first async model as reports.

```go
receipt, err := client.Matching.Create(context.Background(), conduit.CreateMatchingRequest{
	Context: conduit.MatchingContextBehavioralCompatibility,
	Target:  conduit.MatchingEntity("ent_candidate"),
	Group: []conduit.MatchingSubject{
		conduit.MatchingEntity("ent_manager"),
		conduit.MatchingMedia("med_panel", conduit.TargetDominant()),
	},
	Webhook: &conduit.Webhook{URL: "https://my-app.com/webhooks/conduit"},
})
if err != nil {
	log.Fatal(err)
}

log.Printf("accepted matching job %s", receipt.JobID)
```

## Streaming job events

`Handle.Stream(...)` opens the API SSE stream for a single job. The SDK reconnects with exponential backoff and resumes with `Last-Event-ID` when the server supports it.

```go
stream, err := receipt.Handle.Stream(context.Background(), &conduit.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
	event := stream.Event()
	log.Printf("%s status=%s stage=%s", event.Type, event.Job.Status, event.Stage)
}

if err := stream.Err(); err != nil {
	log.Fatal(err)
}
```

Use `StreamOptions.LastEventID` to resume a dropped stream explicitly.

## Waiting locally

`Handle.Wait(...)` is a local-dev convenience layered on top of the SSE stream. It is not the primary production path.

```go
report, err := receipt.Handle.Wait(context.Background(), &conduit.WaitOptions{
	OnEvent: func(event conduit.JobEvent) {
		log.Printf("event=%s status=%s", event.Type, event.Job.Status)
	},
	Timeout: 5 * time.Minute,
})
if err != nil {
	log.Fatal(err)
}

log.Println(report.Output.Markdown)
```

## Stable primitives

Use primitives for advanced workflows, not onboarding.

### Entities

- `client.Primitives.Entities.Get(ctx, entityID)` fetches one entity.
- `client.Primitives.Entities.List(ctx, limit, cursor)` uses cursor pagination. Default limit is `20` when `limit <= 0`.
- `client.Primitives.Entities.Update(ctx, request)` sets or clears the optional entity label.

### Media

- `client.Primitives.Media.Upload(ctx, request)` materializes the same upload-capable source shapes as `Reports.Create(...)`.
- `client.Primitives.Media.Get(ctx, mediaID)` fetches one uploaded media object.
- `client.Primitives.Media.List(ctx, limit, cursor, includeDeleted)` uses cursor pagination. Default limit is `20` when `limit <= 0`.
- `client.Primitives.Media.Delete(ctx, request)` deletes media.
- `client.Primitives.Media.SetRetentionLock(ctx, request)` toggles retention lock state.

### Jobs

- `client.Primitives.Jobs.Get(ctx, jobID)` fetches the canonical job shape.
- `client.Primitives.Jobs.Cancel(ctx, request)` requests cancellation for an in-flight job.

## Error handling

The SDK returns typed errors with stable `Code` values.

```go
report, err := receipt.Handle.Wait(context.Background(), &conduit.WaitOptions{Timeout: 5 * time.Minute})
if err != nil {
	var rateLimit *conduit.RateLimitError
	var remoteTimeout *conduit.RemoteFetchTimeoutError
	var streamErr *conduit.StreamError
	var conduitErr *conduit.ConduitError

	switch {
	case errors.As(err, &rateLimit):
		log.Printf("retry after %s", rateLimit.RetryAfter)
	case errors.As(err, &remoteTimeout):
		log.Printf("remote source timed out: %s", remoteTimeout.URL)
	case errors.As(err, &streamErr):
		log.Printf("stream failed after %d retries", streamErr.RetryCount)
	case errors.As(err, &conduitErr):
		log.Printf("conduit error code=%s requestID=%s", conduitErr.Code, conduitErr.RequestID)
	default:
		log.Printf("unexpected error: %v", err)
	}
	return
}

log.Println(report.ID)
```

## Runtime matrix

| Runtime | `SourceBytes` | `SourceURL` | `SourcePath` | `Handle.Wait()` | `Handle.Stream()` | `Webhooks.VerifySignature()` |
| --- | --- | --- | --- | --- | --- | --- |
| Go server runtime | Yes | Yes | Yes | Yes | Yes | Yes |

## Support

- API docs: [docs.conduit.mappa.ai](https://docs.conduit.mappa.ai)
- SDK issues: use the issue tracker in this repository

## Local checks

```bash
go test ./...
go test -race ./...
go vet ./...
```
