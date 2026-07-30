package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewTimeperiod_PostsFormEncodedParams verifies NewTimeperiod sends the
// timeperiod's fields as a form-urlencoded POST body and follows up with the
// applyconfig call every mutating method requires.
func TestNewTimeperiod_PostsFormEncodedParams(t *testing.T) {
	var sawCreate, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/timeperiod":
			sawCreate = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse create form body: %v", err)
			}
			if got := r.PostFormValue("timeperiod_name"); got != "business_hours" {
				t.Errorf("timeperiod_name = %q, want %q", got, "business_hours")
			}
			if got := r.PostFormValue("monday"); got != "09:00-17:00" {
				t.Errorf("monday = %q, want %q", got, "09:00-17:00")
			}
			if r.URL.Query().Get("force") != "1" {
				t.Errorf("expected force=1 on create, got query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":"Added business_hours to the system."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.NewTimeperiod(context.Background(), &Timeperiod{Name: "business_hours", Alias: "Business Hours", Monday: "09:00-17:00"})
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

// TestGetTimeperiod_Found confirms a successful GET unmarshals the weekday
// fields and the "use"/"exclude" list fields.
func TestGetTimeperiod_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("timeperiod_name") != "business_hours" {
			t.Errorf("expected timeperiod_name=business_hours in query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"timeperiod_name":"business_hours","alias":"Business Hours","monday":"09:00-17:00","exclude":["us-holidays"]}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetTimeperiod(context.Background(), "business_hours")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil timeperiod, got nil")
	}
	if got.Name != "business_hours" || got.Monday != "09:00-17:00" {
		t.Errorf("got %+v, want timeperiod_name=business_hours monday=09:00-17:00", got)
	}
	if len(got.Exclude) != 1 || got.Exclude[0] != "us-holidays" {
		t.Errorf("Exclude = %#v, want [us-holidays]", got.Exclude)
	}
}

// TestGetTimeperiod_UseAsCommaJoinedString confirms Templates unmarshals
// correctly when Nagios sends "use" as a plain comma-joined string rather
// than a JSON array - the real wire shape confirmed against a live
// instance, for both a single template and multiple templates (unlike
// host/service/contact's "use", which always comes back as an array even
// for one value). See Timeperiod.UnmarshalJSON.
func TestGetTimeperiod_UseAsCommaJoinedString(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want []string
	}{
		{name: "single value", wire: `"24x7"`, want: []string{"24x7"}},
		{name: "multiple values", wire: `"24x7,workhours"`, want: []string{"24x7", "workhours"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`[{"timeperiod_name":"business_hours","alias":"Business Hours","use":` + tt.wire + `}]`))
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "TOKEN")
			got, err := c.GetTimeperiod(context.Background(), "business_hours")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected a non-nil timeperiod, got nil")
			}
			if len(got.Templates) != len(tt.want) {
				t.Fatalf("Templates = %#v, want %#v", got.Templates, tt.want)
			}
			for i := range tt.want {
				if got.Templates[i] != tt.want[i] {
					t.Errorf("Templates[%d] = %q, want %q", i, got.Templates[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetTimeperiod_UseAsArray confirms Templates still unmarshals correctly
// if Nagios ever does send "use" as a JSON array (the shape every other
// object type uses), so Timeperiod.UnmarshalJSON handles both possible
// shapes, not just the comma-joined-string one this object type happens to
// send today.
func TestGetTimeperiod_UseAsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"timeperiod_name":"business_hours","alias":"Business Hours","use":["24x7","workhours"]}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetTimeperiod(context.Background(), "business_hours")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil timeperiod, got nil")
	}
	want := []string{"24x7", "workhours"}
	if len(got.Templates) != len(want) {
		t.Fatalf("Templates = %#v, want %#v", got.Templates, want)
	}
	for i := range want {
		if got.Templates[i] != want[i] {
			t.Errorf("Templates[%d] = %q, want %q", i, got.Templates[i], want[i])
		}
	}
}

// TestGetTimeperiod_NotFound confirms an empty result set comes back as
// (nil, nil), per CLAUDE.md quirk 9.
func TestGetTimeperiod_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetTimeperiod(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil timeperiod for an empty result set, got %+v", got)
	}
}

