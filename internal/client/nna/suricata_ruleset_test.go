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

const rulesetRow = `{"id":34,"name":"tf-a","description":"d1","enabled":1,"url":"https:\/\/example.com\/a.rules","secret_code":"abc","secret_code_required":0,"type":"Custom","Last Updated":"Never","Fetch Status":"None"}`

func TestGetSuricataRuleset_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/suricata/rulesets/34" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(rulesetRow))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "T").GetSuricataRuleset(context.Background(), 34)
	if err != nil || got == nil {
		t.Fatalf("got %+v / %v", got, err)
	}
	if got.ID != 34 || got.Name != "tf-a" || !got.Enabled || got.URL != "https://example.com/a.rules" || got.SecretCode != "abc" || got.Type != "Custom" || got.Description != "d1" {
		t.Errorf("unexpected ruleset: %+v", got)
	}
}

// enabled is a bare 0/1 number on GET but a bool on write; null description
// (what NNA stores for an empty one) must decode cleanly.
func TestGetSuricataRuleset_EnabledZeroAndNullDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":35,"name":"b","description":null,"enabled":0,"url":"u","secret_code":"","secret_code_required":1,"type":"Commercial"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "T").GetSuricataRuleset(context.Background(), 35)
	if err != nil || got == nil {
		t.Fatalf("got %+v / %v", got, err)
	}
	if got.Enabled || got.Description != "" || !got.SecretCodeRequired {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestGetSuricataRuleset_EnabledAsBool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"name":"b","enabled":true,"url":"u"}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "T").GetSuricataRuleset(context.Background(), 1)
	if err != nil || got == nil || !got.Enabled {
		t.Fatalf("got %+v / %v", got, err)
	}
}

func TestGetSuricataRuleset_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 9"}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "T").GetSuricataRuleset(context.Background(), 9)
	if err != nil || got != nil {
		t.Fatalf("want (nil,nil), got %+v / %v", got, err)
	}
}

func TestGetSuricataRuleset_AuthErrorIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "BAD").GetSuricataRuleset(context.Background(), 9)
	if err == nil || got != nil {
		t.Fatalf("want error, got %+v / %v", got, err)
	}
}

// NNA's create response has no id, and omitting "enabled" 500s yet still
// creates the row - so enabled must always be sent, as a bool, and the id is
// found afterwards by name.
func TestNewSuricataRuleset_SendsEnabledAndResolvesIDByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/suricata/rulesets":
			raw, _ := io.ReadAll(r.Body)
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			if b["enabled"] != false {
				t.Errorf("enabled must always be sent (as a bool), body: %s", raw)
			}
			if b["name"] != "tf-a" || b["url"] != "https://example.com/a.rules" {
				t.Errorf("unexpected body: %s", raw)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Ruleset added."}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/suricata/rulesets":
			_, _ = w.Write([]byte(`[{"id":1,"name":"other","enabled":1,"url":"u"},` + rulesetRow + `]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "T").NewSuricataRuleset(context.Background(), &SuricataRuleset{Name: "tf-a", URL: "https://example.com/a.rules"})
	if err != nil || got == nil || got.ID != 34 {
		t.Fatalf("got %+v / %v", got, err)
	}
}

func TestNewSuricataRuleset_DuplicateNameIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Ruleset 'tf-a' already exists."}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "T").NewSuricataRuleset(context.Background(), &SuricataRuleset{Name: "tf-a", URL: "u"})
	if err == nil || got != nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("got %+v / %v", got, err)
	}
}

// PATCH (not PUT: PUT also needs url and silently ignores name) carries every
// mutable field, with description/secret_code sent even when empty so they
// can be cleared.
func TestUpdateSuricataRuleset_PatchesAllMutableFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			if r.URL.Path != "/api/v1/suricata/rulesets/34" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			raw, _ := io.ReadAll(r.Body)
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			for _, k := range []string{"url", "description", "enabled", "secret_code"} {
				if _, ok := b[k]; !ok {
					t.Errorf("PATCH body missing %q: %s", k, raw)
				}
			}
			if _, ok := b["name"]; ok {
				t.Errorf("name is immutable and must not be sent: %s", raw)
			}
			_, _ = w.Write([]byte(`{"message":"Ruleset updated successfully"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(rulesetRow))
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "T").UpdateSuricataRuleset(context.Background(), 34, &SuricataRuleset{Name: "tf-a", URL: "u"})
	if err != nil || got == nil {
		t.Fatalf("got %+v / %v", got, err)
	}
}

func TestUpdateSuricataRuleset_NotFoundIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 34"}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "T").UpdateSuricataRuleset(context.Background(), 34, &SuricataRuleset{})
	if err == nil || got != nil {
		t.Fatalf("got %+v / %v", got, err)
	}
}

// Deletion has no {id} route: it's DELETE rulesets with {"ids":[...]}.
func TestDeleteSuricataRuleset_BulkEndpointWithSingleID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/suricata/rulesets" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.ReplaceAll(string(raw), " ", "") != `{"ids":[34]}` {
			t.Errorf("body = %s, want {\"ids\":[34]}", raw)
		}
		_, _ = w.Write([]byte(`{"message":"All rulesets deleted successfully."}`))
	}))
	defer srv.Close()
	if err := NewClient(srv.URL, "T").DeleteSuricataRuleset(context.Background(), 34); err != nil {
		t.Fatal(err)
	}
}

// An id that no longer exists is rejected as a 422 on "ids.0"; that means
// already deleted.
func TestDeleteSuricataRuleset_AlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The selected ids.0 is invalid.","errors":{"ids.0":["The selected ids.0 is invalid."]}}`))
	}))
	defer srv.Close()
	if err := NewClient(srv.URL, "T").DeleteSuricataRuleset(context.Background(), 34); err != nil {
		t.Fatalf("already-gone should be nil, got %v", err)
	}
}

func TestDeleteSuricataRuleset_OtherErrorsPropagate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()
	if err := NewClient(srv.URL, "T").DeleteSuricataRuleset(context.Background(), 34); err == nil {
		t.Fatal("expected error")
	}
}
