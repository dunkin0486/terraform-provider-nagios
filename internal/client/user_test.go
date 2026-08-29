package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewUser_MergesCreateResponseIntoCallerStruct is #174's thin-response
// quirk, same shape as AuthServer's: the create response body is only
// {"success":"...", "user_id":<id>} - not the full object - so NewUser
// unmarshals that response directly into the caller's already-populated
// *User, leaving the other fields (set from the caller's input) untouched
// while filling in ID. It also confirms no applyconfig call is made - #174
// found XI-panel user accounts take effect immediately, unlike core
// monitoring config objects (CLAUDE.md quirk 2). user_id is a bare JSON
// number here, not a quoted string - confirmed against a live instance by
// this client's own acceptance suite (unlike authserver's server_id, a
// quoted string) - see TestNewUser_UserIDAsRawNumber for the dedicated case.
func TestNewUser_MergesCreateResponseIntoCallerStruct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/user":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("username"); got != "jdoe" {
				t.Errorf("username = %q, want %q", got, "jdoe")
			}
			if got := r.PostFormValue("auth_level"); got != "admin" {
				t.Errorf("auth_level = %q, want %q", got, "admin")
			}
			_, _ = w.Write([]byte(`{"success":"User successfully added","user_id":9}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{Username: "jdoe", Password: "s3cret", Name: "Jane Doe", Email: "jdoe@example.com", AuthLevel: "admin"}
	if err := c.NewUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != "9" {
		t.Errorf("ID = %q, want %q", u.ID, "9")
	}
	if u.Email != "jdoe@example.com" {
		t.Errorf("expected caller-supplied Email to survive the merge, got %q", u.Email)
	}
}

// TestNewUser_UserIDAsRawNumber is a dedicated regression test for the same
// case TestNewUser_MergesCreateResponseIntoCallerStruct's response body
// already exercises: Nagios sends user_id back as a bare JSON number, not a
// quoted string, on the create response - discovered by running this
// client's own acceptance suite against a live instance, not anticipated by
// #174's investigation notes. Without User.UnmarshalJSON's normalization,
// json.Unmarshal fails outright with "cannot unmarshal number into Go
// struct field ... of type string".
func TestNewUser_UserIDAsRawNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":"User successfully added","user_id":42}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{Username: "jdoe", Password: "s3cret", Name: "Jane Doe", Email: "jdoe@example.com", AuthLevel: "user"}
	if err := c.NewUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != "42" {
		t.Errorf("ID = %q, want %q", u.ID, "42")
	}
}

// TestNewUser_FailsLoudOnMissingUserID mirrors
// TestNewAuthServer_FailsLoudOnMissingServerID: if Nagios's create response
// ever omits user_id (violating its own documented contract), NewUser must
// fail loud rather than silently leaving u.ID empty.
func TestNewUser_FailsLoudOnMissingUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":"User successfully added"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{Username: "jdoe", Password: "s3cret", Name: "Jane Doe", Email: "jdoe@example.com", AuthLevel: "user"}
	if err := c.NewUser(context.Background(), u); err == nil {
		t.Fatal("expected an error for a create response missing user_id, got nil")
	}
}

// TestNewUser_FailsLoudOnMissingUserID_FallbackPath is the sharper version:
// UpdateUser's existsErrorFor fallback calls NewUser with a *User whose ID
// is already populated with the *old* user's ID. A naive `u.ID == ""` check
// taken right after unmarshal would silently pass in this path even when the
// create response genuinely omits user_id - NewUser must still fail loud.
func TestNewUser_FailsLoudOnMissingUserID_FallbackPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":"User successfully added"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{ID: "old-id", Username: "jdoe", Password: "s3cret", Name: "Jane Doe", Email: "jdoe@example.com", AuthLevel: "user"}
	if err := c.NewUser(context.Background(), u); err == nil {
		t.Fatal("expected an error for a create response missing user_id, even with a stale ID already set, got nil")
	}
}

// TestGetUser_FiltersClientSideByUsername is #174's central GetUser finding:
// GET system/user's own username= query filter is silently ignored by
// Nagios, so GetUser fetches the full unfiltered list and scans it
// client-side - the only GetX in this client that does this.
func TestGetUser_FiltersClientSideByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("username") {
			t.Errorf("expected no username filter in the request query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"records":2,"users":[
			{"user_id":"1","username":"alice","name":"Alice","email":"alice@example.com","enabled":"1"},
			{"user_id":"9","username":"jdoe","name":"Jane Doe","email":"jdoe@example.com","enabled":"1"}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), "jdoe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil user, got nil")
	}
	if got.ID != "9" {
		t.Errorf("ID = %q, want %q", got.ID, "9")
	}
}

// TestGetUser_EnabledAsRawNumber mirrors TestGetAuthServer_EnabledAsRawNumber:
// Nagios has been confirmed (against authserver) to send a Computed+Default
// bool field as a bare JSON number, not a quoted string, when it was never
// explicitly set and Nagios applied its own server-side default - a real
// possibility for a user account created outside Terraform and then
// imported. User.UnmarshalJSON must normalize this the same way
// AuthServer.UnmarshalJSON does, or json.Unmarshal fails outright.
func TestGetUser_EnabledAsRawNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":1,"users":[{"user_id":"1","username":"jdoe","name":"Jane Doe","email":"jdoe@example.com","enabled":1}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), "jdoe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil user, got nil")
	}
	if got.Enabled != "1" {
		t.Errorf("Enabled = %q, want %q", got.Enabled, "1")
	}
}

// TestGetUser_NotFound confirms no matching username in the list comes back
// as (nil, nil), matching every other GetX's not-found contract.
func TestGetUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":1,"users":[{"user_id":"1","username":"alice","name":"Alice","email":"alice@example.com","enabled":"1"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil user for a username not present in the list, got %+v", got)
	}
}

// TestGetUser_ZeroRecordsEnvelope confirms a zero-records envelope (no
// "users" key at all) comes back as (nil, nil) rather than failing to parse.
func TestGetUser_ZeroRecordsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":0}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), "jdoe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil user for a zero-records envelope, got %+v", got)
	}
}

// TestGetUser_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound guards
// against #189: unlike GetHost (#89 finding 3, see
// TestGetHost_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound), GetUser
// unmarshals into an envelope struct ({"records", "users": [...]}) rather
// than a bare array. An {"error":"..."} response has neither a "records" nor
// a "users" key, so json.Unmarshal silently ignores it and leaves the
// envelope at its zero value - Records==0, Entries==nil - which used to look
// exactly like a genuine zero-result GET. Confirmed against a live instance
// (#189): an auth failure returns the same {"error":"..."} object shape
// host's GET does. GetUser must now surface that as an error instead of
// (nil, nil).
func TestGetUser_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"Invalid API Key"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetUser(context.Background(), "jdoe")
	if err == nil {
		t.Fatal("expected an error for an {\"error\":...} response body, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil user alongside the error, got %+v", got)
	}
}

// TestUpdateUser_PUTAddressesOldIDPathSegment confirms UpdateUser addresses
// the PUT by the old numeric ID as a path segment (#174: user is addressed
// by user_id, not by name, unlike host/contact's rename-by-old-name PUT) and
// makes no applyconfig call.
func TestUpdateUser_PUTAddressesOldIDPathSegment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.Path != "/api/v1/system/user/9" {
				t.Errorf("PUT path = %q, want %q", r.URL.Path, "/api/v1/system/user/9")
			}
			if got := r.URL.Query().Get("email"); got != "jdoe@example.com" {
				t.Errorf("email = %q, want %q (PUT requires email on every call, #174)", got, "jdoe@example.com")
			}
			_, _ = w.Write([]byte(`{"success":"User successfully updated"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{ID: "9", Username: "jdoe2", Name: "Jane Doe", Email: "jdoe@example.com"}
	if err := c.UpdateUser(context.Background(), u, "9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdateUser_DoesNotFallBackOnMismatchedErrorText is CLAUDE.md quirk 11's
// user-specific instance: Nagios's error text on a missing id
// ("User with ID <n> does not exist.") doesn't match existsErrorFor's
// "Does the X exist?" pattern - same known gap as hostgroup/servicegroup -
// so the raw error must propagate rather than triggering a surprise create.
func TestUpdateUser_DoesNotFallBackOnMismatchedErrorText(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"User with ID 9 does not exist."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/user":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"ok","user_id":"10"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	u := &User{ID: "9", Username: "jdoe", Name: "Jane Doe", Email: "jdoe@example.com"}
	err := c.UpdateUser(context.Background(), u, "9")
	if err == nil {
		t.Fatal("expected the raw \"User with ID 9 does not exist.\" error to propagate, got nil")
	}
	if err.Error() != "User with ID 9 does not exist." {
		t.Errorf("got error %q, want %q", err.Error(), "User with ID 9 does not exist.")
	}
	if sawCreate {
		t.Error("expected no fallback create request, but one was made")
	}
}

// TestDeleteUser_UsesPathSegment is #174's DELETE addressing quirk, the same
// class as authserver's documented DELETE quirk (CLAUDE.md #5): user's
// DELETE uses a "/{id}" path segment, unlike most other object types'
// query-param-only style. Also confirms no applyconfig call is made.
func TestDeleteUser_UsesPathSegment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			if r.URL.Path != "/api/v1/system/user/9" {
				t.Errorf("DELETE path = %q, want %q", r.URL.Path, "/api/v1/system/user/9")
			}
			_, _ = w.Write([]byte(`{"success":"User successfully deleted"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteUser(context.Background(), "9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
