package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Hostgroup mirrors the fields Nagios XI's hostgroup object API accepts/returns.
type Hostgroup struct {
	Name      string   `json:"hostgroup_name"`
	Alias     string   `json:"alias"`
	Members   []string `json:"members,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	NotesURL  string   `json:"notes_url,omitempty"`
	ActionURL string   `json:"action_url,omitempty"`
}

// NewHostgroup creates a hostgroup and applies the config change.
func (c *Client) NewHostgroup(ctx context.Context, h *Hostgroup) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "hostgroup", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(h)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetHostgroup looks up a hostgroup by name. It returns (nil, nil) if none
// exists with that name.
func (c *Client) GetHostgroup(ctx context.Context, name string) (*Hostgroup, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "hostgroup", http.MethodGet, "hostgroup_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var groups []Hostgroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return &groups[0], nil
}

// UpdateHostgroup renames/updates a hostgroup addressed by oldName, falling
// back to creating it fresh if Nagios reports the old name no longer exists.
func (c *Client) UpdateHostgroup(ctx context.Context, h *Hostgroup, oldName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "hostgroup", http.MethodPut, "hostgroup_name", h.Name, oldName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(h).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("hostgroup", err) {
			return c.NewHostgroup(ctx, h)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteHostgroup deletes a hostgroup by name and applies the config change.
func (c *Client) DeleteHostgroup(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "hostgroup", http.MethodDelete, "hostgroup_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("hostgroup_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
