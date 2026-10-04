package nna

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewNmapProfile_SendsBearerAuthAndJSONBody confirms requests use the
// Authorization: Bearer header (not XI's ?apikey= query param) and a JSON
// body carrying the free-form nmap flag string under "parameters".
func TestNewNmapProfile_SendsBearerAuthAndJSONBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nmap/profiles" {
			t.Errorf("expected POST /api/v1/nmap/profiles, got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type header = %q, want application/json", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully","profile":{"user_id":1,"name":"quick","parameters":"-sS -T4 -e eth0","description":"d","tags":["Quick"],"updated_at":"2026-10-03T16:27:29.000000Z","created_at":"2026-10-03T16:27:29.000000Z","id":11}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{
		Name:        "quick",
		Parameters:  "-sS -T4 -e eth0",
		Description: "d",
		Tags:        []string{"Quick"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 11 {
		t.Fatalf("expected profile with id 11, got %+v", got)
	}
	if gotBody["name"] != "quick" {
		t.Errorf("request body name = %v, want %q", gotBody["name"], "quick")
	}
	if gotBody["parameters"] != "-sS -T4 -e eth0" {
		t.Errorf("request body parameters = %v, want %q", gotBody["parameters"], "-sS -T4 -e eth0")
	}
}

// TestNewNmapProfile_UnwrapsProfileKeyWithoutALookup confirms the id is
// read straight out of the create response's "profile" envelope, with no
// list-and-match-by-name follow-up - NNA returns the created object here,
// unlike the sources/source-groups create responses this package's other
// types have to work around.
func TestNewNmapProfile_UnwrapsProfileKeyWithoutALookup(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully","profile":{"id":42,"user_id":1,"name":"mine","parameters":"-sn -e eth0","description":null,"tags":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "mine", Parameters: "-sn -e eth0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 42 || got.Name != "mine" {
		t.Fatalf("got %+v, want id 42 named \"mine\"", got)
	}
	if len(requests) != 1 || requests[0] != "POST /api/v1/nmap/profiles" {
		t.Errorf("expected exactly one POST and no lookup, got %v", requests)
	}
}

// TestNewNmapProfile_SendsTagsAsEmptyArrayNotNull confirms a profile built
// without tags serializes "tags":[] rather than encoding/json's "tags":null
// for a nil slice, so a tag-less create and a clear-all-tags update leave
// the column in the same shape.
func TestNewNmapProfile_SendsTagsAsEmptyArrayNotNull(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully","profile":{"id":1,"name":"n","parameters":"-sn -e eth0","tags":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -e eth0"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), `"tags":[]`) {
		t.Errorf("expected an empty tags array in the request body, got %s", raw)
	}
	if strings.Contains(string(raw), `"tags":null`) {
		t.Errorf("tags must never be serialized as null, got %s", raw)
	}
}

// TestNewNmapProfile_AlwaysSerializesClearableFields confirms description
// and tags are sent even when empty. NNA's PUT preserves omitted fields,
// so omitting either would make it impossible to clear one that was
// previously set.
func TestNewNmapProfile_AlwaysSerializesClearableFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully","profile":{"id":1,"name":"n","parameters":"-sn -e eth0"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -e eth0"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"description", "tags"} {
		if _, ok := gotBody[key]; !ok {
			t.Errorf("expected %q to always be present in the request body, got %v", key, gotBody)
		}
	}
	// id, user_id and times_ran are server-controlled and ignored if
	// sent, so they're omitted from a create body rather than sent as
	// misleading zero values.
	for _, key := range []string{"id", "user_id", "times_ran"} {
		if _, ok := gotBody[key]; ok {
			t.Errorf("expected server-controlled %q to be omitted from the request body, got %v", key, gotBody)
		}
	}
}

// TestNewNmapProfile_PropagatesDuplicateNameValidationError confirms the
// clean 422 POST returns for a name collision (NNA enforces a UNIQUE index
// on nmap_profiles.name) surfaces as an error, not a nil profile and nil
// error.
func TestNewNmapProfile_PropagatesDuplicateNameValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The name has already been taken.","errors":{"name":["The name has already been taken."]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "dupe", Parameters: "-sn -e eth0"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile on validation failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "already been taken") {
		t.Errorf("got error %q, want it to mention the name conflict", err.Error())
	}
}

