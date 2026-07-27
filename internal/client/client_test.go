package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestBuildURL_GET(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodGet, "host_name", "myhost", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/config/host?apikey=TOKEN&host_name=myhost&pretty=1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_GET_MissingName(t *testing.T) {
	if _, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodGet, "host_name", "", "", ""); err == nil {
		t.Error("expected an error when name is empty, got nil")
	}
}

func TestBuildURL_DELETE_StandardObject(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodDelete, "host_name", "myhost", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/config/host?apikey=TOKEN&host_name=myhost"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// authserver's DELETE is a real API inconsistency: a /<name> path segment
// instead of the query-param style every other object type uses.
func TestBuildURL_DELETE_AuthServerUsesPathSegment(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "authserver", http.MethodDelete, "server_id", "3", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/system/authserver/3?apikey=TOKEN&server_id=3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_PUT_RenameUsesOldNameInPath(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodPut, "host_name", "newname", "oldname", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/config/host/oldname?apikey=TOKEN&pretty=1&force=1&"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_PUT_MissingOldVal(t *testing.T) {
	if _, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodPut, "host_name", "newname", "", ""); err == nil {
		t.Error("expected an error when oldVal is empty, got nil")
	}
}

// service is keyed by (name, description) on PUT - a second, real
// compound-key quirk distinct from authserver's.
func TestBuildURL_PUT_ServiceAppendsDescriptionSegment(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "service", http.MethodPut, "config_name", "newsvc", "oldsvc", "old description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/config/service/oldsvc/old description?apikey=TOKEN&pretty=1&force=1&"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_POST_ForceOne(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "host", http.MethodPost, "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/config/host?apikey=TOKEN&force=1&"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_ApplyConfig_NoForceParam(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "applyconfig", http.MethodPost, "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost/nagiosxi/api/v1/system/applyconfig?apikey=TOKEN"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildURL_ApplyConfig_RejectsNonPost(t *testing.T) {
	if _, err := buildURL("http://localhost/nagiosxi", "TOKEN", "applyconfig", http.MethodGet, "", "", "", ""); err == nil {
		t.Error("expected an error for a non-POST applyconfig call, got nil")
	}
}

func TestBuildURL_NoTrailingSlashOnBaseURL(t *testing.T) {
	got, err := buildURL("http://localhost/nagiosxi", "TOKEN", "applyconfig", http.MethodPost, "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[len("http://localhost/nagiosxi"):len("http://localhost/nagiosxi")+len("/api")] != "/api" {
		t.Errorf("expected a leading slash before api/, got %q", got)
	}
}

func TestParseCommandResponse_Success(t *testing.T) {
	err := parseCommandResponse([]byte(`{"success":"Host successfully added"}`))
	if err != nil {
		t.Errorf("expected nil error for a success response, got %v", err)
	}
}

func TestParseCommandResponse_Error(t *testing.T) {
	err := parseCommandResponse([]byte(`{"error":"Does the host exist?"}`))
	if err == nil {
		t.Fatal("expected an error for an error response, got nil")
	}
	if err.Error() != "Does the host exist?" {
		t.Errorf("got error message %q, want %q", err.Error(), "Does the host exist?")
	}
}

func TestParseCommandResponse_Unparseable(t *testing.T) {
	if err := parseCommandResponse([]byte(`not json`)); err == nil {
		t.Error("expected an error for an unparseable body, got nil")
	}
}

func TestExistsErrorFor(t *testing.T) {
	cases := []struct {
		objectType string
		err        error
		want       bool
	}{
		{"host", errors.New("Does the host exist?"), true},
		{"service", errors.New("Does the service exist?"), true},
		{"host", errors.New("Does the service exist?"), false},
		{"host", errors.New("some other error"), false},
		{"host", nil, false},
	}
	for _, c := range cases {
		if got := existsErrorFor(c.objectType, c.err); got != c.want {
			t.Errorf("existsErrorFor(%q, %v) = %v, want %v", c.objectType, c.err, got, c.want)
		}
	}
}

func TestRetryUntilFound_SucceedsOnFirstTry(t *testing.T) {
	calls := 0
	got, err := RetryUntilFound(context.Background(), 4, time.Millisecond, func() (*string, error) {
		calls++
		v := "found"
		return &v, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != "found" {
		t.Errorf("got %v, want \"found\"", got)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryUntilFound_EventuallyFound(t *testing.T) {
	calls := 0
	got, err := RetryUntilFound(context.Background(), 4, time.Millisecond, func() (*string, error) {
		calls++
		if calls < 3 {
			return nil, nil // transient not-found, as if racing Nagios's own eventual consistency
		}
		v := "found"
		return &v, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != "found" {
		t.Errorf("got %v, want \"found\"", got)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

// A real not-found (every attempt returns nil, nil) must come back as
// (nil, nil), not as an error - this is the exact bug the rewrite fixes.
func TestRetryUntilFound_GenuinelyNotFound(t *testing.T) {
	calls := 0
	got, err := RetryUntilFound(context.Background(), 4, time.Millisecond, func() (*string, error) {
		calls++
		return nil, nil
	})
	if err != nil {
		t.Errorf("expected nil error for a genuine not-found, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result for a genuine not-found, got %v", *got)
	}
	if calls != 4 {
		t.Errorf("expected all 4 attempts to be used, got %d", calls)
	}
}

func TestRetryUntilFound_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := RetryUntilFound(context.Background(), 2, time.Millisecond, func() (*string, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("got error %v, want %v", err, wantErr)
	}
}

func TestRetryUntilFound_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, err := RetryUntilFound(ctx, 100, 50*time.Millisecond, func() (*string, error) {
		calls++
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls >= 100 {
		t.Errorf("expected cancellation to cut the retry loop short, got all %d calls", calls)
	}
}

func TestSetURLParams_OmitsEmptyAndZeroValues(t *testing.T) {
	type testObject struct {
		Name   string   `json:"name"`
		Empty  string   `json:"empty,omitempty"`
		Count  int      `json:"count"`
		Zero   int      `json:"zero"`
		Tags   []string `json:"tags"`
		NoTags []string `json:"no_tags"`
	}
	obj := testObject{Name: "myhost", Count: 5, Tags: []string{"a", "b"}}

	got := setURLParams(&obj)

	if got.Get("name") != "myhost" {
		t.Errorf("name = %q, want %q", got.Get("name"), "myhost")
	}
	if got.Has("empty") {
		t.Error("expected empty string field to be omitted")
	}
	if got.Get("count") != "5" {
		t.Errorf("count = %q, want %q", got.Get("count"), "5")
	}
	if got.Has("zero") {
		t.Error("expected zero-value int field to be omitted")
	}
	if got.Get("tags") != "a,b" {
		t.Errorf("tags = %q, want %q", got.Get("tags"), "a,b")
	}
	if got.Has("no_tags") {
		t.Error("expected empty slice field to be omitted")
	}
}

func TestSetURLParams_FreeVariablesBecomeTopLevelParams(t *testing.T) {
	type testObject struct {
		FreeVariables map[string]string `json:"free_variables"`
	}
	obj := testObject{FreeVariables: map[string]string{"_SNMPCOMMUNITY": "public"}}

	got := setURLParams(&obj)

	if got.Get("_SNMPCOMMUNITY") != "public" {
		t.Errorf("_SNMPCOMMUNITY = %q, want %q", got.Get("_SNMPCOMMUNITY"), "public")
	}
	if got.Has("free_variables") {
		t.Error("expected free_variables itself not to appear as a literal param name")
	}
}

func TestBoolToNagiosString(t *testing.T) {
	if boolToNagiosString(true) != "1" {
		t.Error("expected true -> \"1\"")
	}
	if boolToNagiosString(false) != "0" {
		t.Error("expected false -> \"0\"")
	}
}

// Nagios returns free variables as dynamic top-level keys on the object
// itself, not nested under a "free_variables" key - confirmed against a live
// Nagios XI instance's actual GET response shape.
func TestExtractFreeVariables(t *testing.T) {
	raw := map[string]json.RawMessage{
		"host_name":      json.RawMessage(`"myhost"`),
		"address":        json.RawMessage(`"127.0.0.1"`),
		"_SNMPCOMMUNITY": json.RawMessage(`"public"`),
		"_test":          json.RawMessage(`"test123"`),
		"register":       json.RawMessage(`"1"`),
	}

	got := extractFreeVariables(raw)

	if len(got) != 2 {
		t.Fatalf("got %d free variables, want 2: %v", len(got), got)
	}
	if got["_SNMPCOMMUNITY"] != "public" {
		t.Errorf("_SNMPCOMMUNITY = %q, want %q", got["_SNMPCOMMUNITY"], "public")
	}
	if got["_test"] != "test123" {
		t.Errorf("_test = %q, want %q", got["_test"], "test123")
	}
	if _, ok := got["host_name"]; ok {
		t.Error("expected non-underscore-prefixed keys to be excluded")
	}
}

func TestExtractFreeVariables_None(t *testing.T) {
	raw := map[string]json.RawMessage{
		"host_name": json.RawMessage(`"myhost"`),
	}
	if got := extractFreeVariables(raw); got != nil {
		t.Errorf("expected nil for no free variables, got %v", got)
	}
}

func TestMapArrayToString(t *testing.T) {
	if got := mapArrayToString([]string{"a", "b", "c"}); got != "a,b,c" {
		t.Errorf("got %q, want %q", got, "a,b,c")
	}
	if got := mapArrayToString(nil); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
