// Package client is a plain Go HTTP client for the Nagios XI REST API. It has
// no dependency on terraform-plugin-framework so it can be built and unit
// tested standalone.
package client

import (
	"context"
	"errors"
	"fmt"
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
	// buildURL concatenates old names/descriptions into path segments via
	// raw string writes rather than url.PathEscape, so a value containing a
	// space (e.g. a service description like "CPU Load") would otherwise
	// produce a malformed URL. This is applied once, centrally, so every
	// verb gets it uniformly - do not re-add a per-method ReplaceAll, that's
	// exactly the "only 2 of 8 methods had the fix" bug this replaced.
	nagiosURL = strings.ReplaceAll(nagiosURL, " ", "%20")

	req, err := http.NewRequestWithContext(ctx, method, nagiosURL, body)
	if err != nil {
		return nil, sanitizeTransportError(nagiosURL, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, sanitizeTransportError(nagiosURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return io.ReadAll(resp.Body)
}

// sanitizeTransportError strips the request URL - and with it, the apikey
// token (and, since user.go, a plaintext password - every UpdateX PUT in
// this client sends its full parameter set via the URL's query string, not
// the request body, see url.go's buildURL) embedded in it - out of
// transport-level errors before they can reach a Terraform diagnostic.
// http.NewRequestWithContext and http.Client.Do both wrap failures in
// *url.Error, whose own Error() string includes the full request URL
// verbatim (Go's net/url.Error format is "<op> <url>: <err>"). That's the
// only leak path here, but it's a real one: any transient network failure
// would otherwise print the live API token - or, for a nagios_user
// create/update, the account's password - to CLI output, CI logs, and
// TF_LOG traces. Terraform's Sensitive:true on the relevant schema
// attributes does NOT protect freeform diagnostic strings like this - it
// only governs how tracked state/plan values are displayed.
func sanitizeTransportError(nagiosURL string, err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s %s: %w", urlErr.Op, redactSensitiveParams(nagiosURL), urlErr.Err)
	}
	return err
}

// redactSensitiveParams replaces the apikey and password query parameters'
// values with "REDACTED". apikey is present on every request; password only
// appears on a nagios_user create/update (see sanitizeTransportError).
func redactSensitiveParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<unparseable URL>"
	}
	q := u.Query()
	changed := false
	for _, key := range []string{"apikey", "password"} {
		if q.Has(key) {
			q.Set(key, "REDACTED")
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
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
	// Space-escaping is handled centrally in do() now.
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
