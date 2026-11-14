package nna

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
)

// NmapScheduledScan mirrors a Nagios Network Analyzer scheduled Nmap scan -
// a set of nmap flags plus a recurrence - as accepted/returned by
// /api/v1/nmap/scheduled-scans. Scans that have run produce results under
// /nmap/scans; this type is only the schedule that launches them, and is
// what an nmap-type alert check's object_id refers to.
//
// Everything below was confirmed against a live instance.
type NmapScheduledScan struct {
	ID int64 `json:"id,omitempty"`
	// NmapProfileID optionally links the scan to a reusable NmapProfile.
	// It's a real database foreign key: naming a profile that doesn't
	// exist fails with a raw SQLSTATE[23000] 500 rather than a clean
	// error, and deleting the profile afterwards nulls this out (ON DELETE
	// SET NULL). It has no omitempty on purpose - PUT is a partial update
	// (omitted fields are preserved), so a cleared reference only lands if
	// it is sent as an explicit null.
	NmapProfileID *int64 `json:"nmap_profile_id"`
	// Name is not unique: two scans with the same name are both created.
	Name string `json:"name"`
	// Parameters is the free-form nmap command line, target included. As
	// with NmapProfile, NNA requires a standalone "-e <interface>" flag on
	// create (422 otherwise) but does NOT check it on update, so the
	// provider re-states the rule as a plan-time validator. NNA's own UI
	// additionally insists on a scan target; the API does not, and neither
	// does this client.
	Parameters string `json:"parameters"`
	// ScheduleType is one of every, hourlyAt, dailyAt, weeklyOn, monthlyOn,
	// yearlyOn or cron. NNA does not validate it: an unrecognized value is
	// accepted, stored, and silently drops schedule_parameters to an empty
	// list - the scan would simply never be scheduled. The provider
	// restricts it to the values NNA's UI offers.
	ScheduleType string `json:"schedule_type"`
	// ScheduleParameters carries the per-type recurrence detail (x, minute,
	// time, weekday, day, month, cron). It is replaced wholesale by a PUT -
	// keys from the previous schedule type don't linger. NNA's UI also
	// writes "days" and "between" arrays; this client models string values
	// only, and a response carrying anything else fails to decode rather
	// than being silently dropped (see UnmarshalJSON), since the next
	// write would otherwise erase it.
	ScheduleParameters map[string]string `json:"schedule_parameters"`
	// Enabled must always be sent: a create that omits it produces a
	// DISABLED scan (confirmed live), the opposite of what the UI does.
	Enabled bool `json:"enabled"`
	// NdiffsEnabled turns on ndiff comparison between successive runs.
	// GET reports it as the JSON number 0/1 while create and PUT report a
	// bool, so UnmarshalJSON accepts either.
	NdiffsEnabled bool `json:"ndiffs_enabled"`

	// LastRunAt and TimesRan are written by NNA's scheduler - read-only.
	// Neither is sent (omitempty on the zero value).
	LastRunAt string `json:"last_run_at,omitempty"`
	TimesRan  int64  `json:"times_ran,omitempty"`
}

