package client

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// mapArrayToString joins values with commas, the format Nagios expects for
// list-valued URL parameters (e.g. contacts, templates).
func mapArrayToString(values []string) string {
	return strings.Join(values, ",")
}

// boolToNagiosString converts a Go bool to Nagios's "0"/"1" string
// convention. Callers that need to distinguish "unset" from "explicitly
// false" must check that before calling this - see internal/provider's
// model-to-client converters, which only call this for non-null values.
func boolToNagiosString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// setURLParams reflects over a client object struct (Host, Service, ...) and
// builds the url.Values body Nagios's API expects, using each field's `json`
// tag as the parameter name. Empty strings, nil/empty slices, zero ints, and
// nil maps are omitted so an unset optional field is never sent as an
// explicit empty/zero value that Nagios would treat as intentional.
//
// map[string]string fields (free_variables) are special: each key becomes
// its own top-level URL parameter (e.g. _SNMPCOMMUNITY=public), not a single
// nested parameter - that's the shape Nagios's custom-macro API expects.
func setURLParams(nagiosObject interface{}) *url.Values {
	values := reflect.ValueOf(nagiosObject)
	urlParams := &url.Values{}

	if values.Kind() == reflect.Ptr {
		values = values.Elem()
	}

	for i := 0; i < values.NumField(); i++ {
		field := values.Field(i)
		jsonTag := values.Type().Field(i).Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		tag := strings.SplitN(jsonTag, ",", 2)[0]

		switch v := field.Interface().(type) {
		case string:
			if v != "" {
				urlParams.Add(tag, v)
			}
		case []string:
			if len(v) > 0 {
				urlParams.Add(tag, mapArrayToString(v))
			}
		case int:
			if v != 0 {
				urlParams.Add(tag, strconv.Itoa(v))
			}
		case map[string]string:
			for k, val := range v {
				urlParams.Add(k, val)
			}
		}
	}

	return urlParams
}

// extractFreeVariables pulls Nagios's custom `_`-prefixed macros out of a raw
// JSON object. Nagios returns free variables as dynamic top-level keys on the
// object itself (e.g. {"host_name": "...", "_SNMPCOMMUNITY": "public"}), not
// nested under a "free_variables" key - so these can't be represented as
// fixed Go struct fields the way the rest of an object's attributes are, and
// must be picked out of the raw response separately from the typed unmarshal.
func extractFreeVariables(raw map[string]json.RawMessage) map[string]string {
	vars := map[string]string{}
	for k, v := range raw {
		if !strings.HasPrefix(k, "_") {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			continue
		}
		vars[k] = s
	}
	if len(vars) == 0 {
		return nil
	}
	return vars
}
