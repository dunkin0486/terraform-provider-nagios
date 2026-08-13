package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// User mirrors the fields Nagios XI's system/user object API accepts/
// returns - XI's own login/admin panel accounts, distinct from monitoring
// contacts (see #174).
//
// Password, AuthLevel, ForcePwChange, AuthType, and AuthServerID are
// write-only: Nagios accepts them on create/update but never returns any of
// them from a GET under any field name, confirmed against a live instance
// (#174). internal/provider's modelFromUser never assigns these fields, so
// Terraform's stored state simply retains whatever was last written from
// config - a one-way apply with no drift detection, rather than the usual
// GetX-populates-everything contract every other object type follows.
type User struct {
	ID            string `json:"user_id"`
	Username      string `json:"username"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Enabled       string `json:"enabled,omitempty"`
	Password      string `json:"password,omitempty"`
	AuthLevel     string `json:"auth_level,omitempty"`
	ForcePwChange string `json:"force_pw_change,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	AuthServerID  string `json:"auth_server_id,omitempty"`
}

// UnmarshalJSON normalizes two fields' wire shape, both confirmed against a
// live instance:
//
//   - "user_id" comes back as a bare JSON number on the create response
//     (`{"success":"...","user_id":9}`, not `"9"`) even though every other
//     object type's equivalent ID field (e.g. authserver's "server_id") is a
//     quoted string - discovered by this client's own acceptance suite, not
//     anticipated by #174's investigation notes.
//   - "enabled" is a JSON string once it's ever been explicitly set (via
//     create or update), but a bare JSON number if it was never explicitly
//     set and Nagios applied its own server-side default - the same
//     Computed+Default dual-shape quirk AuthServer.UnmarshalJSON normalizes.
//     This provider's own Create/Update never trigger the number shape (the
//     resource's enabled schema attribute is Computed+Default, so
//     terraform-plugin-framework always sends a concrete value), but an
//     account created outside Terraform and then imported could.
//
// A plain `string` field fails json.Unmarshal outright on the number shape
// without this. Every other field decodes via the struct's own json tags as
// usual.
func (u *User) UnmarshalJSON(data []byte) error {
	type userAlias User
	aux := &struct {
		ID      json.RawMessage `json:"user_id,omitempty"`
		Enabled json.RawMessage `json:"enabled,omitempty"`
		*userAlias
	}{userAlias: (*userAlias)(u)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.ID) > 0 {
		id, err := stringOrNumberJSON(aux.ID)
		if err != nil {
			return err
		}
		u.ID = id
	}

	if len(aux.Enabled) == 0 {
		return nil
	}

	enabled, err := stringOrNumberJSON(aux.Enabled)
	if err != nil {
		return err
	}
	u.Enabled = enabled
	return nil
}

// stringOrNumberJSON decodes a raw JSON value that Nagios sends as either a
// quoted string or a bare number, depending on the field/scenario (see
// User.UnmarshalJSON), into a Go string either way.
func stringOrNumberJSON(raw json.RawMessage) (string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return "", err
	}
	return asNumber.String(), nil
}

// usersListResponse is the envelope Nagios wraps user GET results in - the
// same "records"/named-array shape authserver uses, not a bare JSON array.
type usersListResponse struct {
	Records int    `json:"records"`
	Entries []User `json:"users"`
}

// NewUser creates a user and returns. Unlike every non-authserver NewX
// method, this makes no applyconfig call: #174 confirmed against a live
// instance that XI-panel login accounts take effect immediately, since
// they're stored in Nagios's own app DB rather than being part of the
// monitoring core config applyconfig regenerates.
//
// Like NewAuthServer, the create response body is only
// {"success":"...", "user_id":<id>} - not the full object - so this
// unmarshals that response directly into the caller's already-populated *u,
// leaving its other fields untouched while filling in ID. Unlike
// NewAuthServer's server_id, user_id comes back as a bare JSON number here,
// not a quoted string - confirmed by this client's own acceptance suite, see
// User.UnmarshalJSON. u.ID is reset before unmarshaling so a non-empty ID
// afterward is guaranteed to have come from this response - UpdateUser's
// existsErrorFor fallback calls this with a *User whose ID is already
// populated with the *old* user's ID.
func (c *Client) NewUser(ctx context.Context, u *User) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "user", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}

	body, err := c.post(ctx, nagiosURL, setURLParams(u))
	if err != nil {
		return err
	}

	u.ID = ""
	if err := json.Unmarshal(body, u); err != nil {
		return err
	}
	if u.ID == "" {
		return fmt.Errorf("nagios's user create response reported success but omitted user_id - the user may have been created without this client being able to learn its ID; body: %s", body)
	}

	return nil
}

// GetUser looks up a user by username. It returns (nil, nil) if none exists
// with that username.
//
// Unlike every other GetX in this client, this can't filter server-side:
// GET system/user's own username= query filter is silently ignored by
// Nagios (confirmed against a live instance, #174) - only user_id filters.
// So this fetches the full unfiltered user list and scans it client-side
// instead.
func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "user", http.MethodGet, "", "", "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var resp usersListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	for _, u := range resp.Entries {
		if u.Username == username {
			user := u
			return &user, nil
		}
	}
	return nil, nil
}

// UpdateUser updates a user addressed by oldID (a /{id} path segment, not by
// name - #174), falling back to creating it fresh if Nagios reports the old
// id no longer exists. In practice that fallback never fires: Nagios's error
// text on a missing id ("User with ID <n> does not exist.") doesn't match
// existsErrorFor's "Does the X exist?" pattern - the same known gap
// CLAUDE.md documents for hostgroup/servicegroup (quirk 11). Kept anyway for
// consistency with every other UpdateX. No applyconfig call - see NewUser.
func (c *Client) UpdateUser(ctx context.Context, u *User, oldID string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "user", http.MethodPut, "user_id", u.ID, oldID, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(u).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("user", err) {
			return c.NewUser(ctx, u)
		}
		return err
	}
	return nil
}

// DeleteUser deletes a user by id, addressed as a /{id} path segment - the
// same DELETE addressing quirk CLAUDE.md documents for authserver (quirk 5).
// No applyconfig call - see NewUser.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "user", http.MethodDelete, "user_id", id, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("user_id", id)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return nil
}