// TestUpdateTimeperiod_RealAPIResponseIsUnparseable pins down a real,
// confirmed-against-a-live-instance quirk: Nagios's PUT for timeperiod
// leaks a PHP print_r() debug dump before the JSON body, so the response
// fails JSON parsing rather than reporting a clean success or error. This is
// why every attribute in resource_timeperiod.go's schema is RequiresReplace
// - Update is never actually reachable via Terraform in practice. It also
// asserts the error UpdateTimeperiod actually returns explains that this is
// a known, permanent Nagios quirk rather than reading like a provider bug -
// see UpdateTimeperiod's error-wrapping.
func TestUpdateTimeperiod_RealAPIResponseIsUnparseable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		_, _ = w.Write([]byte("Array\n(\n    [type] => 9\n    [timeperiod_name] => business_hours\n)\n{\"success\":\"Updated business_hours in the system.\"}"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateTimeperiod(context.Background(), &Timeperiod{Name: "business_hours", Alias: "Business Hours"}, "business_hours")
	if err == nil {
		t.Fatal("expected an error surfacing the unparseable response body, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "known, permanent no-op") {
		t.Errorf("error = %q, want it to explain this is a known Nagios quirk, not just surface the raw parse failure", got)
	}
}

// TestUpdateTimeperiod_FallsBackToNewTimeperiodOnDoesNotExist exercises the
// existsErrorFor fallback (CLAUDE.md quirk 11) for structural parity with
// every other UpdateX method's tests. This mocked response shape - a clean
// {"error": "Does the timeperiod exist?"} body - is not one the real Nagios
// API actually returns for timeperiod's PUT (see
// TestUpdateTimeperiod_RealAPIResponseIsUnparseable and the Timeperiod doc
// comment: the real response never parses as JSON in the first place, so
// existsErrorFor never gets a chance to match). The fallback branch is
// therefore dead code against the live API for this object type specifically
// - this test only confirms the branch behaves correctly if it were ever
// reached, not that it's reachable in practice.
func TestUpdateTimeperiod_FallsBackToNewTimeperiodOnDoesNotExist(t *testing.T) {
	var sawCreate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"error":"Does the timeperiod exist?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/timeperiod":
			sawCreate = true
			_, _ = w.Write([]byte(`{"success":"Added business_hours to the system."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateTimeperiod(context.Background(), &Timeperiod{Name: "business_hours", Alias: "Business Hours"}, "goneaway")
	if err != nil {
		t.Fatalf("expected the fallback to NewTimeperiod to succeed, got error: %v", err)
	}
	if !sawCreate {
		t.Error("expected UpdateTimeperiod to fall back to a create request, got none")
	}
}

// TestUpdateTimeperiod_PropagatesOtherErrors confirms an unrelated PUT
// failure is surfaced (wrapped with context, but still inspectable via
// errors.Is/errors.As through %w) rather than silently swallowed into a
// fallback create - the negative counterpart to
// TestUpdateTimeperiod_FallsBackToNewTimeperiodOnDoesNotExist, mirroring
// TestUpdateHost_PropagatesOtherErrors in host_test.go.
func TestUpdateTimeperiod_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"error":"Some other failure"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.UpdateTimeperiod(context.Background(), &Timeperiod{Name: "business_hours", Alias: "Business Hours"}, "business_hours")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "Some other failure") {
		t.Errorf("got error %q, want it to still contain the underlying %q", err.Error(), "Some other failure")
	}
}

// TestDeleteTimeperiod_AppliesConfig confirms DeleteTimeperiod issues a
// DELETE and, per CLAUDE.md quirk 2, still follows up with an applyconfig
// call.
func TestDeleteTimeperiod_AppliesConfig(t *testing.T) {
	var sawDelete, sawApplyConfig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			sawDelete = true
			if got := r.URL.Query().Get("timeperiod_name"); got != "business_hours" {
				t.Errorf("expected timeperiod_name=business_hours in query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"success":"Timeperiod successfully deleted"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/applyconfig":
			sawApplyConfig = true
			_, _ = w.Write([]byte(`{"success":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteTimeperiod(context.Background(), "business_hours"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDelete {
		t.Error("expected a delete request, got none")
	}
	if !sawApplyConfig {
		t.Error("expected an applyconfig request after delete, got none")
	}
}
