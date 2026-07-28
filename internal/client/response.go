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

// existsErrorFor reports whether err is Nagios's "Does the X exist?" signal
// for the given object type name (e.g. "host", "service"). UpdateX methods
// use this to fall back to creating the object fresh when a PUT targets a
// name that's been renamed or manually deleted out from under Terraform.
func existsErrorFor(objectType string, err error) bool {
	return err != nil && strings.Contains(err.Error(), fmt.Sprintf("Does the %s exist?", objectType))
}
