package nna

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSourceGroup_MarshalJSON_NilSourcesBecomesEmptyArray confirms a nil
// Sources slice (Go's zero value, e.g. a SourceGroup built without
// explicitly setting it) serializes as "sources":[] rather than
// encoding/json's default "sources":null - NNA's validator rejects a null
// sources field outright ("The sources field must be an array.", confirmed
// live) even though it accepts an empty array.
func TestSourceGroup_MarshalJSON_NilSourcesBecomesEmptyArray(t *testing.T) {
	g := SourceGroup{Name: "test"}
	if g.Sources != nil {
		t.Fatalf("expected a zero-value SourceGroup to have nil Sources, got %+v", g.Sources)
	}

	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(b), `"sources":[]`) {
		t.Errorf("expected marshaled body to contain \"sources\":[], got %s", b)
	}
	if strings.Contains(string(b), `"sources":null`) {
		t.Errorf("marshaled body must never send \"sources\":null, got %s", b)
	}
}

// TestNewSourceGroup_SendsBearerAuthAndJSONBody confirms requests use the
// Authorization: Bearer header and a JSON body, and that a "sources" array
// element serializes as a bare {"id": N} rather than the full Source shape.
func TestNewSourceGroup_SendsBearerAuthAndJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/source-groups":
			if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
				t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type header = %q, want application/json", got)
			}
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			if !strings.Contains(string(body), `"sources":[{"id":1}]`) {
				t.Errorf("expected JSON body to contain a bare sources id list, got %s", body)
			}
			_, _ = w.Write([]byte(`{"message":"Source group created successfully."}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/source-groups":
			_, _ = w.Write([]byte(`[{"id":1,"name":"test","description":"d","sources":[{"id":1}]}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSourceGroup(context.Background(), &SourceGroup{Name: "test", Description: "d", Sources: []SourceRef{{ID: 1}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 1 {
		t.Fatalf("expected source group with id 1, got %+v", got)
	}
}

// TestNewSourceGroup_ResolvesIDByListingSinceCreateResponseHasNone confirms
// NewSourceGroup looks the created group up by name, since NNA's create
// response never contains the object or its assigned id (confirmed live).
// It also confirms the newest (highest id) match wins - unlike sources,
// group names aren't validated as unique, so this tiebreak is load bearing
// here, not just a defensive safeguard.
func TestNewSourceGroup_ResolvesIDByListingSinceCreateResponseHasNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"message":"Source group created successfully."}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":4,"name":"mine","sources":[]},{"id":9,"name":"mine","sources":[]}]`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSourceGroup(context.Background(), &SourceGroup{Name: "mine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 9 {
		t.Fatalf("expected the newest 'mine' match with id 9, got %+v", got)
	}
}

// TestNewSourceGroup_PropagatesValidationError confirms a 422 validation
// failure (e.g. an unknown source id) is surfaced as an error rather than
// attempting the post-create name lookup.
func TestNewSourceGroup_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The selected sources.0.id is invalid.","errors":{"sources.0.id":["The selected sources.0.id is invalid."]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewSourceGroup(context.Background(), &SourceGroup{Name: "test", Sources: []SourceRef{{ID: 9999}}})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil source group on validation failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "sources.0.id is invalid") {
		t.Errorf("got error %q, want it to mention the invalid source id", err.Error())
	}
}

// TestGetSourceGroup_Found confirms a successful GET-by-id unmarshals the
// bare object, extracting just the id of each nested source (the full
// nested source object and its "pivot" key are ignored, since only the id
// is needed).
func TestGetSourceGroup_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/source-groups/1" {
			t.Errorf("expected path /api/v1/source-groups/1, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":1,"name":"test","description":"d","sources":[{"id":5,"name":"src","port":9995,"pivot":{"source_group_id":1,"source_id":5}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetSourceGroup(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "test" || got.Description != "d" {
		t.Fatalf("got %+v, want name=test description=d", got)
	}
	if len(got.Sources) != 1 || got.Sources[0].ID != 5 {
		t.Errorf("got Sources=%+v, want a single source ref with id 5", got.Sources)
	}
}

// TestGetSourceGroup_NotFound confirms NNA's 404 comes back as (nil, nil),
// per CLAUDE.md quirk 9.
func TestGetSourceGroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 999"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetSourceGroup(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil source group for a 404, got %+v", got)
	}
}

// TestUpdateSourceGroup_AddressesByIDAndRefetchesSinceResponseHasNoObject
// confirms PUT is addressed by the immutable numeric id and that, unlike
// UpdateSource's {"source": {...}} wrapped response, a source group's PUT
// response carries no object at all - the updated object is fetched with a
// follow-up GET instead.
func TestUpdateSourceGroup_AddressesByIDAndRefetchesSinceResponseHasNoObject(t *testing.T) {
	var sawGet bool
	var putBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/source-groups/5":
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			putBody = string(b)
			_, _ = w.Write([]byte(`{"message":"Source group updated successfully."}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/source-groups/5":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":5,"name":"renamed","description":"d2","sources":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateSourceGroup(context.Background(), 5, &SourceGroup{Name: "renamed", Description: "d2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawGet {
		t.Error("expected a follow-up GET after update, got none")
	}
	if got == nil || got.Name != "renamed" {
		t.Errorf("got %+v, want name=renamed", got)
	}

	// The single trickiest live-confirmed behavior in this whole file: PUT
	// must always carry an explicit "sources" key (see SourceGroup's doc
	// comment) - a caller passing a nil Sources field (as above) must still
	// serialize "sources":[] on the wire, never omit the key or send null,
	// or NNA silently preserves whatever sources were already attached
	// instead of clearing them. This is the one thing at risk of silently
	// regressing (e.g. if omitempty were ever re-added to the Sources
	// field) without a live NNA instance to catch it - pin it down here at
	// the unit level too, not just via the acceptance suite.
	if !strings.Contains(putBody, `"sources":[]`) {
		t.Errorf("expected PUT body to explicitly send \"sources\":[], got %s", putBody)
	}
}

// TestUpdateSourceGroup_SendsPopulatedSourcesExplicitly confirms a non-empty
// Sources list is also always sent on PUT (not just the empty/nil case
// above) - a group with existing members being updated must still carry its
// full desired membership, since (per SourceGroup's doc comment) sources
// isn't a partial-update field the way description is.
func TestUpdateSourceGroup_SendsPopulatedSourcesExplicitly(t *testing.T) {
	var putBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/source-groups/5":
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			putBody = string(b)
			_, _ = w.Write([]byte(`{"message":"Source group updated successfully."}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/source-groups/5":
			_, _ = w.Write([]byte(`{"id":5,"name":"renamed","sources":[{"id":7}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.UpdateSourceGroup(context.Background(), 5, &SourceGroup{Name: "renamed", Sources: []SourceRef{{ID: 7}}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(putBody, `"sources":[{"id":7}]`) {
		t.Errorf("expected PUT body to explicitly carry the populated sources list, got %s", putBody)
	}
}

// TestDeleteSourceGroup_IdempotentOnAlreadyGone confirms a delete of a
// nonexistent id still returns success (with an empty "deleted" array)
// rather than an error - confirmed live.
func TestDeleteSourceGroup_IdempotentOnAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/source-groups/999" {
			t.Errorf("expected DELETE /api/v1/source-groups/999, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Source groups deleted successfully.","deleted":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteSourceGroup(context.Background(), 999); err != nil {
		t.Errorf("expected no error deleting an already-gone id, got %v", err)
	}
}
