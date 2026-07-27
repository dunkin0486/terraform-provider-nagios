package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Servicegroup mirrors the fields Nagios XI's servicegroup object API
// accepts/returns.
type Servicegroup struct {
	Name      string   `json:"servicegroup_name,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	Members   []string `json:"members,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	NotesURL  string   `json:"notes_url,omitempty"`
	ActionURL string   `json:"action_url,omitempty"`
}

// NewServicegroup creates a servicegroup and applies the config change.
func (c *Client) NewServicegroup(ctx context.Context, sg *Servicegroup) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "servicegroup", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(sg)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetServicegroup looks up a servicegroup by name. It returns (nil, nil) if
// none exists with that name.
func (c *Client) GetServicegroup(ctx context.Context, name string) (*Servicegroup, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "servicegroup", http.MethodGet, "servicegroup_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var groups []Servicegroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return &groups[0], nil
}

// UpdateServicegroup renames/updates a servicegroup addressed by oldName,
// falling back to creating it fresh if Nagios reports the old name no
// longer exists.
func (c *Client) UpdateServicegroup(ctx context.Context, sg *Servicegroup, oldName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "servicegroup", http.MethodPut, "servicegroup_name", sg.Name, oldName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(sg).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("servicegroup", err) {
			return c.NewServicegroup(ctx, sg)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteServicegroup deletes a servicegroup by name and applies the config
// change.
func (c *Client) DeleteServicegroup(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "servicegroup", http.MethodDelete, "servicegroup_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("servicegroup_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
