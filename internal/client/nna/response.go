package nna

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// errorBody mirrors the two distinct shapes NNA uses for a failed request:
// a Laravel validation failure (422) puts per-field messages under "errors"
// keyed by attribute name, while a request that fails deeper in the
// controller (e.g. the source_controller.py collector process crashing)
// reports a single top-level "error" string instead alongside "message" -
// confirmed live against /api/v1/sources. There's no single consistent
// field name the way XI always uses "success"/"error" - callers should
// prefer whichever of Error/Message is non-empty.
type errorBody struct {
	Message string              `json:"message"`
	Error   string              `json:"error"`
	Errors  map[string][]string `json:"errors"`
}

// APIError wraps a failed NNA API response, preserving the HTTP status code
// since (unlike XI) it's a meaningful signal callers branch on - a 404
// means "not found", a 422 means "validation failure", a 500 means the
// request was accepted far enough to run server-side logic that then
// failed (see source.go's NewSource for why that distinction matters).
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("nna: %s (HTTP %d)", e.Message, e.StatusCode)
}

// parseError builds an APIError from a non-2xx response body, preferring
// the top-level "error" string, falling back to "message", and appending
// field-level validation messages when present.
func parseError(statusCode int, body []byte) error {
	var b errorBody
	if err := json.Unmarshal(body, &b); err != nil {
		// Deliberately doesn't include the raw body: a non-JSON response is
		// most likely an HTML error/debug page (NNA is Laravel-based), and
		// Laravel's own debug page echoes the request - including the
		// Authorization header carrying this client's Bearer token - back
		// into the page when APP_DEBUG is on. Surfacing that verbatim into
		// a Terraform diagnostic would print it straight to CLI output, CI
		// logs, and TF_LOG traces.
		return &APIError{StatusCode: statusCode, Message: "non-JSON response body (likely an HTML error page)"}
	}

	msg := b.Error
	if msg == "" {
		msg = b.Message
	}

	if len(b.Errors) > 0 {
		fields := make([]string, 0, len(b.Errors))
		for field := range b.Errors {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		var sb strings.Builder
		sb.WriteString(msg)
		for _, field := range fields {
			for _, fe := range b.Errors[field] {
				sb.WriteString("; ")
				sb.WriteString(field)
				sb.WriteString(": ")
				sb.WriteString(fe)
			}
		}
		msg = sb.String()
	}

	return &APIError{StatusCode: statusCode, Message: msg}
}

func isSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}
