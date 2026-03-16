package conduit

import (
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL       = "https://api.mappa.ai"
	defaultStreamTimeout = 5 * time.Minute
	defaultTimeout       = 30 * time.Second
	defaultMaxRetries    = 2
	defaultUserAgent     = "conduit-go/0.1.8"
)

// DefaultMaxSourceBytes is the default maximum size for local and remote sources.
const DefaultMaxSourceBytes int64 = 5 * 1024 * 1024 * 1024

// Option configures a Client.
type Option func(*clientConfig) error

// RequestTelemetry describes an outgoing SDK request.
type RequestTelemetry struct {
	// Method is the HTTP method sent to the API.
	Method string
	// RequestID is the client-side request identifier attached to the request.
	RequestID string
	// URL is the fully resolved request URL.
	URL string
}

// ResponseTelemetry describes a completed SDK request.
type ResponseTelemetry struct {
	// Duration is the total request duration observed by the SDK.
	Duration time.Duration
	// Method is the HTTP method sent to the API.
	Method string
	// RequestID is the client-side request identifier attached to the request.
	RequestID string
	// Status is the HTTP status code returned by the API.
	Status int
	// URL is the fully resolved request URL.
	URL string
}

// ErrorTelemetry describes a failed SDK request.
type ErrorTelemetry struct {
	// Duration is the elapsed time before the request failed.
	Duration time.Duration
	// Error is the request failure observed by the SDK.
	Error error
	// Method is the HTTP method sent to the API.
	Method string
	// RequestID is the client-side request identifier attached to the request.
	RequestID string
	// URL is the fully resolved request URL.
	URL string
}

// Telemetry exposes request lifecycle hooks.
type Telemetry struct {
	// OnError runs after a request or SSE attempt fails.
	OnError func(ErrorTelemetry)
	// OnRequest runs before a request or SSE attempt is sent.
	OnRequest func(RequestTelemetry)
	// OnResponse runs after an HTTP response is received.
	OnResponse func(ResponseTelemetry)
}

type clientConfig struct {
	baseURL        string
	httpClient     *http.Client
	maxRetries     int
	maxSourceBytes int64
	telemetry      *Telemetry
	timeout        time.Duration
	userAgent      string
}

// Client is the root Conduit SDK client.
type Client struct {
	transport *transport
	// Reports exposes report generation workflows.
	Reports *ReportsResource
	// Matching exposes matching workflows.
	Matching *MatchingResource
	// Primitives exposes the stable low-level surface for advanced integrations.
	Primitives *PrimitivesResource
	// Webhooks exposes webhook verification and parsing helpers.
	Webhooks *WebhooksResource
}

// New creates a new Conduit client.
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, &InitializationError{ConduitError: newConduitError("apiKey is required", "config_error", "", nil)}
	}

	config := clientConfig{
		baseURL:        defaultBaseURL,
		maxRetries:     defaultMaxRetries,
		maxSourceBytes: DefaultMaxSourceBytes,
		timeout:        defaultTimeout,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	resolvedBaseURL := strings.TrimSpace(config.baseURL)
	if _, err := parseHTTPURL(resolvedBaseURL, "baseURL"); err != nil {
		return nil, &InitializationError{ConduitError: newConduitError("baseURL must be a valid URL", "config_error", "", err)}
	}
	if config.timeout <= 0 {
		return nil, &InitializationError{ConduitError: newConduitError("timeout must be greater than zero", "config_error", "", nil)}
	}
	if config.maxRetries < 0 {
		return nil, &InitializationError{ConduitError: newConduitError("maxRetries must be zero or greater", "config_error", "", nil)}
	}
	if config.maxSourceBytes <= 0 {
		return nil, &InitializationError{ConduitError: newConduitError("maxSourceBytes must be greater than zero", "config_error", "", nil)}
	}

	transport := newTransport(apiKey, config)
	jobs := &JobsResource{transport: transport}
	media := &MediaResource{transport: transport, timeout: config.timeout, maxSourceBytes: config.maxSourceBytes}
	entities := &EntitiesResource{transport: transport}
	client := &Client{
		transport: transport,
		Reports:   &ReportsResource{transport: transport, jobs: jobs, media: media},
		Matching:  &MatchingResource{transport: transport, jobs: jobs},
		Primitives: &PrimitivesResource{
			Entities: entities,
			Media:    media,
			Jobs:     jobs,
		},
		Webhooks: &WebhooksResource{},
	}
	return client, nil
}

// Close releases idle HTTP connections owned by the client.
func (c *Client) Close() {
	if c == nil || c.transport == nil {
		return
	}
	c.transport.closeIdleConnections()
}

// WithBaseURL sets the API base URL.
func WithBaseURL(value string) Option {
	return func(config *clientConfig) error {
		config.baseURL = value
		return nil
	}
}

// WithHTTPClient sets the HTTP client used for API and source URL requests.
func WithHTTPClient(client *http.Client) Option {
	return func(config *clientConfig) error {
		if client == nil {
			return &InitializationError{ConduitError: newConduitError("httpClient cannot be nil", "config_error", "", nil)}
		}
		config.httpClient = client
		return nil
	}
}

// WithMaxRetries sets the retry budget for transient failures.
func WithMaxRetries(value int) Option {
	return func(config *clientConfig) error {
		config.maxRetries = value
		return nil
	}
}

// WithMaxSourceBytes sets the maximum accepted source size.
func WithMaxSourceBytes(value int64) Option {
	return func(config *clientConfig) error {
		config.maxSourceBytes = value
		return nil
	}
}

// WithTelemetry configures request lifecycle hooks.
func WithTelemetry(telemetry *Telemetry) Option {
	return func(config *clientConfig) error {
		config.telemetry = telemetry
		return nil
	}
}

// WithTimeout sets the per-request timeout.
func WithTimeout(value time.Duration) Option {
	return func(config *clientConfig) error {
		config.timeout = value
		return nil
	}
}

// WithUserAgent appends a custom user agent token.
func WithUserAgent(value string) Option {
	return func(config *clientConfig) error {
		config.userAgent = strings.TrimSpace(value)
		return nil
	}
}
