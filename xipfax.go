package xipfax

import (
	"errors"
	"net/http"
	"strings"
)

// ErrNoAPIKey is what New answers when it is given no API key.
var ErrNoAPIKey = errors.New("xipfax: no API key: make one in the developer console, https://developer.xipfax.com/console/")

// An Option sets what New takes beside your API key.
type Option func(*Configuration)

// WithBaseURL names where the API answers: https://api.xipfax.com unless you name another.
func WithBaseURL(baseURL string) Option {
	return func(cfg *Configuration) {
		cfg.Servers = ServerConfigurations{{URL: strings.TrimSuffix(baseURL, "/")}}
	}
}

// WithHTTPClient names the client each request is sent with: http.DefaultClient unless you give
// one.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(cfg *Configuration) {
		cfg.HTTPClient = httpClient
	}
}

// New makes a client of XipFax's API from your API key, xipfax_key_live_ and 32 more characters,
// which you make in the developer console and keep on your server. Each request it sends carries
// the key as a bearer token and names APIVersion in its Xip-Version header. A key that is empty or
// only spaces is refused with ErrNoAPIKey.
func New(apiKey string, options ...Option) (*APIClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNoAPIKey
	}
	cfg := NewConfiguration()
	cfg.AddDefaultHeader("Authorization", "Bearer "+apiKey)
	cfg.AddDefaultHeader("Xip-Version", APIVersion)
	for _, option := range options {
		option(cfg)
	}
	return NewAPIClient(cfg), nil
}
