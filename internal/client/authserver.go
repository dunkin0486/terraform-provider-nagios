package client

import (
	"context"
	"encoding/json"
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
	if err := json.Unmarshal(body, a); err != nil {
		return err
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
