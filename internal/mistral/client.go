// Package mistral is a small upstream client for the Mistral API.
package mistral

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrMissingKey means no upstream credential is configured.
var ErrMissingKey = errors.New("Mistral API key is not configured")

// Client sends requests to one Mistral API origin. The credential is selected
// per request from the key pool and attached to the request context.
type Client struct {
	baseURL   string
	userAgent string
	http      *http.Client
}

// New builds a client with connection reuse and streaming-friendly timeouts.
// There is no overall request timeout: a completion may stream for minutes,
// and cancellation travels through the request context.
func New(baseURL, userAgent string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 300 * time.Second

	return &Client{
		baseURL:   baseURL,
		userAgent: userAgent,
		http:      &http.Client{Transport: transport},
	}
}

// ChatCompletions forwards one encoded chat/completions request, carrying the
// caller's headers (affinity, client metadata) the way the Vibe CLI does.
func (c *Client) ChatCompletions(ctx context.Context, body []byte, stream bool, inbound http.Header) (*http.Response, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	if stream {
		headers["Accept"] = "text/event-stream"
	} else {
		headers["Accept"] = "application/json"
	}
	// Identity keeps SSE bytes uncompressed and lets the proxy rewrite JSON.
	headers["Accept-Encoding"] = "identity"
	return c.do(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body), inbound, headers)
}

// Models lists the upstream models.
func (c *Client) Models(ctx context.Context, inbound http.Header) (*http.Response, error) {
	return c.get(ctx, "/v1/models", inbound)
}

// Model fetches one upstream model by id.
func (c *Client) Model(ctx context.Context, id string, inbound http.Header) (*http.Response, error) {
	return c.get(ctx, "/v1/models/"+url.PathEscape(id), inbound)
}

func (c *Client) get(ctx context.Context, path string, inbound http.Header) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, nil, inbound, map[string]string{
		"Accept":          "application/json",
		"Accept-Encoding": "identity",
	})
}

// ValidateKey checks a candidate key with a models request before it is saved.
func (c *Client) ValidateKey(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("validate key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("Mistral rejected the API key")
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Mistral returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, inbound http.Header, overrides map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	copyForwardable(req.Header, inbound)
	for key, value := range overrides {
		req.Header.Set(key, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	key := apiKeyFromContext(ctx)
	if key == "" {
		return nil, ErrMissingKey
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call upstream: %w", err)
	}
	return resp, nil
}

// hopByHop and per-connection headers that must not cross the proxy.
var blockedHeaders = map[string]bool{
	"authorization":       true,
	"connection":          true,
	"content-length":      true,
	"host":                true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"proxy-connection":    true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// copyForwardable carries the caller's end-to-end headers upstream. The
// Authorization value is replaced by the client, and encoding is chosen by
// the caller of do.
func copyForwardable(dst, src http.Header) {
	for name, values := range src {
		lower := strings.ToLower(name)
		if blockedHeaders[lower] || lower == "accept-encoding" {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}
