package client

import (
	"encoding/json"
	"fmt"
	"strings"
)

// commandResponse mirrors Nagios XI's REST API convention: mutating requests
// (POST/PUT/DELETE) always return HTTP 200, even on failure. Success/failure
// is only knowable by parsing the JSON body for a "success" or "error" key.
type commandResponse struct {
	Success string `json:"success"`
	Error   string `json:"error"`
}

// APIError wraps an error message returned in a Nagios XI command response body.
type APIError struct {
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

// parseCommandResponse is the single choke point for Nagios XI's "always 200,
// check the body" behavior described above. Every mutating client call routes
// through this exactly once.
func parseCommandResponse(body []byte) error {
	var r commandResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("nagios: unparseable response body: %w", err)
	}
	if r.Success != "" {
		return nil
	}
	return &APIError{Message: r.Error}
}

// envelopeError checks a GET response body for a top-level "error" key
// before the caller trusts an envelope-shaped ({"records", "<type>":[...]})
// unmarshal as authoritative. Unlike a bare []X array response (see
// TestGetHost_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound: an
// {"error":"..."} body already fails json.Unmarshal into []X loudly), an
// envelope struct has no "error" field of its own, so encoding/json's
// default "ignore unknown fields" behavior lets {"error":"..."} decode
// silently into a zero-value envelope (Records==0, no entries) -
// indistinguishable from a genuine zero-result GET. GetAuthServer and
// GetUser call this before trusting their envelope unmarshal.
func envelopeError(body []byte) error {
	var probe struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		// Malformed JSON: let the caller's own envelope unmarshal surface
		// this failure with its usual error message.
		return nil
	}
	if probe.Error != "" {
		return &APIError{Message: probe.Error}
	}
	return nil
}

// existsErrorFor reports whether err is Nagios's "Does the X exist?" signal
// for the given object type name (e.g. "host", "service"). UpdateX methods
// use this to fall back to creating the object fresh when a PUT targets a
// name that's been renamed or manually deleted out from under Terraform.
func existsErrorFor(objectType string, err error) bool {
	return err != nil && strings.Contains(err.Error(), fmt.Sprintf("Does the %s exist?", objectType))
}