// TestNewNmapProfile_PropagatesMissingInterfaceFlagError confirms the
// create-only "-e flag" rejection is surfaced. Its body shape differs from
// a Laravel validation failure - a {"message","file","line"} trio with no
// "errors" map - so this exercises parseError's message-only fallback.
func TestNewNmapProfile_PropagatesMissingInterfaceFlagError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The -e flag (interface) is required for Nmap scans.","file":"\/var\/www\/html\/nagiosna\/app\/Services\/NmapService.php","line":0}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -T4"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile, got %+v", got)
	}
	if !strings.Contains(err.Error(), "-e flag") {
		t.Errorf("got error %q, want it to mention the -e flag requirement", err.Error())
	}
}

// TestGetNmapProfile_Found confirms a successful GET-by-id unmarshals the
// bare object (not the "profile" envelope the create/update responses use),
// including the JSON-array tags column and the server-managed times_ran
// counter that the create response omits.
func TestGetNmapProfile_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nmap/profiles/3" {
			t.Errorf("expected path /api/v1/nmap/profiles/3, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":3,"user_id":1,"name":"Ping Scan","parameters":"-sn -e eth0","description":"d","tags":["Ping","Quick"],"times_ran":7,"created_at":"2026-10-03T16:20:33.000000Z","updated_at":"2026-10-03T16:20:33.000000Z"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetNmapProfile(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a profile, got nil")
	}
	if got.Name != "Ping Scan" || got.Parameters != "-sn -e eth0" || got.TimesRan != 7 {
		t.Errorf("got %+v, want name=\"Ping Scan\" parameters=\"-sn -e eth0\" times_ran=7", got)
	}
	if got.UserID == nil || *got.UserID != 1 {
		t.Errorf("got user_id %v, want 1", got.UserID)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "Ping" || got.Tags[1] != "Quick" {
		t.Errorf("got tags %v, want [Ping Quick]", got.Tags)
	}
}

// TestGetNmapProfile_NullUserIDAndTags confirms the two nullable columns
// decode to their zero values rather than failing - NNA's seeded default
// profiles have a null user_id, and a profile whose tags were explicitly
// cleared reads back as null rather than [].
func TestGetNmapProfile_NullUserIDAndTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":5,"user_id":null,"name":"seeded","parameters":"-sn -e <iface>","description":null,"tags":null,"times_ran":0}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetNmapProfile(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a profile, got nil")
	}
	if got.UserID != nil {
		t.Errorf("expected a nil UserID for a null user_id, got %v", *got.UserID)
	}
	if got.Tags != nil {
		t.Errorf("expected nil Tags for a null tags column, got %v", got.Tags)
	}
	if got.Description != "" {
		t.Errorf("expected an empty Description for a null description, got %q", got.Description)
	}
}

// TestGetNmapProfile_NotFound confirms NNA's 404 comes back as (nil, nil)
// rather than an error or a non-nil empty struct, per CLAUDE.md quirk 9 -
// Terraform's state-clearing logic depends on that distinction.
func TestGetNmapProfile_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 999"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetNmapProfile(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil profile for a 404, got %+v", got)
	}
}

