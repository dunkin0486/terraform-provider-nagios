package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewContact_PostsFormEncodedParams verifies NewContact sends the
// contact's fields as a form-urlencoded POST body (not query params) and
// follows up with the applyconfig call every mutating method requires.
func TestNewContact_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/contact":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("contact_name"); got != "mycontact" {
				t.Errorf("contact_name = %q, want %q", got, "mycontact")
			}
			if got := r.PostFormValue("email"); got != "mycontact@example.com" {
				t.Errorf("email = %q, want %q", got, "mycontact@example.com")
			}
			if r.URL.Query().Get("force") != "1" {
				t.Errorf("expected force=1 on create, got query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":"Contact successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewContact(context.Background(), &Contact{ContactName: "mycontact", Email: "mycontact@example.com"})
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

// TestGetContact_Found confirms a successful GET unmarshals the contact.
func TestGetContact_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("contact_name") != "mycontact" {
			t.Errorf("expected contact_name=mycontact in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"contact_name":"mycontact","email":"mycontact@example.com"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetContact(context.Background(), "mycontact")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil contact, got nil")
	}
	if got.ContactName != "mycontact" || got.Email != "mycontact@example.com" {
		t.Errorf("got %+v, want contact_name=mycontact email=mycontact@example.com", got)
	}
}

// TestGetContact_NotFound pins down the (nil, nil)-on-empty-array contract
// (CLAUDE.md quirk 9) - a non-nil empty struct here would silently break
// Terraform's state-clearing logic.
func TestGetContact_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetContact(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil contact for an empty result set, got %+v", got)
	}
}

// TestUpdateContact_Success confirms a successful PUT addresses the *old*
// name as a path segment (CLAUDE.md quirk 3) and still applies config.
func TestUpdateContact_Success(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			if r.URL.Path != "/api/v1/config/contact/oldname" {
				t.Errorf("expected PUT path segment to be the old name, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("contact_name"); got != "newname" {
				t.Errorf("expected new name in query params, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Contact successfully updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateContact(context.Background(), &Contact{ContactName: "newname"}, "oldname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after a successful update, got none")
	}
}

// TestUpdateContact_FallsBackToNewContactOnDoesNotExist exercises the
// existsErrorFor fallback (CLAUDE.md quirk 11): when Nagios reports the old
// name no longer exists, UpdateContact falls back to creating the contact
// fresh via NewContact rather than surfacing the error.
func TestUpdateContact_FallsBackToNewContactOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the contact exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/contact":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Contact successfully added"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateContact(context.Background(), &Contact{ContactName: "newname"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewContact to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateContact to fall back to a create request, got none")
	}
}

// TestDeleteContact_AppliesConfig confirms DeleteContact issues a DELETE and,
// per CLAUDE.md quirk 2, still follows up with an applyconfig call.
func TestDeleteContact_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("contact_name"); got != "mycontact" {
				t.Errorf("expected contact_name=mycontact in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Contact successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteContact(context.Background(), "mycontact"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
