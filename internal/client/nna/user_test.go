package nna

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewUser_SendsBearerAuthAndConvertsFields confirms the create request
// carries the Bearer header, auto-derives confirm_password from Password
// (NNA validates the two match but only ever stores one), and marshals
// APIAccess as the string "1"/"0" NNA's validator requires (confirmed live:
// a JSON boolean is rejected with "The apiaccess field must be a string.").
func TestNewUser_SendsBearerAuthAndConvertsFields(t *testing.T) {
	// The follow-up GET by id NewUser makes since the create response
	// carries only the id, not the object.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method on /api/v1/users: %s", r.Method)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		got := string(body)
		for _, want := range []string{`"password":"Secret123!"`, `"confirm_password":"Secret123!"`, `"apiaccess":"1"`} {
			if !strings.Contains(got, want) {
				t.Errorf("expected JSON body to contain %s, got %s", want, got)
			}
		}
		_, _ = w.Write([]byte(`{"message":"User created successfully","user_id":7}`))
	})
	mux.HandleFunc("/api/v1/users/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method on /api/v1/users/7: %s", r.Method)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"username":"tftest","email":"tftest@example.com","role_id":2,"apiaccess":true,"lang":"en_US","theme":"default","type":"Local"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewUser(context.Background(), &User{
		Username:  "tftest",
		Password:  "Secret123!",
		Email:     "tftest@example.com",
		RoleID:    2,
		APIAccess: true,
		Lang:      "en_US",
		Theme:     "default",
		Type:      "Local",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 || got.Username != "tftest" {
		t.Fatalf("expected user id 7 named tftest, got %+v", got)
	}
}

// TestNewUser_PropagatesValidationError confirms a 422 validation failure
// is surfaced as an error rather than attempting the follow-up GET.
func TestNewUser_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The lang field is required.","errors":{"lang":["The lang field is required."]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewUser(context.Background(), &User{Username: "tftest"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil user on validation failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "lang field is required") {
		t.Errorf("got error %q, want it to mention the lang field", err.Error())
	}
}

// TestGetUser_Found confirms a successful GET-by-id unmarshals the bare
// object envelope.
func TestGetUser_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/1" {
			t.Errorf("expected path /api/v1/users/1, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":1,"username":"nagiosadmin","email":"a@example.com","role_id":1,"apiaccess":true,"lang":"en_US","theme":"default","type":"Local"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Username != "nagiosadmin" {
		t.Errorf("got %+v, want username=nagiosadmin", got)
	}
}

// TestGetUser_NotFound confirms NNA's per-type 404 body for users
// ({"message": "User not found"}, distinct from sources'/source-groups'
// "Resource not found for id: N" shape - confirmed live) still comes back
// as (nil, nil), per CLAUDE.md quirk 9.
func TestGetUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"User not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil user for a 404, got %+v", got)
	}
}

// TestUpdateUser_UsesPATCHNotPUTAndFollowsUpWithGET confirms the update
// verb is PATCH, not PUT - confirmed live, NNA's router rejects PUT on this
// route with a 500 "PUT method is not supported... Supported methods: GET,
// HEAD, PATCH, DELETE." - and that, since the PATCH response is just
// {"message": "..."} with no object (unlike UpdateSource's {"source": {}}
// wrapper), the updated object is fetched with a follow-up GetUser.
func TestUpdateUser_UsesPATCHNotPUTAndFollowsUpWithGET(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/5", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"message":"User updated successfully"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":5,"username":"renamed","email":"a@example.com","role_id":2,"apiaccess":false,"lang":"en_US","theme":"dark","type":"Local"}`))
		default:
			t.Errorf("expected PATCH or GET /api/v1/users/5, got %s", r.Method)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateUser(context.Background(), 5, &User{Theme: "dark"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Theme != "dark" || got.Username != "renamed" {
		t.Errorf("got %+v, want theme=dark username=renamed (from the follow-up GET)", got)
	}
}

// TestDeleteUser_NotIdempotentOnAlreadyGone confirms that, unlike
// DeleteSource/DeleteSourceGroup, a second delete of an already-gone user
// id is NOT idempotent - confirmed live, it returns HTTP 403 {"message":
// "This action is unauthorized."} rather than a 200.
func TestDeleteUser_NotIdempotentOnAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/users/999" {
			t.Errorf("expected DELETE /api/v1/users/999, got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"This action is unauthorized."}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteUser(context.Background(), 999); err == nil {
		t.Error("expected an error deleting an already-gone user id, got nil")
	}
}

// TestDeleteUser_Success confirms a normal delete of an existing id
// succeeds.
func TestDeleteUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/users/5" {
			t.Errorf("expected DELETE /api/v1/users/5, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"User deleted successfully"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteUser(context.Background(), 5); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestListUsers_ReturnsFlatArray confirms the list response is a bare
// array, same as ListSourceGroups.
func TestListUsers_ReturnsFlatArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users" {
			t.Errorf("expected path /api/v1/users, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":1,"username":"nagiosadmin"},{"id":2,"username":"other"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 users, got %d", len(got))
	}
}
