package nna

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewSource_SendsBearerAuthAndJSONBody confirms requests use the
// Authorization: Bearer header (not XI's ?apikey= query param) and a JSON
// body (not form-urlencoded).
func TestNewSource_SendsBearerAuthAndJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sources":
			if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
				t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type header = %q, want application/json", got)
			}
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			if !strings.Contains(string(body), `"flowtype":"netflow"`) {
				t.Errorf("expected JSON body to contain flowtype, got %s", body)
			}
			_, _ = w.Write([]byte(`{"message":"Source started successfully","output":["proc started..."]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sources":
			_, _ = w.Write([]byte(`[{"id":1,"name":"test","port":9995,"lifetime":"30","description":"d","flowtype":"netflow","directory":"/var/1","is_active":1}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSource(context.Background(), &Source{Name: "test", Port: 9995, Lifetime: "30", Description: "d", FlowType: "netflow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 1 {
		t.Fatalf("expected source with id 1, got %+v", got)
	}
}

// TestNewSource_ResolvesIDByListingSinceCreateResponseHasNone confirms
// NewSource looks the created source up by name, since NNA's create
// response never contains the object or its assigned id (confirmed live -
// unlike XI's authserver quirk, which at least returns a server_id).
func TestNewSource_ResolvesIDByListingSinceCreateResponseHasNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"message":"Source started successfully","output":[]}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":42,"name":"other","port":1},{"id":7,"name":"mine","port":2}]`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSource(context.Background(), &Source{Name: "mine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Fatalf("expected the source named 'mine' with id 7, got %+v", got)
	}
}

// TestNewSource_PropagatesValidationError confirms a 422 validation
// failure (e.g. a duplicate name/port) is surfaced as an error rather than
// attempting the post-create name lookup.
func TestNewSource_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The port has already been taken.","errors":{"port":["The port has already been taken."]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSource(context.Background(), &Source{Name: "test", Port: 9995})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil source on validation failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "already been taken") {
		t.Errorf("got error %q, want it to mention the port conflict", err.Error())
	}
}

// TestGetSource_Found confirms a successful GET-by-id unmarshals the bare
// object envelope (not wrapped, unlike PUT's response).
func TestGetSource_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sources/1" {
			t.Errorf("expected path /api/v1/sources/1, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":1,"name":"test","port":9995,"lifetime":"30","description":"d","flowtype":"netflow","directory":"/var/1","is_active":1}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetSource(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "test" || got.Port != 9995 {
		t.Errorf("got %+v, want name=test port=9995", got)
	}
}

// TestGetSource_NotFound confirms NNA's 404 (with a
// {"message": "Resource not found for id: N"} body, not XI's empty-array
// convention) comes back as (nil, nil), per CLAUDE.md quirk 9.
func TestGetSource_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 999"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetSource(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil source for a 404, got %+v", got)
	}
}

// TestUpdateSource_AddressesByIDAndUnwrapsSourceKey confirms PUT is
// addressed by the immutable numeric id (not a rename-by-old-name path
// segment like XI's PUT, CLAUDE.md quirk 3) and that the updated object
// comes back nested under a "source" key.
func TestUpdateSource_AddressesByIDAndUnwrapsSourceKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/sources/5" {
			t.Errorf("expected PUT /api/v1/sources/5, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Source updated successfully","source":{"id":5,"name":"renamed","port":9995,"lifetime":"45","description":"d","flowtype":"netflow","directory":"/var/5","is_active":1}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateSource(context.Background(), 5, &Source{Name: "renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "renamed" || got.Lifetime != "45" {
		t.Errorf("got %+v, want name=renamed lifetime=45", got)
	}
}

// TestDeleteSource_IdempotentOnAlreadyGone confirms a delete of a
// nonexistent id still returns success rather than an error - confirmed
// live, NNA's DELETE returns HTTP 200 "Source deleted successfully."
// regardless of whether the id existed.
func TestDeleteSource_IdempotentOnAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/sources/999" {
			t.Errorf("expected DELETE /api/v1/sources/999, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Source deleted successfully."}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteSource(context.Background(), 999); err != nil {
		t.Errorf("expected no error deleting an already-gone id, got %v", err)
	}
}

// TestStartStopSource_HitDedicatedActionEndpoints confirms enable/disable
// goes through the dedicated action endpoints rather than the create/
// update body, since NNA ignores an is_active field sent there (confirmed
// live: a create request with "is_active":0 still comes back active).
func TestStartStopSource_HitDedicatedActionEndpoints(t *testing.T) {
	var sawStart, sawStop bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sources/3/start":
			sawStart = true
			_, _ = w.Write([]byte(`{"message":"Source started successfully","output":["proc started..."]}`))
		case "/api/v1/sources/3/stop":
			sawStop = true
			_, _ = w.Write([]byte(`{"message":"Source stopped successfully","output":["proc stopped."]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.StartSource(context.Background(), 3); err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}
	if err := c.StopSource(context.Background(), 3); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}
	if !sawStart || !sawStop {
		t.Errorf("expected both start and stop requests, sawStart=%v sawStop=%v", sawStart, sawStop)
	}
}
