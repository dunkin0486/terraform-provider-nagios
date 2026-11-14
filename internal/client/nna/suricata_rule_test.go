package nna

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const ruleJSON = `{"action":"alert","protocol":"tcp","source ip":"any","source port":"any","direction":"outbound","destination ip":"any","destination port":"80","status":"Active","options":{"msg":"tf test","sid":"9000001","rev":"1","content":"evil"}}`

func TestGetSuricataRule_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/suricata/rules/9000001" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(ruleJSON))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").GetSuricataRule(context.Background(), 9000001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a rule, got nil")
	}
	if got.SID != 9000001 || got.Msg != "tf test" || got.DestinationPort != "80" || got.SourceIP != "any" || got.Status != "Active" {
		t.Errorf("unexpected rule: %+v", got)
	}
	// msg/sid are lifted out of options into dedicated fields.
	if _, ok := got.Options["msg"]; ok {
		t.Errorf("msg should not remain in Options: %v", got.Options)
	}
	if _, ok := got.Options["sid"]; ok {
		t.Errorf("sid should not remain in Options: %v", got.Options)
	}
	if got.Options["rev"] != "1" || got.Options["content"] != "evil" {
		t.Errorf("unexpected Options: %v", got.Options)
	}
}

// NNA answers a missing sid with 404 {"message":"SID not found in rules"}.
func TestGetSuricataRule_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"SID not found in rules"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").GetSuricataRule(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a 404, got %+v", got)
	}
}

func TestGetSuricataRule_ErrorIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "BAD").GetSuricataRule(context.Background(), 1)
	if err == nil || got != nil {
		t.Fatalf("expected an error and nil rule, got %+v / %v", got, err)
	}
}

func sampleRule() *SuricataRule {
	return &SuricataRule{
		SID: 9000001, Action: "alert", Protocol: "tcp",
		SourceIP: "any", SourcePort: "any", Direction: "outbound",
		DestinationIP: "any", DestinationPort: "80", Status: "Active",
		Msg: "tf test", Options: map[string]string{"rev": "1", "content": "evil"},
	}
}

// TestNewSuricataRule_RefusesExistingSID confirms Create guards against NNA's
// POST blindly appending a duplicate line for an sid that already exists
// (confirmed live: POST never checks for conflicts, unlike PUT's 409).
func TestNewSuricataRule_RefusesExistingSID(t *testing.T) {
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posted = true
		}
		_, _ = w.Write([]byte(ruleJSON))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").NewSuricataRule(context.Background(), sampleRule())
	if err == nil {
		t.Fatal("expected an error for an already-existing sid")
	}
	if got != nil || posted {
		t.Errorf("must not POST or return a rule when the sid already exists (posted=%v, got=%+v)", posted, got)
	}
}

func TestNewSuricataRule_PostBodyAndReadBack(t *testing.T) {
	created := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && !created:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"SID not found in rules"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/suricata/rules":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("bad JSON: %v", err)
			}
			// NNA's validator requires string (not number) sids.
			if body["original_sid"] != "9000001" {
				t.Errorf("original_sid = %v, want string \"9000001\"", body["original_sid"])
			}
			nr := body["new_rule"].(map[string]any)
			if nr["source ip"] != "any" || nr["destination port"] != "80" || nr["status"] != "Active" {
				t.Errorf("unexpected new_rule: %v", nr)
			}
			opts := nr["options"].(map[string]any)
			if opts["msg"] != "tf test" || opts["sid"] != "9000001" || opts["content"] != "evil" {
				t.Errorf("unexpected options: %v", opts)
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"New rule added successfully."}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(ruleJSON))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").NewSuricataRule(context.Background(), sampleRule())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.SID != 9000001 {
		t.Fatalf("unexpected rule: %+v", got)
	}
}

func TestNewSuricataRule_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"SID not found in rules"}`))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The selected new rule.action is invalid.","errors":{"new_rule.action":["The selected new rule.action is invalid."]}}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").NewSuricataRule(context.Background(), sampleRule())
	if err == nil || got != nil {
		t.Fatalf("expected error and nil rule, got %+v / %v", got, err)
	}
	if !strings.Contains(err.Error(), "action is invalid") {
		t.Errorf("error %q should surface the validation message", err)
	}
}

// TestUpdateSuricataRule_PutsOriginalSID confirms the PUT addresses the
// existing rule through original_sid in the body (PUT rules/{sid} is a 500
// "method not supported" route - confirmed live).
func TestUpdateSuricataRule_PutsOriginalSID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.Path != "/api/v1/suricata/rules" {
				t.Errorf("PUT path = %s, want /api/v1/suricata/rules", r.URL.Path)
			}
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"original_sid":"9000001"`) {
				t.Errorf("body missing original_sid: %s", raw)
			}
			_, _ = w.Write([]byte(`{"message":"Rule updated successfully."}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(ruleJSON))
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").UpdateSuricataRule(context.Background(), sampleRule())
	if err != nil || got == nil {
		t.Fatalf("unexpected result: %+v / %v", got, err)
	}
}

// NNA's PUT answers 200 "updated successfully" even when the original sid
// doesn't exist (confirmed live), so a rule that vanished must be detected
// by the read-back, not trusted from the PUT response.
func TestUpdateSuricataRule_ErrorsIfRuleMissingAfterPut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"message":"Rule updated successfully."}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"SID not found in rules"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").UpdateSuricataRule(context.Background(), sampleRule())
	if err == nil || got != nil {
		t.Fatalf("expected an error and nil rule, got %+v / %v", got, err)
	}
}

func TestDeleteSuricataRule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/suricata/rules/9000001" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Deleted rules successfully."}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "TOKEN").DeleteSuricataRule(context.Background(), 9000001); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A missing sid is answered 404 "No matching SIDs found in rule file" -
// deleting something already gone is a success for Terraform's purposes.
func TestDeleteSuricataRule_AlreadyGoneIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No matching SIDs found in rule file"}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "TOKEN").DeleteSuricataRule(context.Background(), 1); err != nil {
		t.Fatalf("a 404 on delete should be tolerated, got %v", err)
	}
}

func TestDeleteSuricataRule_ServerErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "TOKEN").DeleteSuricataRule(context.Background(), 1); err == nil {
		t.Fatal("expected an error")
	}
}
