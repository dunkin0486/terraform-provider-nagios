package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewHost_PostsFormEncodedParams verifies NewHost sends the host's fields
// as a form-urlencoded POST body (not query params) and follows up with the
// applyconfig call every mutating method requires.
func TestNewHost_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/host":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("host_name"); got != "myhost" {
				t.Errorf("host_name = %q, want %q", got, "myhost")
			}
			if got := r.PostFormValue("address"); got != "127.0.0.1" {
				t.Errorf("address = %q, want %q", got, "127.0.0.1")
			}
			if r.URL.Query().Get("force") != "1" {
				t.Errorf("expected force=1 on create, got query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":"Host successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewHost(context.Background(), &Host{HostName: "myhost", Address: "127.0.0.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawCreate {
		t.Error("expected a create request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after create, got none")
	}
}

// TestGetHost_Found confirms a successful GET both unmarshals the host and
// extracts free variables from the dynamic top-level "_"-prefixed keys
// (see CLAUDE.md quirk 8), not from a nested "free_variables" field.
func TestGetHost_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("host_name") != "myhost" {
			t.Errorf("expected host_name=myhost in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"host_name":"myhost","address":"127.0.0.1","_SNMPCOMMUNITY":"public"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetHost(context.Background(), "myhost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil host, got nil")
	}
	if got.HostName != "myhost" || got.Address != "127.0.0.1" {
		t.Errorf("got %+v, want host_name=myhost address=127.0.0.1", got)
	}
	if got.FreeVariables["_SNMPCOMMUNITY"] != "public" {
		t.Errorf("FreeVariables = %v, want _SNMPCOMMUNITY=public", got.FreeVariables)
	}
}

// TestGetHost_NotFound is the exact contract CLAUDE.md documents as a
// previously-shipped real bug: an empty result set must come back as
// (nil, nil), never a non-nil empty struct, or Terraform's state-clearing
// logic silently breaks.
func TestGetHost_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetHost(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil host for an empty result set, got %+v", got)
	}
}

// TestUpdateHost_Success confirms a successful PUT addresses the *old* name
// as a path segment (CLAUDE.md quirk 3) and still applies config.
func TestUpdateHost_Success(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			if r.URL.Path != "/api/v1/config/host/oldname" {
				t.Errorf("expected PUT path segment to be the old name, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("host_name"); got != "newname" {
				t.Errorf("expected new name in query params, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Host successfully updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateHost(context.Background(), &Host{HostName: "newname", Address: "127.0.0.1"}, "oldname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after a successful update, got none")
	}
}

// TestUpdateHost_FallsBackToNewHostOnDoesNotExist exercises the
// existsErrorFor fallback (CLAUDE.md quirk 11): when Nagios reports the old
// name no longer exists (e.g. manually deleted out-of-band, or already
// renamed), UpdateHost falls back to creating the host fresh via NewHost
// rather than surfacing the error.
func TestUpdateHost_FallsBackToNewHostOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the host exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/host":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Host successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateHost(context.Background(), &Host{HostName: "newname", Address: "127.0.0.1"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewHost to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateHost to fall back to a create request, got none")
	}
}

// TestUpdateHost_PropagatesOtherErrors confirms an unrelated PUT failure is
// surfaced as-is, not silently swallowed into a fallback create.
func TestUpdateHost_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"error":"Some other failure"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateHost(context.Background(), &Host{HostName: "newname", Address: "127.0.0.1"}, "oldname")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err.Error() != "Some other failure" {
		t.Errorf("got error %q, want %q", err.Error(), "Some other failure")
	}
}

// TestDeleteHost_AppliesConfig confirms DeleteHost issues a DELETE and, per
// CLAUDE.md quirk 2, still follows up with an applyconfig call.
func TestDeleteHost_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("host_name"); got != "myhost" {
				t.Errorf("expected host_name=myhost in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Host successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteHost(context.Background(), "myhost"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
