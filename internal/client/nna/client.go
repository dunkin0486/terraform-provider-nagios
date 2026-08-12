// Package nna is a plain Go HTTP client for Nagios Network Analyzer's REST
// API. It has no dependency on terraform-plugin-framework so it can be built
// and unit tested standalone - mirroring internal/client's separation for
// Nagios XI, but kept in its own package because NNA's API is a genuinely
// different shape, not just a different base URL:
//
//   - Auth is an "Authorization: Bearer <token>" header (Laravel Sanctum),
//     not XI's "?apikey=" query parameter.
//   - Requests/responses are real JSON bodies, not form-urlencoded params.
//   - NNA returns real, meaningful HTTP status codes (404, 422, 500) - it
//     does not follow XI's "always 200, parse the body" convention.
//   - Objects are addressed by a numeric "id" path segment, not by name.
//   - There is no applyconfig-equivalent step: writes take effect
//     immediately.
package nna

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client talks to a Nagios Network Analyzer instance's REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient returns a Client configured to talk to the Nagios Network
// Analyzer instance at baseURL (e.g. "http://localhost:8081") using the
// given Sanctum API token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do is the single HTTP transport choke point every verb helper routes
// through: it sets the headers NNA's API expects, JSON-encodes body (if
// non-nil), and returns the raw response body alongside the HTTP status
// code. Unlike internal/client's XI transport, the status code is
// meaningful here and callers are expected to branch on it.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/"+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return respBody, resp.StatusCode, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, int, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, int, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *Client) put(ctx context.Context, path string, body any) ([]byte, int, error) {
	return c.do(ctx, http.MethodPut, path, body)
}

func (c *Client) delete(ctx context.Context, path string) ([]byte, int, error) {
	return c.do(ctx, http.MethodDelete, path, nil)
}

func idPath(base string, id int64) string {
	return base + "/" + strconv.FormatInt(id, 10)
}
