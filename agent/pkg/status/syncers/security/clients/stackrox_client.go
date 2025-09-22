package clients

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// StackRoxClientBuilder contains the data and logic needed to create a client for the StackRox API. Don't create
// instances of this type directly, use the NewStackRoxClient function instead.
type StackRoxClientBuilder struct {
	logger   *zap.SugaredLogger
	url      string
	token    string
	ca       *x509.CertPool
	wrappers []func(http.RoundTripper) http.RoundTripper
}

// StackRoxClient simplifies access to the StackRox API. Don't create instances of this type directly, use the
// NewStackRoxClient function instead.
type StackRoxClient struct {
	logger *zap.SugaredLogger
	url    string
	token  string
	client *http.Client
}

// NewStackRoxClient creates a builder that can then be used to configure and create a client for the StackRox API.
func NewStackRoxClient() *StackRoxClientBuilder {
	return &StackRoxClientBuilder{}
}

// SetLogger sets the logger that will be used by the client. This is mandatory.
func (b *StackRoxClientBuilder) SetLogger(value *zap.SugaredLogger) *StackRoxClientBuilder {
	b.logger = value
	return b
}

// SetURL sets the base URL of the StackRox API server. This is mandatory.
func (b *StackRoxClientBuilder) SetURL(value string) *StackRoxClientBuilder {
	b.url = value
	return b
}

// SetToken sets the token that will be used to authenticate to the StackRox API server. This is mandatory.
func (b *StackRoxClientBuilder) SetToken(value string) *StackRoxClientBuilder {
	b.token = value
	return b
}

// SetCA sets the certificate authorities that are trusted when connecting to the StackRox API server.
func (b *StackRoxClientBuilder) SetCA(value *x509.CertPool) *StackRoxClientBuilder {
	b.ca = value
	return b
}

// AddWrapper adds a function that wraps the HTTP client. This is intended to add things like logging, caching, or
// any other behaviour that can be implemented in such a wrapper.
func (b *StackRoxClientBuilder) AddWrapper(value func(http.RoundTripper) http.RoundTripper) *StackRoxClientBuilder {
	b.wrappers = append(b.wrappers, value)
	return b
}

// Build uses the information stored in the builder to create and configure a new StackRox API client.
func (b *StackRoxClientBuilder) Build() (result *StackRoxClient, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return result, err
	}
	if b.url == "" {
		err = errors.New("URL is mandatory")
		return result, err
	}
	if b.token == "" {
		err = errors.New("token is mandatory")
		return result, err
	}

	// Create the HTTP client:
	var transport http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    b.ca,
		},
	}
	for _, wrapper := range b.wrappers {
		transport = wrapper(transport)
	}
	client := &http.Client{
		Transport: transport,
	}

	// Create and populate the object:
	result = &StackRoxClient{
		logger: b.logger,
		url:    b.url,
		token:  b.token,
		client: client,
	}
	return result, err
}

// DoRequests send an HTTP request with the given method, path and request body. It returns the response body and a
// pointer to the status code. Both the response body and the pointer to the status code will be nil if there is some
// error while sending the request or receiving the response. Responses with error status codes, like 401 or 403, aren't
// considered errors.
func (c *StackRoxClient) DoRequest(method, path string, body string) ([]byte, *int, error) {
	endpoint := fmt.Sprintf("%s%s", c.url, path)
	req, err := http.NewRequest(method, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Add("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make request (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response (method: %s, path: %s, body: %s): %v", method, path, body, err)
	}

	return respBody, &resp.StatusCode, nil
}
