package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Host mirrors the fields Nagios XI's host object API accepts/returns.
type Host struct {
	HostName                   string            `json:"host_name"`
	Address                    string            `json:"address"`
	DisplayName                string            `json:"display_name,omitempty"`
	MaxCheckAttempts           string            `json:"max_check_attempts"`
	CheckPeriod                string            `json:"check_period"`
	NotificationInterval       string            `json:"notification_interval"`
	NotificationPeriod         string            `json:"notification_period"`
	Contacts                   []string          `json:"contacts"`
	Alias                      string            `json:"alias,omitempty"`
	Templates                  []string          `json:"use,omitempty"`
	CheckCommand               string            `json:"check_command,omitempty"`
	ContactGroups              []string          `json:"contact_groups,omitempty"`
	Notes                      string            `json:"notes,omitempty"`
	NotesURL                   string            `json:"notes_url,omitempty"`
	ActionURL                  string            `json:"action_url,omitempty"`
	InitialState               string            `json:"initial_state,omitempty"`
	RetryInterval              string            `json:"retry_interval,omitempty"`
	PassiveChecksEnabled       string            `json:"passive_checks_enabled,omitempty"`
	ActiveChecksEnabled        string            `json:"active_checks_enabled,omitempty"`
	ObsessOverHost             string            `json:"obsess_over_host,omitempty"`
	EventHandler               string            `json:"event_handler,omitempty"`
	EventHandlerEnabled        string            `json:"event_handler_enabled,omitempty"`
	FlapDetectionEnabled       string            `json:"flap_detection_enabled,omitempty"`
	FlapDetectionOptions       []string          `json:"flap_detection_options,omitempty"`
	LowFlapThreshold           string            `json:"low_flap_threshold,omitempty"`
	HighFlapThreshold          string            `json:"high_flap_threshold,omitempty"`
	ProcessPerfData            string            `json:"process_perf_data,omitempty"`
	RetainStatusInformation    string            `json:"retain_status_information,omitempty"`
	RetainNonstatusInformation string            `json:"retain_nonstatus_information,omitempty"`
	CheckFreshness             string            `json:"check_freshness,omitempty"`
	FreshnessThreshold         string            `json:"freshness_threshold,omitempty"`
	FirstNotificationDelay     string            `json:"first_notification_delay,omitempty"`
	NotificationOptions        string            `json:"notification_options,omitempty"`
	NotificationsEnabled       string            `json:"notifications_enabled,omitempty"`
	StalkingOptions            string            `json:"stalking_options,omitempty"`
	IconImage                  string            `json:"icon_image,omitempty"`
	IconImageAlt               string            `json:"icon_image_alt,omitempty"`
	VRMLImage                  string            `json:"vrml_image,omitempty"`
	StatusMapImage             string            `json:"statusmap_image,omitempty"`
	TwoDCoords                 string            `json:"2d_coords,omitempty"`
	ThreeDCoords               string            `json:"3d_coords,omitempty"`
	Register                   string            `json:"register,omitempty"`
	FreeVariables              map[string]string `json:"free_variables,omitempty"`
}

// NewHost creates a host and applies the config change.
func (c *Client) NewHost(ctx context.Context, h *Host) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "host", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(h)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetHost looks up a host by name. It returns (nil, nil) if no host with
// that name exists - callers must not assume a non-nil error means not-found.
func (c *Client) GetHost(ctx context.Context, name string) (*Host, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "host", http.MethodGet, "host_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var hosts []Host
	if err := json.Unmarshal(body, &hosts); err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, nil
	}
	host := hosts[0]

	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		host.FreeVariables = extractFreeVariables(raw[0])
	}

	return &host, nil
}

// UpdateHost renames/updates a host addressed by oldHostName. If Nagios
// reports the old name no longer exists (e.g. manually deleted or already
// renamed outside Terraform), it falls back to creating the host fresh.
func (c *Client) UpdateHost(ctx context.Context, h *Host, oldHostName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "host", http.MethodPut, "host_name", h.HostName, oldHostName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(h).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("host", err) {
			return c.NewHost(ctx, h)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteHost deletes a host by name and applies the config change.
func (c *Client) DeleteHost(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "host", http.MethodDelete, "host_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("host_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