// TestUpdateNmapProfile_AddressesByIDAndUnwrapsProfileKey confirms PUT is
// addressed by the immutable numeric id (so a rename is an ordinary field
// update, unlike XI's rename-by-old-name PUT, CLAUDE.md quirk 3) and that
// the updated object is read out of the response envelope rather than via a
// follow-up GET.
func TestUpdateNmapProfile_AddressesByIDAndUnwrapsProfileKey(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"message":"Nmap profile updated successfully","profile":{"id":5,"user_id":1,"name":"renamed","parameters":"-sS -T3 -e eth0","description":"d2","tags":["TCP SYN"],"times_ran":4}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateNmapProfile(context.Background(), 5, &NmapProfile{
		Name:        "renamed",
		Parameters:  "-sS -T3 -e eth0",
		Description: "d2",
		Tags:        []string{"TCP SYN"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "renamed" || got.Parameters != "-sS -T3 -e eth0" || got.TimesRan != 4 {
		t.Errorf("got %+v, want name=renamed parameters=\"-sS -T3 -e eth0\" times_ran=4", got)
	}
	if len(requests) != 1 || requests[0] != "PUT /api/v1/nmap/profiles/5" {
		t.Errorf("expected exactly one PUT and no follow-up read, got %v", requests)
	}
}

// TestUpdateNmapProfile_SendsExplicitEmptyClearableFields confirms a
// cleared description and tag list are serialized as "" and [] rather than
// omitted. PUT preserves omitted fields, so omitting them would silently
// leave the old values in place instead of clearing them.
func TestUpdateNmapProfile_SendsExplicitEmptyClearableFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		_, _ = w.Write([]byte(`{"message":"Nmap profile updated successfully","profile":{"id":5,"name":"n","parameters":"-sn -e eth0","description":null,"tags":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.UpdateNmapProfile(context.Background(), 5, &NmapProfile{Name: "n", Parameters: "-sn -e eth0"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := gotBody["description"]; !ok || got != "" {
		t.Errorf("expected an explicit empty description in the request body, got %v (present=%v)", got, ok)
	}
	tags, ok := gotBody["tags"]
	if !ok {
		t.Fatalf("expected an explicit tags key in the request body, got %v", gotBody)
	}
	if list, isList := tags.([]any); !isList || len(list) != 0 {
		t.Errorf("expected an explicit empty tags array, got %v", tags)
	}
}

// TestUpdateNmapProfile_PropagatesDuplicateNameServerError confirms the
// HTTP 500 raw-MySQL-error PUT returns for a rename collision (it skips the
// validator POST uses and hits the UNIQUE index directly) surfaces as an
// error rather than being mistaken for success.
func TestUpdateNmapProfile_PropagatesDuplicateNameServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"SQLSTATE[23000]: Integrity constraint violation: 1062 Duplicate entry 'Ping Scan' for key 'nmap_profiles.nmap_profiles_name_unique'","file":"\/var\/www\/html\/nagiosna\/vendor\/laravel\/framework\/src\/Illuminate\/Database\/Connection.php","line":0}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateNmapProfile(context.Background(), 5, &NmapProfile{Name: "Ping Scan", Parameters: "-sn -e eth0"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile on a failed update, got %+v", got)
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("got error %q, want it to name the duplicate key", err.Error())
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected an APIError carrying HTTP 500, got %#v", err)
	}
}

// TestUpdateNmapProfile_NotFoundIsAnError confirms a PUT against an id that
// doesn't exist surfaces the 404 as an error. Unlike GetNmapProfile (where
// not-found is a legitimate "it's gone, clear state" signal) and
// DeleteNmapProfile (where it means the desired end state already holds),
// an update that silently did nothing must not look like a success.
func TestUpdateNmapProfile_NotFoundIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 999"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateNmapProfile(context.Background(), 999, &NmapProfile{Name: "n", Parameters: "-sn -e eth0"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile, got %+v", got)
	}
}

// TestListNmapProfiles confirms the collection GET unmarshals a bare array,
// which on a fresh instance holds NNA's nine seeded default profiles.
func TestListNmapProfiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nmap/profiles" {
			t.Errorf("expected path /api/v1/nmap/profiles, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":1,"name":"Intense Scan","parameters":"-T4 -A -v -e <iface>"},{"id":5,"name":"Ping Scan","parameters":"-sn -e <iface>"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.ListNmapProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[1].Name != "Ping Scan" || got[1].ID != 5 {
		t.Errorf("got %+v, want two profiles ending in \"Ping Scan\" (id 5)", got)
	}
}

// TestDeleteNmapProfile_AddressesPerIDRoute confirms DELETE targets the
// per-id route rather than the sibling bulk /nmap/profiles route, which
// deletes many profiles at once from a request body.
func TestDeleteNmapProfile_AddressesPerIDRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/nmap/profiles/4" {
			t.Errorf("expected DELETE /api/v1/nmap/profiles/4, got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Nmap profile deleted successfully"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteNmapProfile(context.Background(), 4); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDeleteNmapProfile_TreatsNotFoundAsSuccess confirms the 404 this route
// returns for an already-gone id is translated to success. Unlike
// DeleteSource/DeleteSourceGroup, NNA is not server-side idempotent here
// (confirmed live), so without this a Terraform destroy of a profile
// already removed out-of-band would fail instead of converging.
func TestDeleteNmapProfile_TreatsNotFoundAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 999"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteNmapProfile(context.Background(), 999); err != nil {
		t.Errorf("expected no error deleting an already-gone id, got %v", err)
	}
}

// TestDeleteNmapProfile_PropagatesOtherFailures confirms the not-found
// tolerance above is narrow: any other non-2xx status still errors, so a
// genuine server-side failure isn't mistaken for a completed delete.
func TestDeleteNmapProfile_PropagatesOtherFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Something broke server-side."}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.DeleteNmapProfile(context.Background(), 4)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "Something broke") {
		t.Errorf("got error %q, want the server message surfaced", err.Error())
	}
}

// TestNewNmapProfile_BareMessageResponseRecoversByName confirms a 2xx
// response carrying only a message - the shape the sibling source-groups
// route actually returns - falls back to a by-name lookup instead of
// erroring. The POST already landed at that point, so an error would leave
// a live profile with no Terraform state tracking it; the next apply would
// fail "The name has already been taken." with manual cleanup the only
// remedy.
func TestNewNmapProfile_BareMessageResponseRecoversByName(t *testing.T) {
	var sawList bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nmap/profiles":
			sawList = true
			_, _ = w.Write([]byte(`[{"id":3,"name":"other","parameters":"-sn -e eth0"},{"id":8,"name":"n","parameters":"-sn -e eth0"}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -e eth0"})
	if err != nil {
		t.Fatalf("expected recovery by name, got error: %v", err)
	}
	if got == nil || got.ID != 8 {
		t.Fatalf("got %+v, want the profile named \"n\" with id 8", got)
	}
	if !sawList {
		t.Error("expected a by-name lookup after the unusable create response")
	}
}

// TestNewNmapProfile_BareMessageResponseWithNoMatchIsAnError confirms the
// recovery above doesn't paper over a create that genuinely didn't happen:
// with no matching profile to adopt, the envelope error is surfaced.
func TestNewNmapProfile_BareMessageResponseWithNoMatchIsAnError(t *testing.T) {
	// This is the one case that exhausts every retry before giving up, so
	// shorten the backoff rather than really sleeping through it.
	defer withFastNmapProfileLookup()()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully"}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -e eth0"})
	if err == nil {
		t.Fatal("expected an error when no profile could be adopted, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile, got %+v", got)
	}
}

// TestUpdateNmapProfile_BareMessageResponseIsAnError is the update-side
// counterpart to the create test above.
func TestUpdateNmapProfile_BareMessageResponseIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"Nmap profile updated successfully"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateNmapProfile(context.Background(), 5, &NmapProfile{Name: "n", Parameters: "-sn -e eth0"})
	if err == nil {
		t.Fatal("expected an error for a response with no profile object, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile, got %+v", got)
	}
}

// TestNewNmapProfile_IDLessProfileRecoversByName covers the envelope being
// present but id-less, which is just as unusable as a missing envelope and
// takes the same by-name recovery.
func TestNewNmapProfile_IDLessProfileRecoversByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Nmap profile created successfully","profile":{"name":"n","parameters":"-sn -e eth0"}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":12,"name":"n","parameters":"-sn -e eth0"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewNmapProfile(context.Background(), &NmapProfile{Name: "n", Parameters: "-sn -e eth0"})
	if err != nil {
		t.Fatalf("expected recovery by name, got error: %v", err)
	}
	if got == nil || got.ID != 12 {
		t.Fatalf("got %+v, want id 12", got)
	}
}

// TestGetNmapProfile_NonProfileSuccessBodyIsAnError confirms a 2xx body
// that isn't a profile object errors instead of decoding to a zero-value
// profile. Without the guard, Read would overwrite good state with id 0
// and empty Required attributes, and the next Update would PUT to
// /nmap/profiles/0 while the real profile drifted untracked.
func TestGetNmapProfile_NonProfileSuccessBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"Nmap profile deleted successfully"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetNmapProfile(context.Background(), 7)
	if err == nil {
		t.Fatal("expected an error for a non-profile success body, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil profile, got %+v", got)
	}
}

// withFastNmapProfileLookup collapses the create-recovery lookup's retry
// budget for the duration of a test, returning a func that restores it.
func withFastNmapProfileLookup() func() {
	attempts, backoff := newNmapProfileLookupAttempts, newNmapProfileLookupBackoff
	newNmapProfileLookupAttempts, newNmapProfileLookupBackoff = 1, 0
	return func() {
		newNmapProfileLookupAttempts, newNmapProfileLookupBackoff = attempts, backoff
	}
}
