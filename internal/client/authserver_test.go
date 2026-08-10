package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewAuthServer_MergesCreateResponseIntoCallerStruct is CLAUDE.md quirk 6:
// the create response body is only {"success":"...", "server_id":"..."} -
// not the full object - so NewAuthServer unmarshals that response directly
// into the caller's already-populated *AuthServer, leaving the other fields
// (set from the caller's input) untouched while filling in ServerID/ID.
func TestNewAuthServer_MergesCreateResponseIntoCallerStruct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/authserver":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("conn_method"); got != "ldap" {
				t.Errorf("conn_method = %q, want %q", got, "ldap")
			}
			_, _ = w.Write([]byte(`{"success":"Authentication server successfully added","server_id":"7"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	a := &AuthServer{Enabled: "1", ConnectionMethod: "ldap", LDAPHost: "ldap.example.com"}
	if err := c.NewAuthServer(context.Background(), a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.ServerID != "7" || a.ID != "7" {
		t.Errorf("ServerID/ID = %q/%q, want both %q", a.ServerID, a.ID, "7")
	}
	if a.LDAPHost != "ldap.example.com" {
		t.Errorf("expected caller-supplied LDAPHost to survive the merge, got %q", a.LDAPHost)
	}
}

// TestNewAuthServer_FailsLoudOnMissingServerID guards against #89 finding 2:
// if Nagios's create response ever omits server_id (violating its own
// documented contract), json.Unmarshal succeeding with a.ServerID left as ""
// used to look like success - a.ID would silently become "", and the
// caller's follow-up GetAuthServer(ctx, "") would then fail with a confusing
// "name must be provided" error, or worse, retry into a duplicate orphaned
// auth server, even though Nagios did actually create one. NewAuthServer
// must fail loud here instead of proceeding with an empty ID.
func TestNewAuthServer_FailsLoudOnMissingServerID(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/authserver":
			_, _ = w.Write([]byte(`{"success":"Authentication server successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	a := &AuthServer{Enabled: "1", ConnectionMethod: "ldap", LDAPHost: "ldap.example.com"}
	err := c.NewAuthServer(context.Background(), a)
	if err == nil {
		t.Fatal("expected an error for a create response missing server_id, got nil")
	}
	if sawApplyConfig {
		t.Error("expected NewAuthServer to fail before applyconfig, since the create response never confirmed a usable ID")
	}
}

// TestNewAuthServer_FailsLoudOnMissingServerID_FallbackPath is the sharper
// version of the test above: UpdateAuthServer's existsErrorFor fallback (see
// CLAUDE.md quirk 11) calls NewAuthServer with an *AuthServer whose ServerID
// is already populated with the *old* server's ID (set by
// resource_authserver.go's Update before the call). json.Unmarshal never
// clears a field when the response omits its key, so a naive
// `a.ServerID == ""` check taken right after unmarshal would silently pass
// in this path even when the create response genuinely omits server_id,
// because the stale old ID is still sitting there from before the call.
// NewAuthServer must still fail loud here.
func TestNewAuthServer_FailsLoudOnMissingServerID_FallbackPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/authserver":
			_, _ = w.Write([]byte(`{"success":"Authentication server successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	// Simulates the existsErrorFor fallback: ID/ServerID pre-populated with
	// the old server's ID before NewAuthServer is called.
	a := &AuthServer{ID: "old-id", ServerID: "old-id", Enabled: "1", ConnectionMethod: "ldap"}
	if err := c.NewAuthServer(context.Background(), a); err == nil {
		t.Fatal("expected an error for a create response missing server_id, even with a stale ServerID already set, got nil")
	}
}

// TestGetAuthServer_EnvelopeResponseReconcilesID confirms GetAuthServer
// unwraps the {"records", "authservers"} envelope every other object type
// doesn't use, and reconciles ServerID <- ID after a GET (the GET response's
// own field is "id", not "server_id" - see the AuthServer doc comment).
func TestGetAuthServer_EnvelopeResponseReconcilesID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("server_id"); got != "7" {
			t.Errorf("expected server_id=7 in query, got %q", got)
		}
		_, _ = w.Write([]byte(`{"records":1,"authservers":[{"id":"7","enabled":"1","conn_method":"ldap"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetAuthServer(context.Background(), "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil auth server, got nil")
	}
	if got.ServerID != "7" {
		t.Errorf("ServerID = %q, want %q (reconciled from id)", got.ServerID, "7")
	}
}

// TestGetAuthServer_EnabledAsRawNumber pins down a real quirk confirmed
// against a live instance while investigating #89: Nagios's "enabled" field
// on this object type is a JSON string ("0"/"1") when it was ever explicitly
// set (via create or update), but comes back as a bare JSON number (no
// quotes) when it was never explicitly set and Nagios applied its own
// server-side default instead. A plain `Enabled string` field fails
// json.Unmarshal outright on the number shape - AuthServer.UnmarshalJSON
// below normalizes both possible shapes, the same pattern Timeperiod uses
// for its "use" field.
func TestGetAuthServer_EnabledAsRawNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":1,"authservers":[{"id":"7","enabled":1,"conn_method":"ldap"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetAuthServer(context.Background(), "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil auth server, got nil")
	}
	if got.Enabled != "1" {
		t.Errorf("Enabled = %q, want %q", got.Enabled, "1")
	}
}

// TestGetAuthServer_NotFound confirms a zero-records envelope comes back as
// (nil, nil), matching every other GetX's not-found contract despite the
// different response shape.
func TestGetAuthServer_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":0,"authservers":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetAuthServer(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil auth server for a zero-records envelope, got %+v", got)
	}
}

// TestUpdateAuthServer_DoesNotFallBackOnUnknownEndpoint is CLAUDE.md quirk 7:
// authserver has no real update route at all - PUT always returns "Unknown
// API endpoint.", not "Does the authentication server exist?" - so the
// existsErrorFor fallback other UpdateX methods use never matches here, and
// the raw error must propagate to the caller rather than triggering a
// surprise create.
func TestUpdateAuthServer_DoesNotFallBackOnUnknownEndpoint(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Unknown API endpoint."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/authserver":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"ok","server_id":"9"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	a := &AuthServer{ID: "7", Enabled: "0", ConnectionMethod: "ldap"}
	err := c.UpdateAuthServer(context.Background(), a, "7")
	if err == nil {
		t.Fatal("expected the raw \"Unknown API endpoint.\" error to propagate, got nil")
	}
	if err.Error() != "Unknown API endpoint." {
		t.Errorf("got error %q, want %q", err.Error(), "Unknown API endpoint.")
	}
	if sawCreate {
		t.Error("expected no fallback create request, but one was made")
	}
}

// TestDeleteAuthServer_UsesPathSegment is CLAUDE.md quirk 5: authserver's
// DELETE uses a "/{id}" path segment, unlike every other object type's
// query-param-only style.
func TestDeleteAuthServer_UsesPathSegment(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			if r.URL.Path != "/api/v1/system/authserver/7" {
				t.Errorf("DELETE path = %q, want %q", r.URL.Path, "/api/v1/system/authserver/7")
			}
			_, _ = w.Write([]byte(`{"success":"Authentication server successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteAuthServer(context.Background(), "7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
