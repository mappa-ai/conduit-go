# Conduit Go SDK

Official Go SDK for the Conduit API.

## Install

```bash
go get github.com/mappa-ai/conduit-go
```

## Quickstart

```go
package main

import (
	"context"
	"log"

	conduit "github.com/mappa-ai/conduit-go"
)

func main() {
	client, err := conduit.New("sk_...")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	receipt, err := client.Reports.Create(context.Background(), conduit.CreateReportRequest{
		Source: conduit.SourceURL("https://storage.example.com/call.wav"),
		Output: conduit.ReportOutputRequest{Template: conduit.ReportTemplateGeneralReport},
		Target: conduit.TargetDominant(),
		Webhook: &conduit.Webhook{URL: "https://my-app.com/webhooks/conduit"},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("job accepted: %s", receipt.JobID)
	_ = receipt
}
```

Verify webhooks before parsing:

```go
func handleWebhook(client *conduit.Client, body []byte, headers http.Header, secret string) error {
	if err := client.Webhooks.VerifySignature(body, headers, secret, 5*time.Minute); err != nil {
		return err
	}

	event, err := client.Webhooks.ParseEvent(body)
	if err != nil {
		return err
	}

	if event.Type != "report.completed" {
		return nil
	}

	data := event.Data.(map[string]any)
	_, err = client.Reports.Get(context.Background(), data["reportId"].(string))
	return err
}
```

## Runtime notes

- Server-side Go only.
- `SourceBytes`, `SourceReader`, `SourceURL`, and `SourcePath` currently materialize uploads in memory before submission.
- `Handle.Wait` and `Handle.Stream` use polling, not SSE.

## Local checks

```bash
go test ./...
go vet ./...
```
