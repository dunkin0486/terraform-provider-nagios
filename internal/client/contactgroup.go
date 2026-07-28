package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Contactgroup mirrors the fields Nagios XI's contactgroup object API
// accepts/returns.
type Contactgroup struct {
	ContactgroupName    string   `json:"contactgroup_name"`
	Alias               string   `json:"alias"`
	Members             []string `json:"members,omitempty"`
	ContactgroupMembers []string `json:"contactgroup_members,omitempty"`
}

// NewContactgroup creates a contactgroup and applies the config change.
func (c *Client) NewContactgroup(ctx context.Context, cg *Contactgroup) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contactgroup", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(cg)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetContactgroup looks up a contactgroup by name. It returns (nil, nil) if
// none exists with that name.
func (c *Client) GetContactgroup(ctx context.Context, name string) (*Contactgroup, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contactgroup", http.MethodGet, "contactgroup_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var groups []Contactgroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return &groups[0], nil
}

// UpdateContactgroup renames/updates a contactgroup addressed by oldName,
// falling back to creating it fresh if Nagios reports the old name no
// longer exists.
func (c *Client) UpdateContactgroup(ctx context.Context, cg *Contactgroup, oldName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contactgroup", http.MethodPut, "contactgroup_name", cg.ContactgroupName, oldName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(cg).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("contactgroup", err) {
			return c.NewContactgroup(ctx, cg)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteContactgroup deletes a contactgroup by name and applies the config
// change.
func (c *Client) DeleteContactgroup(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "contactgroup", http.MethodDelete, "contactgroup_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("contactgroup_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
