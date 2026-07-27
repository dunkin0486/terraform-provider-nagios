// Package client is a plain Go HTTP client for the Nagios XI REST API. It has
// no dependency on terraform-plugin-framework so it can be built and unit
// tested standalone.
package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Nagios XI instance's REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient returns a Client configured to talk to the Nagios XI instance at
// baseURL (e.g. "http://localhost:8080/nagiosxi") using the given API token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do is the single HTTP transport choke point every verb helper below routes
// through: it sets the headers Nagios's API expects and reads the response
// body. It deliberately does not interpret the body - Nagios always answers
// HTTP 200 even on failure (see response.go's parseCommandResponse), so a
// nil error here only means "the HTTP round trip succeeded," not "the
// operation succeeded."
func (c *Client) do(ctx context.Context, method, nagiosURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, nagiosURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *Client) get(ctx context.Context, nagiosURL string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, nagiosURL, nil)
}

func (c *Client) post(ctx context.Context, nagiosURL string, data *url.Values) ([]byte, error) {
	body, err := c.do(ctx, http.MethodPost, nagiosURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	if err := parseCommandResponse(body); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) put(ctx context.Context, nagiosURL string) ([]byte, error) {
	nagiosURL = strings.ReplaceAll(nagiosURL, " ", "%20")

	body, err := c.do(ctx, http.MethodPut, nagiosURL, nil)
	if err != nil {
		return nil, err
	}
	if err := parseCommandResponse(body); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) delete(ctx context.Context, nagiosURL string, data *url.Values) ([]byte, error) {
	body, err := c.do(ctx, http.MethodDelete, nagiosURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	if err := parseCommandResponse(body); err != nil {
		return nil, err
	}
	return body, nil
}

// applyConfig pushes pending config changes (from a preceding create/update/
// delete) into Nagios's live running configuration. Every mutating client
// call in host.go/hostgroup.go/etc. must call this after its write succeeds -
// without it, the change lands in Nagios's DB but never takes effect.
func (c *Client) applyConfig(ctx context.Context) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "applyconfig", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	_, err = c.post(ctx, nagiosURL, &url.Values{})
	return err
}