// UnmarshalJSON normalizes the three shapes that differ between verbs: the
// 0/1-or-bool flags, and schedule_parameters, which NNA (PHP) serializes as
// a JSON array rather than an object when it is empty.
func (s *NmapScheduledScan) UnmarshalJSON(data []byte) error {
	type scanAlias NmapScheduledScan
	var raw struct {
		scanAlias
		Enabled            flexBool        `json:"enabled"`
		NdiffsEnabled      flexBool        `json:"ndiffs_enabled"`
		ScheduleParameters json.RawMessage `json:"schedule_parameters"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = NmapScheduledScan(raw.scanAlias)
	s.Enabled = bool(raw.Enabled)
	s.NdiffsEnabled = bool(raw.NdiffsEnabled)

	params, err := decodeScheduleParameters(raw.ScheduleParameters)
	if err != nil {
		return fmt.Errorf("nna: nmap scheduled scan %d: %w", s.ID, err)
	}
	s.ScheduleParameters = params
	return nil
}

// flexBool decodes a JSON bool or a 0/1 number.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", "1":
		*b = true
	case "false", "0", "null":
		*b = false
	default:
		return fmt.Errorf("expected a boolean or 0/1, got %s", data)
	}
	return nil
}

func decodeScheduleParameters(raw json.RawMessage) (map[string]string, error) {
	params := map[string]string{}
	if len(raw) == 0 || string(raw) == "null" {
		return params, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		// An empty PHP array arrives as [] rather than {}.
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil && len(arr) == 0 {
			return params, nil
		}
		return nil, fmt.Errorf("unexpected schedule_parameters shape: %w", err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := obj[k]
		var str string
		if json.Unmarshal(v, &str) == nil {
			params[k] = str
			continue
		}
		var num json.Number
		if json.Unmarshal(v, &num) == nil {
			params[k] = num.String()
			continue
		}
		return nil, fmt.Errorf("schedule parameter %q has a value this provider cannot represent (%s); only string and number parameters are supported, so edit it in Network Analyzer's UI or remove it", k, v)
	}
	return params, nil
}

const nmapScheduledScansPath = "nmap/scheduled-scans"

// NewNmapScheduledScan creates a scheduled scan. NNA answers 201 with
// {"message", "nmap_scheduled_scan": {...}}, id included, but that object is
// partial (no times_ran/last_run_at, and ndiffs_enabled as a bool rather
// than GET's 0/1), so the id is taken from it and the scan is read back by
// GET for a consistent shape. The POST is never retried, so a retry cannot
// duplicate the create (CLAUDE.md quirk 10).
func (c *Client) NewNmapScheduledScan(ctx context.Context, s *NmapScheduledScan) (*NmapScheduledScan, error) {
	body, status, err := c.post(ctx, nmapScheduledScansPath, s)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	created, err := unwrapNmapScheduledScan(body)
	if err != nil {
		return nil, err
	}
	got, err := c.GetNmapScheduledScan(ctx, created.ID)
	if err != nil {
		return nil, err
	}
	if got == nil {
		return nil, fmt.Errorf("nna: nmap scheduled scan %d was created but could not be read back", created.ID)
	}
	return got, nil
}

// GetNmapScheduledScan returns (nil, nil) when none exists, per CLAUDE.md
// quirk 9 - NNA answers 404 {"message":"Resource not found for id: N"}.
func (c *Client) GetNmapScheduledScan(ctx context.Context, id int64) (*NmapScheduledScan, error) {
	body, status, err := c.get(ctx, idPath(nmapScheduledScansPath, id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var scan NmapScheduledScan
	if err := json.Unmarshal(body, &scan); err != nil {
		return nil, err
	}
	if scan.ID == 0 {
		return nil, errors.New("nna: success response was not an nmap scheduled scan object (no id)")
	}
	return &scan, nil
}

// UpdateNmapScheduledScan updates a scan by id. PUT is a partial update, but
// this client always sends the full object so the result never depends on
// what was there before. The response wraps the updated scan under
// "nmap_scheduled_scan" and, unlike create's, is complete.
func (c *Client) UpdateNmapScheduledScan(ctx context.Context, id int64, s *NmapScheduledScan) (*NmapScheduledScan, error) {
	body, status, err := c.put(ctx, idPath(nmapScheduledScansPath, id), s)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return unwrapNmapScheduledScan(body)
}

// DeleteNmapScheduledScan deletes a scan by id, treating 404 as success:
// deleting twice answers 404 {"message":"Resource not found for id: N"}
// (confirmed live), and a destroy shouldn't fail on an already-gone scan.
func (c *Client) DeleteNmapScheduledScan(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath(nmapScheduledScansPath, id))
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}

func unwrapNmapScheduledScan(body []byte) (*NmapScheduledScan, error) {
	var wrapper struct {
		Scan *NmapScheduledScan `json:"nmap_scheduled_scan"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		// Deliberately doesn't echo the body: see NewCheck for why.
		return nil, errors.New("nna: success response was not valid JSON")
	}
	if wrapper.Scan == nil || wrapper.Scan.ID == 0 {
		return nil, errors.New("nna: success response carried no \"nmap_scheduled_scan\" object with an id")
	}
	return wrapper.Scan, nil
}
