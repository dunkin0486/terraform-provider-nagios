package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Contact mirrors the fields Nagios XI's contact object API accepts/returns.
type Contact struct {
	ContactName                 string   `json:"contact_name"`
	HostNotificationsEnabled    string   `json:"host_notifications_enabled,omitempty"`
	ServiceNotificationsEnabled string   `json:"service_notifications_enabled,omitempty"`
	HostNotificationPeriod      string   `json:"host_notification_period,omitempty"`
	ServiceNotificationPeriod   string   `json:"service_notification_period,omitempty"`
	HostNotificationOptions     string   `json:"host_notification_options,omitempty"`
	ServiceNotificationOptions  string   `json:"service_notification_options,omitempty"`
	HostNotificationCommands    []string `json:"host_notification_commands,omitempty"`
	ServiceNotificationCommands []string `json:"service_notification_commands,omitempty"`
	Alias                       string   `json:"alias,omitempty"`
	ContactGroups               []string `json:"contact_groups,omitempty"`
	Templates                   []string `json:"use,omitempty"`
	Email                       string   `json:"email,omitempty"`
	Pager                       string   `json:"pager,omitempty"`
	Address1                    string   `json:"address1,omitempty"`
	Address2                    string   `json:"address2,omitempty"`
	Address3                    string   `json:"address3,omitempty"`
	CanSubmitCommands           string   `json:"can_submit_commands,omitempty"`
	RetainStatusInformation     string   `json:"retain_status_information,omitempty"`
	RetainNonstatusInformation  string   `json:"retain_nonstatus_information,omitempty"`
}

// NewContact creates a contact and applies the config change.
func (c *Client) NewContact(ctx context.Context, contact *Contact) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contact", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(contact)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetContact looks up a contact by name. It returns (nil, nil) if none
// exists with that name.
func (c *Client) GetContact(ctx context.Context, name string) (*Contact, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contact", http.MethodGet, "contact_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var contacts []Contact
	if err := json.Unmarshal(body, &contacts); err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}
	return &contacts[0], nil
}

// UpdateContact renames/updates a contact addressed by oldContactName,
// falling back to creating it fresh if Nagios reports the old name no
// longer exists.
func (c *Client) UpdateContact(ctx context.Context, contact *Contact, oldContactName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contact", http.MethodPut, "contact_name", contact.ContactName, oldContactName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(contact).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("contact", err) {
			return c.NewContact(ctx, contact)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteContact deletes a contact by name and applies the config change.
func (c *Client) DeleteContact(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contact", http.MethodDelete, "contact_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("contact_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
