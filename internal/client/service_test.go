package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetService_KeysOffConfigNameAndReturnsFreeVariables confirms GetService
// filters by config_name (not description) and extracts free variables the
// same way GetHost does.
func TestGetService_KeysOffConfigNameAndReturnsFreeVariables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("config_name"); got != "my_service" {
			t.Errorf("expected config_name=my_service in query, got %q", got)
		}
		_, _ = w.Write([]byte(`[{"config_name":"my_service","service_description":"CPU Load","_CUSTOM":"x"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetService(context.Background(), "my_service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil service, got nil")
	}
	if got.Description != "CPU Load" {
		t.Errorf("Description = %q, want %q", got.Description, "CPU Load")
	}
	if got.FreeVariables["_CUSTOM"] != "x" {
		t.Errorf("FreeVariables = %v, want _CUSTOM=x", got.FreeVariables)
	}
}

func TestGetService_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetService(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil service for an empty result set, got %+v", got)
	}
}

// TestUpdateService_PUTAddressesOldNameAndOldDescription confirms the PUT
// path segment is built from (oldServiceName, oldDescription) - services are
// addressed by that compound key on GET/PUT, distinct from DELETE's
// (host_name, description) key (CLAUDE.md quirk 4).
func TestUpdateService_PUTAddressesOldNameAndOldDescription(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			want := "/api/v1/config/service/old_svc/old description"
			if r.URL.Path != want {
				t.Errorf("PUT path = %q, want %q", r.URL.Path, want)
			}
			_, _ = w.Write([]byte(`{"success":"Service successfully updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	s := &Service{ServiceName: "new_svc", Description: "new description", HostName: []string{"host1"}}
	if err := c.UpdateService(context.Background(), s, "old_svc", "old description"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after a successful update, got none")
	}
}

// TestUpdateService_FallsBackToNewServiceOnDoesNotExist mirrors the host
// existsErrorFor fallback test, for the "service" object type string.
func TestUpdateService_FallsBackToNewServiceOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the service exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/service":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Service successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	s := &Service{ServiceName: "new_svc", Description: "new description", HostName: []string{"host1"}}
	if err := c.UpdateService(context.Background(), s, "gone_svc", "gone description"); err != nil {
		t.Fatalf("expected the fallback to NewService to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateService to fall back to a create request, got none")
	}
}

// TestDeleteService_JoinsMultipleHostsAndEscapesSpaces is the real gap
// #88 called out: DeleteService keys off (host_name, description), where
// host_name is the full host set comma-joined into one value (CLAUDE.md
// quirk 4), and no existing test ever exercised more than one host or a
// description containing a space.
func TestDeleteService_JoinsMultipleHostsAndEscapesSpaces(t *testing.T) {
	var requestURI string
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			requestURI = r.RequestURI
			if got := r.URL.Query().Get("host_name"); got != "host1,host2,host3" {
				t.Errorf("host_name = %q, want %q", got, "host1,host2,host3")
			}
			if got := r.URL.Query().Get("service_description"); got != "CPU Load" {
				t.Errorf("service_description = %q, want %q", got, "CPU Load")
			}
			_, _ = w.Write([]byte(`{"success":"Service successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteService(context.Background(), []string{"host1", "host2", "host3"}, "CPU Load"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
	if requestURI == "" {
		t.Fatal("delete request never reached the server handler")
	}
}
