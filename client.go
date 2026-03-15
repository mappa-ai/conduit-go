package conduit

import (
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL      = "https://api.mappa.ai"
	defaultTimeout      = 30 * time.Second
	defaultPollInterval = 3 * time.Second
	defaultMaxRetries   = 2
)

const DefaultMaxSourceBytes int64 = 5 * 1024 * 1024 * 1024

type Option func(*clientConfig) error

type clientConfig struct {
	baseURL        string
	httpClient     *http.Client
	maxRetries     int
	maxSourceBytes int64
	timeout        time.Duration
	userAgent      string
}

type Client struct {
	transport  *transport
	Reports    *ReportsResource
	Matching   *MatchingResource
	Primitives *PrimitivesResource
	Webhooks   *WebhooksResource
}

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
	if _, err := parseHTTPURL(resolvedBaseURL, "baseUrl"); err != nil {
		return nil, &InitializationError{ConduitError: newConduitError("baseUrl must be a valid URL", "config_error", "", err)}
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
	jobs := &JobsResource{transport: transport, pollInterval: defaultPollInterval}
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

func (c *Client) Close() {
	if c == nil || c.transport == nil {
		return
	}
	c.transport.closeIdleConnections()
}

func WithBaseURL(value string) Option {
	return func(config *clientConfig) error {
		config.baseURL = value
		return nil
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(config *clientConfig) error {
		if client == nil {
			return &InitializationError{ConduitError: newConduitError("httpClient cannot be nil", "config_error", "", nil)}
		}
		config.httpClient = client
		return nil
	}
}

func WithMaxRetries(value int) Option {
	return func(config *clientConfig) error {
		config.maxRetries = value
		return nil
	}
}

func WithMaxSourceBytes(value int64) Option {
	return func(config *clientConfig) error {
		config.maxSourceBytes = value
		return nil
	}
}

func WithTimeout(value time.Duration) Option {
	return func(config *clientConfig) error {
		config.timeout = value
		return nil
	}
}

func WithUserAgent(value string) Option {
	return func(config *clientConfig) error {
		config.userAgent = strings.TrimSpace(value)
		return nil
	}
}
