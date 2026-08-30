package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewServicegroup_PostsFormEncodedParams verifies NewServicegroup sends
// the servicegroup's fields as a form-urlencoded POST body (not query
// params) and follows up with the applyconfig call every mutating method
// requires.
func TestNewServicegroup_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/servicegroup":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("servicegroup_name"); got != "mygroup" {
				t.Errorf("servicegroup_name = %q, want %q", got, "mygroup")
			}
			if got := r.PostFormValue("alias"); got != "My Group" {
				t.Errorf("alias = %q, want %q", got, "My Group")
			}
			if r.URL.Query().Get("force") != "1" {
				t.Errorf("expected force=1 on create, got query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":"Servicegroup successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewServicegroup(context.Background(), &Servicegroup{Name: "mygroup", Alias: "My Group"})
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

// TestGetServicegroup_Found confirms a successful GET unmarshals the
// servicegroup.
func TestGetServicegroup_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("servicegroup_name") != "mygroup" {
			t.Errorf("expected servicegroup_name=mygroup in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"servicegroup_name":"mygroup","alias":"My Group"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetServicegroup(context.Background(), "mygroup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil servicegroup, got nil")
	}
	if got.Name != "mygroup" || got.Alias != "My Group" {
		t.Errorf("got %+v, want servicegroup_name=mygroup alias=\"My Group\"", got)
	}
}

// TestGetServicegroup_NotFound pins down the (nil, nil)-on-empty-array
// contract (CLAUDE.md quirk 9) - a non-nil empty struct here would silently
// break Terraform's state-clearing logic.
func TestGetServicegroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetServicegroup(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil servicegroup for an empty result set, got %+v", got)
	}
}

// TestUpdateServicegroup_Success confirms a successful PUT addresses the
// *old* name as a path segment (CLAUDE.md quirk 3) and still applies config.
func TestUpdateServicegroup_Success(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			if r.URL.Path != "/api/v1/config/servicegroup/oldname" {
				t.Errorf("expected PUT path segment to be the old name, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("servicegroup_name"); got != "newname" {
				t.Errorf("expected new name in query params, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Servicegroup successfully updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateServicegroup(context.Background(), &Servicegroup{Name: "newname", Alias: "New Group"}, "oldname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after a successful update, got none")
	}
}

// TestUpdateServicegroup_FallsBackToNewServicegroupOnDoesNotExist exercises
// the existsErrorFor fallback (CLAUDE.md quirk 11): the generic
// implementation falls back to NewServicegroup whenever the error text
// matches "Does the servicegroup exist?" - CLAUDE.md notes the *real*
// Nagios error text for hostgroup/servicegroup doesn't actually match this
// pattern, making the fallback unreachable live, but this test still pins
// down that the shared generic code path itself behaves correctly given a
// matching error.
func TestUpdateServicegroup_FallsBackToNewServicegroupOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the servicegroup exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/servicegroup":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Servicegroup successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateServicegroup(context.Background(), &Servicegroup{Name: "newname", Alias: "New Group"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewServicegroup to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateServicegroup to fall back to a create request, got none")
	}
}

// TestDeleteServicegroup_AppliesConfig confirms DeleteServicegroup issues a
// DELETE and, per CLAUDE.md quirk 2, still follows up with an applyconfig
// call.
func TestDeleteServicegroup_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("servicegroup_name"); got != "mygroup" {
				t.Errorf("expected servicegroup_name=mygroup in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Servicegroup successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteServicegroup(context.Background(), "mygroup"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
