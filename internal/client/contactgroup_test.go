package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewContactgroup_PostsFormEncodedParams verifies NewContactgroup sends
// the contactgroup's fields as a form-urlencoded POST body (not query
// params) and follows up with the applyconfig call every mutating method
// requires.
func TestNewContactgroup_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/contactgroup":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("contactgroup_name"); got != "mygroup" {
				t.Errorf("contactgroup_name = %q, want %q", got, "mygroup")
			}
			if got := r.PostFormValue("alias"); got != "My Group" {
				t.Errorf("alias = %q, want %q", got, "My Group")
			}
			if r.URL.Query().Get("force") != "1" {
				t.Errorf("expected force=1 on create, got query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":"Contactgroup successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewContactgroup(context.Background(), &Contactgroup{ContactgroupName: "mygroup", Alias: "My Group"})
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

// TestGetContactgroup_Found confirms a successful GET unmarshals the
// contactgroup.
func TestGetContactgroup_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("contactgroup_name") != "mygroup" {
			t.Errorf("expected contactgroup_name=mygroup in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"contactgroup_name":"mygroup","alias":"My Group"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetContactgroup(context.Background(), "mygroup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil contactgroup, got nil")
	}
	if got.ContactgroupName != "mygroup" || got.Alias != "My Group" {
		t.Errorf("got %+v, want contactgroup_name=mygroup alias=\"My Group\"", got)
	}
}

// TestGetContactgroup_NotFound pins down the (nil, nil)-on-empty-array
// contract (CLAUDE.md quirk 9) - a non-nil empty struct here would silently
// break Terraform's state-clearing logic.
func TestGetContactgroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetContactgroup(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil contactgroup for an empty result set, got %+v", got)
	}
}

// TestUpdateContactgroup_Success confirms a successful PUT addresses the
// *old* name as a path segment (CLAUDE.md quirk 3) and still applies config.
func TestUpdateContactgroup_Success(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			if r.URL.Path != "/api/v1/config/contactgroup/oldname" {
				t.Errorf("expected PUT path segment to be the old name, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("contactgroup_name"); got != "newname" {
				t.Errorf("expected new name in query params, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Contactgroup successfully updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateContactgroup(context.Background(), &Contactgroup{ContactgroupName: "newname", Alias: "New Group"}, "oldname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after a successful update, got none")
	}
}

// TestUpdateContactgroup_FallsBackToNewContactgroupOnDoesNotExist exercises
// the existsErrorFor fallback (CLAUDE.md quirk 11): when Nagios reports the
// old name no longer exists, UpdateContactgroup falls back to creating the
// contactgroup fresh via NewContactgroup rather than surfacing the error.
func TestUpdateContactgroup_FallsBackToNewContactgroupOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the contactgroup exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/contactgroup":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Contactgroup successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateContactgroup(context.Background(), &Contactgroup{ContactgroupName: "newname", Alias: "New Group"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewContactgroup to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateContactgroup to fall back to a create request, got none")
	}
}

// TestDeleteContactgroup_AppliesConfig confirms DeleteContactgroup issues a
// DELETE and, per CLAUDE.md quirk 2, still follows up with an applyconfig
// call.
func TestDeleteContactgroup_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("contactgroup_name"); got != "mygroup" {
				t.Errorf("expected contactgroup_name=mygroup in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Contactgroup successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteContactgroup(context.Background(), "mygroup"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
