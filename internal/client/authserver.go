package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// AuthServer mirrors the fields Nagios XI's authentication server object API
// accepts/returns.
//
// Nagios has a real quirk here: the create response and the GET-by-id query
// param both use "server_id", but each GET result's own JSON body reports
// the field as "id" instead - so both fields are kept in sync manually
// (ServerID <- ID after a GET, ID <- ServerID after a create) rather than
// being a single field, matching what the live API actually does.
//
// One more confirmed-against-a-live-instance quirk, specific to "enabled":
// it comes back as a JSON string ("0"/"1") once it's ever been explicitly
// set (via create or update), but as a bare JSON number (no quotes) if it
// was never explicitly set and Nagios applied its own server-side default.
// This provider's own Create/Update always send it explicitly (the
// resource's `enabled` schema attribute is Computed+Default, so
// terraform-plugin-framework's plan already has a concrete value before
// Create ever runs), so this only bites on an auth server that was created
// entirely outside this provider and then imported. See
// AuthServer.UnmarshalJSON, which normalizes both possible shapes - the same
// pattern Timeperiod's UnmarshalJSON uses for its "use" field.
type AuthServer struct {
	ID                  string `json:"id"`
	ServerID            string `json:"server_id"`
	Enabled             string `json:"enabled"`
	ConnectionMethod    string `json:"conn_method"`
	ADAccountSuffix     string `json:"ad_account_suffix,omitempty"`
	ADDomainControllers string `json:"ad_domain_controllers,omitempty"`
	BaseDN              string `json:"base_dn,omitempty"`
	SecurityLevel       string `json:"security_level,omitempty"`
	LDAPPort            string `json:"ldap_port,omitempty"`
	LDAPHost            string `json:"ldap_host,omitempty"`
}

// UnmarshalJSON normalizes "enabled"'s inconsistent wire shape (see the
// AuthServer doc comment above) into a string regardless of whether Nagios
// sent a JSON string or a bare number. Every other field decodes via the
// struct's own json tags as usual; only "enabled" needs special handling.
func (a *AuthServer) UnmarshalJSON(data []byte) error {
	type authServerAlias AuthServer
	aux := &struct {
		Enabled json.RawMessage `json:"enabled,omitempty"`
		*authServerAlias
	}{authServerAlias: (*authServerAlias)(a)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Enabled) == 0 {
		return nil
	}

	var asString string
	if err := json.Unmarshal(aux.Enabled, &asString); err == nil {
		a.Enabled = asString
		return nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(aux.Enabled, &asNumber); err != nil {
		return err
	}
	a.Enabled = asNumber.String()
	return nil
}

// authServerListResponse is the envelope Nagios wraps auth server GET
// results in - unlike every other object type, which returns a bare JSON
// array.
type authServerListResponse struct {
	Records int          `json:"records"`
	Entries []AuthServer `json:"authservers"`
}

// NewAuthServer creates an authentication server and applies the config
// change. Nagios assigns server_id itself, so the create request body
// intentionally omits id/server_id (unlike UpdateAuthServer, which sends the
// full object via setURLParams).
//
// Unlike every other NewX method, this one populates a into the freshly
// assigned ID: the create response body is only {"success":"...",
// "server_id":"..."} - not the full object - and Nagios has no other way to
// learn the new ID before a follow-up GET (which itself requires the ID).
// Unmarshaling that response directly into the already-populated a leaves
// its other fields (set from the caller's input) untouched while filling in
// ServerID, matching how the old provider handled this same response shape.
func (c *Client) NewAuthServer(ctx context.Context, a *AuthServer) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "authserver", http.MethodPost, "", "", "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("enabled", a.Enabled)
	data.Set("conn_method", a.ConnectionMethod)
	if a.ADAccountSuffix != "" {
		data.Set("ad_account_suffix", a.ADAccountSuffix)
	}
	if a.ADDomainControllers != "" {
		data.Set("ad_domain_controllers", a.ADDomainControllers)
	}
	if a.BaseDN != "" {
		data.Set("base_dn", a.BaseDN)
	}
	if a.SecurityLevel != "" {
		data.Set("security_level", a.SecurityLevel)
	}
	if a.LDAPPort != "" {
		data.Set("ldap_port", a.LDAPPort)
	}
	if a.LDAPHost != "" {
		data.Set("ldap_host", a.LDAPHost)
	}

	body, err := c.post(ctx, nagiosURL, data)
	if err != nil {
		return err
	}
	// Reset before unmarshaling so a non-empty ServerID below is guaranteed
	// to have come from this response, not a stale value already sitting on
	// a - the existsErrorFor fallback in UpdateAuthServer calls this with an
	// *AuthServer whose ServerID is already populated with the *old*
	// server's ID, and json.Unmarshal never clears a field when the
	// response omits its key.
	a.ServerID = ""
	if err := json.Unmarshal(body, a); err != nil {
		return err
	}
	if a.ServerID == "" {
		return fmt.Errorf("nagios's authserver create response reported success but omitted server_id - the auth server may have been created without this client being able to learn its ID; body: %s", body)
	}
	a.ID = a.ServerID

	return c.applyConfig(ctx)
}

// GetAuthServer looks up an authentication server by ID. It returns
// (nil, nil) if none exists with that ID.
func (c *Client) GetAuthServer(ctx context.Context, id string) (*AuthServer, error) {
	nagiosURL, err := buildURL(c.baseURL, c.token, "authserver", http.MethodGet, "server_id", id, "", "")
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, nagiosURL)
	if err != nil {
		return nil, err
	}

	var resp authServerListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Records == 0 || len(resp.Entries) == 0 {
		return nil, nil
	}

	authServer := resp.Entries[0]
	authServer.ServerID = authServer.ID
	return &authServer, nil
}

// UpdateAuthServer updates an authentication server addressed by oldID,
// falling back to creating it fresh if Nagios reports the old ID no longer
// exists.
func (c *Client) UpdateAuthServer(ctx context.Context, a *AuthServer, oldID string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "authserver", http.MethodPut, "server_id", a.ID, oldID, "")
	if err != nil {
		return err
	}
	nagiosURL += setURLParams(a).Encode()

	if _, err := c.put(ctx, nagiosURL); err != nil {
		if existsErrorFor("authentication server", err) {
			return c.NewAuthServer(ctx, a)
		}
		return err
	}
	return c.applyConfig(ctx)
}

// DeleteAuthServer deletes an authentication server by ID and applies the
// config change.
func (c *Client) DeleteAuthServer(ctx context.Context, id string) error {
	nagiosURL, err := buildURL(c.baseURL, c.token, "authserver", http.MethodDelete, "server_id", id, "", "")
	if err != nil {
		return err
	}

	data := &url.Values{}
	data.Set("server_id", id)

	if _, err := c.delete(ctx, nagiosURL, data); err != nil {
		return err
	}
	return c.applyConfig(ctx)
}
