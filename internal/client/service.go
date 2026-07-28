package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Service mirrors the fields Nagios XI's service object API accepts/returns.
//
// Nagios's service API has a real, verb-specific compound-key inconsistency:
// GET/PUT key off (ServiceName, Description) - see GetService/UpdateService -
// while DELETE keys off (host_name, Description) instead - see DeleteService,
// which takes a comma-joined host name list rather than ServiceName at all.
// This isn't normalized away here; it's what the live API actually expects.
type Service struct {
	ServiceName                string            `json:"config_name"`
	HostName                   []string          `json:"host_name"`
	DisplayName                string            `json:"display_name,omitempty"`
	Description                string            `json:"service_description"`
	CheckCommand               string            `json:"check_command"`
	MaxCheckAttempts           string            `json:"max_check_attempts"`
	CheckInterval              string            `json:"check_interval"`
	RetryInterval              string            `json:"retry_interval"`
	CheckPeriod                string            `json:"check_period"`
	NotificationInterval       string            `json:"notification_interval"`
	NotificationPeriod         string            `json:"notification_period"`
	Contacts                   []string          `json:"contacts"`
	Templates                  []string          `json:"use,omitempty"`
	IsVolatile                 string            `json:"is_volatile,omitempty"`
	InitialState               string            `json:"initial_state,omitempty"`
	ActiveChecksEnabled        string            `json:"active_checks_enabled,omitempty"`
	PassiveChecksEnabled       string            `json:"passive_checks_enabled,omitempty"`
	ObsessOverService          string            `json:"obsess_over_service,omitempty"`
	CheckFreshness             string            `json:"check_freshness,omitempty"`
	FreshnessThreshold         string            `json:"freshness_threshold,omitempty"`
	EventHandler               string            `json:"event_handler,omitempty"`
	EventHandlerEnabled        string            `json:"event_handler_enabled,omitempty"`
	LowFlapThreshold           string            `json:"low_flap_threshold,omitempty"`
	HighFlapThreshold          string            `json:"high_flap_threshold,omitempty"`
	FlapDetectionEnabled       string            `json:"flap_detection_enabled,omitempty"`
	FlapDetectionOptions       []string          `json:"flap_detection_options,omitempty"`
	ProcessPerfData            string            `json:"process_perf_data,omitempty"`
	RetainStatusInformation    string            `json:"retain_status_information,omitempty"`
	RetainNonStatusInformation string            `json:"retain_nonstatus_information,omitempty"`
	FirstNotificationDelay     string            `json:"first_notification_delay,omitempty"`
	NotificationOptions        []string          `json:"notification_options,omitempty"`
	NotificationsEnabled       string            `json:"notifications_enabled,omitempty"`
	ContactGroups              []string          `json:"contact_groups,omitempty"`
	Notes                      string            `json:"notes,omitempty"`
	NotesURL                   string            `json:"notes_url,omitempty"`
	ActionURL                  string            `json:"action_url,omitempty"`
	IconImage                  string            `json:"icon_image,omitempty"`
	IconImageAlt               string            `json:"icon_image_alt,omitempty"`
	Register                   string            `json:"register,omitempty"`
	FreeVariables              map[string]string `json:"free_variables,omitempty"`
}

// NewService creates a service and applies the config change.
func (c *Client) NewService(ctx context.Context, s *Service) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "service", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(s)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetService looks up a service by its config name (ServiceName). It
// returns (nil, nil) if none exists with that name.
func (c *Client) GetService(ctx context.Context, serviceName string) (*Service, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "service", http.MethodGet, "config_name", serviceName, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var services []Service
	if err := json.Unmarshal(body, &services); err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, nil
	}
	service := services[0]

	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		service.FreeVariables = extractFreeVariables(raw[0])
	}

	return &service, nil
}

// UpdateService renames/updates a service addressed by (oldServiceName,
// oldDescription) - the PUT path is keyed by (config_name, description), not
// by host. Falls back to creating the service fresh if Nagios reports the
// old identity no longer exists.
func (c *Client) UpdateService(ctx context.Context, s *Service, oldServiceName, oldDescription string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "service", http.MethodPut, "config_name", s.ServiceName, oldServiceName, oldDescription)
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(s).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("service", err) {
			return c.NewService(ctx, s)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteService deletes a service by (host_name, description) - Nagios's
// DELETE endpoint is keyed differently than GET/PUT (see the Service doc
// comment). hostNamesCSV is the full set of hosts the service applies to,
// comma-joined into a single value, matching what the old provider sent
// (Nagios accepts this as one opaque host_name parameter value, not a
// per-host delete loop).
func (c *Client) DeleteService(ctx context.Context, hostNamesCSV, description string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "service", http.MethodDelete, "host_name", hostNamesCSV, "", description)
	if err != nil {
		return err
	}
	// Space-escaping for this appended fragment is handled centrally in
	// client.do() now, along with every other verb's URL.
	nagiosURL += "&service_description=" + description

	data := &url.Values{}
	data.Set("host_name", hostNamesCSV)
	data.Set("service_description", description)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
