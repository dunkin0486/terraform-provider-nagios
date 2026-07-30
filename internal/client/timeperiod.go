package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Timeperiod mirrors the fields Nagios XI's timeperiod object API
// accepts/returns.
//
// Nagios has a real quirk here, confirmed against a live instance: PUT
// (update) on timeperiod is a complete no-op. It reports a fake
// {"success": "..."} whether renaming the object or changing any other
// field on it in place, but the object is left completely unchanged either
// way - and the response body itself has a stray PHP print_r() debug dump
// (e.g. "Array\n(\n    [type] => 9\n    [timeperiod_name] => oldname\n)\n")
// prepended before the JSON, so it fails JSON parsing entirely rather than
// unmarshaling into a misleading "success". UpdateTimeperiod still exists
// (resource.Resource requires an Update method), but resource_timeperiod.go
// marks every schema attribute RequiresReplace so it's unreachable in
// practice - the same shape as authserver's documented "no update route"
// quirk (#104), except authserver's PUT at least fails with a clean,
// parseable error ("Unknown API endpoint.") instead of unparseable output.
//
// Calendar-date exceptions (e.g. Nagios's "december 25" key) are
// intentionally not modeled: also confirmed against a live instance, the API
// returns these with odd fixed-width whitespace padding baked into the key
// itself (e.g. "december 25            "), and are out of scope here - only
// the standard weekday fields plus use/exclude are exposed.
//
// One more confirmed-against-a-live-instance quirk, specific to this object
// type: every other object type with a "use" (template inheritance) field -
// host, service, contact - returns it as a JSON array even for a single
// value (e.g. {"use": ["linux-server"]}). timeperiod's "use" never comes
// back as an array at all, for one value or many: it's always a plain
// comma-joined string (e.g. {"use": "24x7,workhours"}), unlike this same
// object's own "exclude" field, which does round-trip as an array. See
// Timeperiod's UnmarshalJSON below, which normalizes both possible shapes.
type Timeperiod struct {
	Name      string   `json:"timeperiod_name"`
	Alias     string   `json:"alias"`
	Templates []string `json:"use,omitempty"`
	Exclude   []string `json:"exclude,omitempty"`
	Sunday    string   `json:"sunday,omitempty"`
	Monday    string   `json:"monday,omitempty"`
	Tuesday   string   `json:"tuesday,omitempty"`
	Wednesday string   `json:"wednesday,omitempty"`
	Thursday  string   `json:"thursday,omitempty"`
	Friday    string   `json:"friday,omitempty"`
	Saturday  string   `json:"saturday,omitempty"`
}

// UnmarshalJSON normalizes the "use" field's inconsistent wire shape (see
// the Timeperiod doc comment above) into []string regardless of whether
// Nagios sent a JSON array or a comma-joined string. Every other field
// decodes via the struct's own json tags as usual; only "use" needs special
// handling. This only affects decoding a response body - setURLParams
// already comma-joins []string identically for every list-valued field on
// write, so encoding is unaffected.
func (tp *Timeperiod) UnmarshalJSON(data []byte) error {
	type timeperiodAlias Timeperiod
	aux := &struct {
		Use json.RawMessage `json:"use,omitempty"`
		*timeperiodAlias
	}{timeperiodAlias: (*timeperiodAlias)(tp)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Use) == 0 {
		return nil
	}

	var asArray []string
	if err := json.Unmarshal(aux.Use, &asArray); err == nil {
		tp.Templates = asArray
		return nil
	}

	var asString string
	if err := json.Unmarshal(aux.Use, &asString); err != nil {
		return err
	}
	if asString != "" {
		tp.Templates = strings.Split(asString, ",")
	}
	return nil
}

// NewTimeperiod creates a timeperiod and applies the config change.
func (c *Client) NewTimeperiod(ctx context.Context, tp *Timeperiod) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "timeperiod", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(tp)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetTimeperiod looks up a timeperiod by name. It returns (nil, nil) if none
// exists with that name.
func (c *Client) GetTimeperiod(ctx context.Context, name string) (*Timeperiod, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "timeperiod", http.MethodGet, "timeperiod_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var periods []Timeperiod
	if err := json.Unmarshal(body, &periods); err != nil {
		return nil, err
	}
	if len(periods) == 0 {
		return nil, nil
	}
	return &periods[0], nil
}

// UpdateTimeperiod addresses a timeperiod by oldName and attempts a PUT,
// falling back to creating it fresh if Nagios reports the old name no longer
// exists. In practice Nagios's PUT for timeperiod never succeeds (see the
// Timeperiod doc comment above), so this call almost always returns an
// error - resource_timeperiod.go's RequiresReplace schema is what actually
// keeps this reachable only in the fallback/defensive sense, matching the
// pattern used for authserver.
//
// The existsErrorFor fallback below is structural parity with every other
// UpdateX method, not a reachable path for timeperiod specifically: Nagios's
// PUT response for this object type never parses as JSON in the first place
// (see the doc comment above), so parseCommandResponse never gets far enough
// to produce the "Does the timeperiod exist?" string existsErrorFor looks
// for. The non-fallback error branch is wrapped below precisely because it's
// the one a caller will actually observe.
func (c *Client) UpdateTimeperiod(ctx context.Context, tp *Timeperiod, oldName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "timeperiod", http.MethodPut, "timeperiod_name", tp.Name, oldName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(tp).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("timeperiod", err) {
			return c.NewTimeperiod(ctx, tp)
		}
		return fmt.Errorf("nagios's timeperiod update API is a known, permanent no-op (see the Timeperiod doc comment) - this should be unreachable via Terraform since every schema attribute is RequiresReplace; seeing this error means that invariant has regressed: %w", err)
	}
	return c.applyConfig(ctx)
}

// DeleteTimeperiod deletes a timeperiod by name and applies the config
// change.
func (c *Client) DeleteTimeperiod(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "timeperiod", http.MethodDelete, "timeperiod_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("timeperiod_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
