// Package bedrock implements the model provider for Amazon Bedrock.
//
// It talks to the Bedrock runtime Converse API, which offers a single request
// format (including tool use) for every model hosted on Bedrock: Anthropic
// Claude, Amazon Nova, Meta Llama, Mistral and others.
//
//	provider := bedrock.NewProvider("eu-north-1")
//	provider.SetDefaultModel("anthropic.claude-3-5-sonnet-20241022-v2:0")
//
// Credentials are read from the standard AWS environment variables
// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN) unless they are
// set explicitly with WithCredentials.
package bedrock

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pontus-devoteam/agent-sdk-go/pkg/model"
)

const (
	// DefaultRegion is used when no region is configured.
	DefaultRegion = "us-east-1"

	// ServiceName is the AWS service name used when signing requests.
	ServiceName = "bedrock"

	// DefaultMaxRetries is the number of retries for throttled requests.
	DefaultMaxRetries = 5

	// DefaultRetryAfter is the base delay for the retry backoff.
	DefaultRetryAfter = 1 * time.Second

	// DefaultTimeout is the HTTP client timeout.
	DefaultTimeout = 120 * time.Second
)

// Provider implements model.Provider for Amazon Bedrock.
type Provider struct {
	// Region is the AWS region of the Bedrock runtime endpoint.
	Region string

	// DefaultModel is the model id used when no model is requested by name.
	DefaultModel string

	// HTTPClient performs the requests.
	HTTPClient *http.Client

	// MaxRetries is the maximum number of retries for throttled requests.
	MaxRetries int

	// RetryAfter is the base delay between retries.
	RetryAfter time.Duration

	// Internal state
	credentials Credentials
	baseURL     string
	mu          sync.RWMutex
}

// NewProvider creates a Bedrock provider for the given region. When region is
// empty, AWS_REGION, AWS_DEFAULT_REGION and finally DefaultRegion are used.
func NewProvider(region string) *Provider {
	if region == "" {
		region = firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"), DefaultRegion)
	}

	return &Provider{
		Region:     region,
		HTTPClient: &http.Client{Timeout: DefaultTimeout},
		MaxRetries: DefaultMaxRetries,
		RetryAfter: DefaultRetryAfter,
		credentials: Credentials{
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		},
	}
}

// WithCredentials sets static credentials, overriding the environment.
func (p *Provider) WithCredentials(accessKeyID, secretAccessKey, sessionToken string) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.credentials = Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}
	return p
}

// WithRegion sets the AWS region.
func (p *Provider) WithRegion(region string) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Region = region
	return p
}

// WithDefaultModel sets the default model id.
func (p *Provider) WithDefaultModel(modelID string) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.DefaultModel = modelID
	return p
}

// SetDefaultModel sets the default model id. Alias for WithDefaultModel.
func (p *Provider) SetDefaultModel(modelID string) *Provider {
	return p.WithDefaultModel(modelID)
}

// WithHTTPClient sets the HTTP client used for requests.
func (p *Provider) WithHTTPClient(client *http.Client) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.HTTPClient = client
	return p
}

// WithRetryConfig configures retries for throttled requests.
func (p *Provider) WithRetryConfig(maxRetries int, retryAfter time.Duration) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MaxRetries = maxRetries
	p.RetryAfter = retryAfter
	return p
}

// SetBaseURL overrides the Bedrock runtime endpoint. Mainly useful for tests
// and for VPC endpoints.
func (p *Provider) SetBaseURL(baseURL string) *Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseURL = baseURL
	return p
}

// GetBaseURL returns the endpoint requests are sent to.
func (p *Provider) GetBaseURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.baseURLLocked()
}

// baseURLLocked returns the endpoint. The caller must hold the lock.
func (p *Provider) baseURLLocked() string {
	if p.baseURL != "" {
		return p.baseURL
	}
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", p.Region)
}

// GetCredentials returns the credentials used for signing.
func (p *Provider) GetCredentials() Credentials {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.credentials
}

// GetModel returns a model by id.
func (p *Provider) GetModel(modelID string) (model.Model, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if modelID == "" {
		if p.DefaultModel == "" {
			return nil, fmt.Errorf("no model id provided and no default model set")
		}
		modelID = p.DefaultModel
	}

	if p.credentials.AccessKeyID == "" || p.credentials.SecretAccessKey == "" {
		return nil, fmt.Errorf("no AWS credentials found: set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY or call WithCredentials")
	}

	return &Model{ModelID: modelID, Provider: p}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
