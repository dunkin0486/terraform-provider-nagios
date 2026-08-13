package client

import (
	"errors"
	"net/http"
	"strings"
)

// buildURL generates the Nagios XI API URL for the given objectType (e.g.
// "host", "service", "authserver", "user", "applyconfig") and HTTP method.
//
//   - GET/DELETE filter results via a single "<objectName>=<name>" query
//     param, except DELETE on "authserver"/"user", which Nagios expects as a
//     "/<id>" path segment instead - a real inconsistency in the API, not
//     something normalized away here. GET's filter is skipped entirely when
//     objectName is empty, for "user"'s full-list fetch (see user.go's
//     GetUser - Nagios silently ignores its username= filter, so listing
//     must be unfiltered and scanned client-side).
//   - PUT addresses the *old* name/id as a path segment (rename-in-place)
//     plus force=1; for "service" it additionally appends
//     "/<objectDescription>" to the path, since services are addressed by
//     (name, description). The returned URL intentionally ends with a
//     trailing "&" so callers can append setURLParams(...).Encode() directly
//     (see host.go etc.).
//   - POST needs force=1 for every object type except "applyconfig" itself.
func buildURL(baseURL, token, objectType, method, objectName, name, oldVal, objectDescription string) (string, error) {
	var nagiosURL strings.Builder

	var apiType string
	switch objectType {
	case "applyconfig":
		apiType = "system"
		if method != http.MethodPost {
			return "", errors.New("you must use a HTTP POST when performing an applyconfig")
		}
	case "authserver", "user":
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

		if objectName != "" {
			if name == "" {
				return "", errors.New("name must be provided when using the " + method + " method")
			}
			nagiosURL.WriteString("&")
			nagiosURL.WriteString(objectName)
			nagiosURL.WriteString("=")
			nagiosURL.WriteString(name)
		}
		nagiosURL.WriteString("&pretty=1")

	case http.MethodDelete:
		if objectType == "authserver" || objectType == "user" {
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
