package client

import (
	"errors"
	"net/http"
	"strings"
)

// buildURL generates the Nagios XI API URL for the given objectType (e.g.
// "host", "service", "authserver", "applyconfig") and HTTP method.
//
//   - GET/DELETE filter results via a single "<objectName>=<name>" query
//     param, except DELETE on "authserver", which Nagios expects as a
//     "/<name>" path segment instead - a real inconsistency in the API, not
//     something normalized away here.
//   - PUT addresses the *old* name as a path segment (rename-in-place) plus
//     force=1; for "service" it additionally appends "/<objectDescription>"
//     to the path, since services are addressed by (name, description).
//     The returned URL intentionally ends with a trailing "&" so callers can
//     append setURLParams(...).Encode() directly (see host.go etc.).
//   - POST needs force=1 for every object type except "applyconfig" itself.
func buildURL(baseURL, token, objectType, method, objectName, name, oldVal, objectDescription string) (string, error) {
	var nagiosURL strings.Builder

	var apiType string
	switch {
	case objectType == "applyconfig":
		apiType = "system"
		if method != http.MethodPost {
			return "", errors.New("you must use a HTTP POST when performing an applyconfig")
		}
	case objectType == "authserver":
		apiType = "system"
	default:
		apiType = "config"
	}

	apiURL := "api/v1/" + apiType + "/"
	if !strings.HasSuffix(baseURL, "/") {
		apiURL = "/" + apiURL
	}

	nagiosURL.WriteString(baseURL)
	nagiosURL.WriteString(apiURL)
	nagiosURL.WriteString(objectType)

	switch method {
	case http.MethodGet:
		nagiosURL.WriteString("?apikey=")
		nagiosURL.WriteString(token)
		nagiosURL.WriteString("&")
		nagiosURL.WriteString(objectName)
		nagiosURL.WriteString("=")

		if name == "" {
			return "", errors.New("name must be provided when using the " + method + " method")
		}
		nagiosURL.WriteString(name)
		nagiosURL.WriteString("&pretty=1")

	case http.MethodDelete:
		if objectType == "authserver" {
			nagiosURL.WriteString("/" + name)
		}
		nagiosURL.WriteString("?apikey=")
		nagiosURL.WriteString(token)
		nagiosURL.WriteString("&")
		nagiosURL.WriteString(objectName)
		nagiosURL.WriteString("=")

		if name == "" {
			return "", errors.New("name must be provided when using the " + method + " method")
		}
		nagiosURL.WriteString(name)

	case http.MethodPut:
		nagiosURL.WriteString("/")

		if oldVal == "" {
			return "", errors.New("a value for oldVal must be provided when attempting a PUT")
		}
		nagiosURL.WriteString(oldVal)

		if objectType == "service" {
			nagiosURL.WriteString("/" + objectDescription)
		}

		nagiosURL.WriteString("?apikey=")
		nagiosURL.WriteString(token)
		nagiosURL.WriteString("&pretty=1&force=1&")

	case http.MethodPost:
		nagiosURL.WriteString("?apikey=")
		nagiosURL.WriteString(token)

		if objectType != "applyconfig" {
			nagiosURL.WriteString("&force=1&")
		}
	}

	return nagiosURL.String(), nil
}
