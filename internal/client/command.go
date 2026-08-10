package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Command mirrors the fields Nagios XI's command object API accepts/returns.
// Unlike most other object types this client wraps, a Nagios command
// definition only has two fields: a name and the check/notification/event
// handler line it runs.
type Command struct {
	CommandName string `json:"command_name"`
	CommandLine string `json:"command_line"`
}

// NewCommand creates a command and applies the config change.
func (c *Client) NewCommand(ctx context.Context, cmd *Command) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "command", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, nagiosURL, setURLParams(cmd)); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}

// GetCommand looks up a command by name. It returns (nil, nil) if none
// exists with that name.
func (c *Client) GetCommand(ctx context.Context, name string) (*Command, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "command", http.MethodGet, "command_name", name, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var commands []Command
	if err := json.Unmarshal(body, &commands); err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, nil
	}
	return &commands[0], nil
}

// UpdateCommand renames/updates a command addressed by oldName, falling back
// to creating it fresh if Nagios reports the old name no longer exists.
func (c *Client) UpdateCommand(ctx context.Context, cmd *Command, oldName string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "command", http.MethodPut, "command_name", cmd.CommandName, oldName, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(cmd).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("command", err) {
			return c.NewCommand(ctx, cmd)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteCommand deletes a command by name and applies the config change.
func (c *Client) DeleteCommand(ctx context.Context, name string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "command", http.MethodDelete, "command_name", name, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("command_name", name)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
