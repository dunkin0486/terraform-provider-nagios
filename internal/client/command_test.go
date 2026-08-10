package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewCommand_PostsFormEncodedParams verifies NewCommand sends the
// command's fields as a form-urlencoded POST body and follows up with the
// applyconfig call every mutating method requires (CLAUDE.md quirk 2).
func TestNewCommand_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/command":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("command_name"); got != "check_ping" {
				t.Errorf("command_name = %q, want %q", got, "check_ping")
			}
			if got := r.PostFormValue("command_line"); got != "$USER1$/check_ping -H $HOSTADDRESS$" {
				t.Errorf("command_line = %q, want %q", got, "$USER1$/check_ping -H $HOSTADDRESS$")
			}
			_, _ = w.Write([]byte(`{"success":"Added check_ping to the system."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewCommand(context.Background(), &Command{CommandName: "check_ping", CommandLine: "$USER1$/check_ping -H $HOSTADDRESS$"})
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

// TestGetCommand_Found confirms a successful GET unmarshals both fields.
func TestGetCommand_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("command_name") != "check_ping" {
			t.Errorf("expected command_name=check_ping in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"command_name":"check_ping","command_line":"$USER1$/check_ping -H $HOSTADDRESS$"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetCommand(context.Background(), "check_ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil command, got nil")
	}
	if got.CommandName != "check_ping" || got.CommandLine != "$USER1$/check_ping -H $HOSTADDRESS$" {
		t.Errorf("got %+v, want command_name=check_ping command_line=$USER1$/check_ping -H $HOSTADDRESS$", got)
	}
}

// TestGetCommand_NotFound confirms an empty result set comes back as
// (nil, nil), per CLAUDE.md quirk 9.
func TestGetCommand_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetCommand(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil command for an empty result set, got %+v", got)
	}
}

// TestUpdateCommand_AddressesOldNameAndAppliesConfig confirms UpdateCommand
// PUTs to the old name's URL segment (CLAUDE.md quirk 3) with the new fields
// as params, then applies config on success.
func TestUpdateCommand_AddressesOldNameAndAppliesConfig(t *testing.T) {
	var sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			if !strings.Contains(r.URL.Path, "check_ping_old") {
				t.Errorf("expected PUT path to address old name check_ping_old, got %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("command_name"); got != "check_ping" {
				t.Errorf("expected new command_name=check_ping in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Updated check_ping in the system."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateCommand(context.Background(), &Command{CommandName: "check_ping", CommandLine: "$USER1$/check_ping"}, "check_ping_old")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after update, got none")
	}
}

// TestUpdateCommand_FallsBackToNewCommandOnDoesNotExist exercises the
// existsErrorFor fallback (CLAUDE.md quirk 11).
func TestUpdateCommand_FallsBackToNewCommandOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the command exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/command":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Added check_ping to the system."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateCommand(context.Background(), &Command{CommandName: "check_ping", CommandLine: "$USER1$/check_ping"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewCommand to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateCommand to fall back to a create request, got none")
	}
}

// TestUpdateCommand_PropagatesOtherErrors confirms an unrelated PUT failure
// is surfaced rather than silently swallowed into a fallback create.
func TestUpdateCommand_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"error":"Some other failure"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateCommand(context.Background(), &Command{CommandName: "check_ping", CommandLine: "$USER1$/check_ping"}, "check_ping")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "Some other failure") {
		t.Errorf("got error %q, want it to still contain the underlying %q", err.Error(), "Some other failure")
	}
}

// TestDeleteCommand_AppliesConfig confirms DeleteCommand issues a DELETE and,
// per CLAUDE.md quirk 2, still follows up with an applyconfig call.
func TestDeleteCommand_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("command_name"); got != "check_ping" {
				t.Errorf("expected command_name=check_ping in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Command successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteCommand(context.Background(), "check_ping"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
