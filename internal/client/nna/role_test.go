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

// TestRole_MarshalJSON_NilTraceroutePermissionsBecomesEmptyObject confirms a
// nil TraceroutePermissions serializes as an empty object rather than
// encoding/json's default null (or being omitted entirely). Both of those
// forms hit a raw MySQL 500 server-side - the roles table's
// traceroute_permissions column is NOT NULL with no default, and NNA's
// validator never catches it (confirmed live on both POST and PUT).
func TestRole_MarshalJSON_NilTraceroutePermissionsBecomesEmptyObject(t *testing.T) {
	r := Role{Name: "test"}
	if r.TraceroutePermissions != nil {
		t.Fatalf("expected a zero-value Role to have nil TraceroutePermissions, got %+v", r.TraceroutePermissions)
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"traceroute_permissions":{`) {
		t.Errorf("expected traceroute_permissions to serialize as an object, got %s", got)
	}
	if strings.Contains(got, `"traceroute_permissions":null`) {
		t.Errorf("must never send traceroute_permissions:null (raw MySQL 500 server-side), got %s", got)
	}
}

// TestRole_MarshalJSON_SendsExplicitNullForUnsetGroups confirms the five
// nullable permission groups are sent as an explicit null when unset, never
// omitted. NNA's PUT is a partial update, so an omitted group keeps whatever
// was already stored - omitting them would make clearing a group impossible
// and silently leave stale permissions in force (confirmed live, and caught
// by TestAccNNARoleUpdatePermissions failing before this was fixed).
func TestRole_MarshalJSON_SendsExplicitNullForUnsetGroups(t *testing.T) {
	b, err := json.Marshal(Role{Name: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(b)
	for _, key := range []string{
		"flow_source_permissions",
		"report_permissions",
		"suricata_permissions",
		"wireshark_permissions",
		"nmap_permissions",
	} {
		if !strings.Contains(got, `"`+key+`":null`) {
			t.Errorf("expected %s to be sent as an explicit null when unset, got %s", key, got)
		}
	}
}

// TestRole_MarshalJSON_OmitsReadOnlyAndSendsFalseBools confirms the booleans
// inside a set permission group are always sent (false is a meaningful,
// distinct value from "not configured"), while empty verb lists are omitted.
func TestRole_MarshalJSON_OmitsEmptyVerbListsButSendsFalseBools(t *testing.T) {
	b, err := json.Marshal(Role{
		Name:                  "test",
		FlowSourcePermissions: &FlowSourcePermissions{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"start_stop_sources":false`) {
		t.Errorf("expected start_stop_sources:false to be sent explicitly, got %s", got)
	}
	if strings.Contains(got, `"sources"`) {
		t.Errorf("expected an empty sources verb list to be omitted, got %s", got)
	}
}

// TestRole_UnmarshalJSON_EmptyArrayPermissionsAreSetButEmpty confirms the
// empty JSON ARRAY NNA echoes back for a permission group stored as an empty
// object decodes to a non-nil zero struct, not an error and not nil. PHP's
// json_encode renders an empty associative array as [], so a role created
// with "traceroute_permissions":{} always reads back as
// "traceroute_permissions":[] (confirmed live) - decoding that straight into
// a struct would otherwise fail with "cannot unmarshal array into Go value".
func TestRole_UnmarshalJSON_EmptyArrayPermissionsAreSetButEmpty(t *testing.T) {
	var r Role
	if err := json.Unmarshal([]byte(`{"id":3,"name":"n","traceroute_permissions":[],"flow_source_permissions":[]}`), &r); err != nil {
		t.Fatalf("unexpected error decoding an empty-array permission group: %v", err)
	}
	if r.TraceroutePermissions == nil {
		t.Error("expected traceroute_permissions:[] to decode as set-but-empty (non-nil), got nil")
	}
	if r.FlowSourcePermissions == nil {
		t.Error("expected flow_source_permissions:[] to decode as set-but-empty (non-nil), got nil")
	}
}

// TestRole_UnmarshalJSON_NullPermissionsStayNil confirms NNA's SQL NULL
// ("never configured") stays distinguishable from the empty-array
// set-but-empty form above - the two really are different states server-side
// (confirmed live: an omitted group reads back null, an empty one reads back
// []), so the provider can round-trip an unset block without a permadiff.
func TestRole_UnmarshalJSON_NullPermissionsStayNil(t *testing.T) {
	var r Role
	if err := json.Unmarshal([]byte(`{"id":3,"name":"n","traceroute_permissions":null,"nmap_permissions":null}`), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TraceroutePermissions != nil {
		t.Errorf("expected traceroute_permissions:null to stay nil, got %+v", r.TraceroutePermissions)
	}
	if r.NmapPermissions != nil {
		t.Errorf("expected nmap_permissions:null to stay nil, got %+v", r.NmapPermissions)
	}
}

// TestRole_UnmarshalJSON_PopulatedGroupsDecode confirms a fully populated
// permission payload decodes into the typed nested structs.
func TestRole_UnmarshalJSON_PopulatedGroupsDecode(t *testing.T) {
	var r Role
	body := `{"id":1,"name":"Admin","type":"admin","protected":true,
		"flow_source_permissions":{"sources":["get","put"],"start_stop_sources":true},
		"report_permissions":{"reports":["get"],"report_history":["get","put"]},
		"traceroute_permissions":{"ncpa_host":["get"],"traceroutes":["get","post"],"scheduled_scans":["get"]},
		"suricata_permissions":{"data":["get"],"rules":["get"],"rulesets":["get"],"alerts":true,"config":false,"scan_pcap":true,"start_stop_scan":false},
		"wireshark_permissions":{"pcaps":["get"],"ring_buffer":["put"],"start_stop_capture":true,"start_stop_ring_buffer":false},
		"nmap_permissions":{"scans":["get"],"ndiffs":["get"],"profiles":["get"],"scheduled_scans":["get"]}}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Type != "admin" || !r.Protected {
		t.Errorf("expected type=admin protected=true, got type=%q protected=%v", r.Type, r.Protected)
	}
	if r.FlowSourcePermissions == nil || !r.FlowSourcePermissions.StartStopSources {
		t.Errorf("flow_source_permissions did not decode: %+v", r.FlowSourcePermissions)
	}
	if r.TraceroutePermissions == nil || len(r.TraceroutePermissions.Traceroutes) != 2 {
		t.Errorf("traceroute_permissions did not decode: %+v", r.TraceroutePermissions)
	}
	if r.SuricataPermissions == nil || !r.SuricataPermissions.ScanPCAP || r.SuricataPermissions.StartStopScan {
		t.Errorf("suricata_permissions did not decode: %+v", r.SuricataPermissions)
	}
	if r.WiresharkPermissions == nil || !r.WiresharkPermissions.StartStopCapture {
		t.Errorf("wireshark_permissions did not decode: %+v", r.WiresharkPermissions)
	}
	if r.NmapPermissions == nil || len(r.NmapPermissions.ScheduledScans) != 1 {
		t.Errorf("nmap_permissions did not decode: %+v", r.NmapPermissions)
	}
}

// TestRole_UnmarshalJSON_IgnoresEmbeddedUsers confirms the "users" array NNA
// embeds in every role GET response is dropped on the floor. Each element is
// a full user object including that user's plaintext "apikey", so decoding it
// into Role would pull other accounts' API credentials into this provider's
// state file and CLI output. Role deliberately has no field for it.
func TestRole_UnmarshalJSON_IgnoresEmbeddedUsers(t *testing.T) {
	body := `{"id":1,"name":"Admin","traceroute_permissions":[],"users":[{"id":1,"username":"nagiosadmin","apikey":"1|SUPERSECRET"}]}`
	var r Role
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Re-encoding must not carry the embedded users (or their apikeys) back
	// out, which is what would leak them into state.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(b), "SUPERSECRET") || strings.Contains(string(b), "users") {
		t.Errorf("Role must never carry the embedded users array, got %s", b)
	}
}

// TestNewRole_SendsBearerAuthAndJSONBody confirms requests use the
// Authorization: Bearer header and a JSON body.
func TestNewRole_SendsBearerAuthAndJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/roles":
			if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
				t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type header = %q, want application/json", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"name":"ops"`) {
				t.Errorf("expected the role name in the JSON body, got %s", body)
			}
			if !strings.Contains(string(body), `"traceroute_permissions":`) {
				t.Errorf("expected traceroute_permissions to always be sent, got %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Role created successfully"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/roles":
			_, _ = w.Write([]byte(`[{"id":7,"name":"ops","type":"custom","protected":false,"traceroute_permissions":[]}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewRole(context.Background(), &Role{Name: "ops"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Fatalf("expected role with id 7, got %+v", got)
	}
}

// TestNewRole_ResolvesIDByListingSinceCreateResponseHasNone confirms NewRole
// discovers the assigned id by listing and matching on name - NNA's create
// response is only {"message":"Role created successfully"} with no object and
// no id (confirmed live), and unlike users there's no "role_id" to read back.
// The newest (highest id) match wins as a defensive tiebreak, the same way
// getSourceByName does.
func TestNewRole_ResolvesIDByListingSinceCreateResponseHasNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Role created successfully"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":4,"name":"mine","traceroute_permissions":[]},{"id":9,"name":"mine","traceroute_permissions":[]},{"id":5,"name":"other","traceroute_permissions":[]}]`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewRole(context.Background(), &Role{Name: "mine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 9 {
		t.Fatalf("expected the newest 'mine' match with id 9, got %+v", got)
	}
}

// TestNewRole_PropagatesValidationError confirms a 422 (e.g. a duplicate
// name, which NNA's validator does enforce as unique) surfaces as an error
// rather than proceeding to the post-create name lookup.
func TestNewRole_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected no lookup after a failed create, got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The name has already been taken.","errors":{"name":["The name has already been taken."]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewRole(context.Background(), &Role{Name: "Admin"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil role on validation failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "already been taken") {
		t.Errorf("got error %q, want it to mention the duplicate name", err.Error())
	}
}

// TestGetRole_FiltersListSinceNoGetByIDRouteExists is the central quirk of
// this type: NNA has NO get-a-role-by-id route at all. GET /api/v1/roles/{x}
// addresses roles by TYPE, not id ("admin"/"user"), and returns only the
// first role of that type - so every custom role shares type "custom" and is
// unreachable that way (confirmed live: GET /roles/5 404s for an existing
// id 5). GetRole therefore lists all roles and filters client-side, and must
// never request an id path segment.
func TestGetRole_FiltersListSinceNoGetByIDRouteExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/roles" {
			t.Errorf("expected the unfiltered list path /api/v1/roles, got %s (there is no get-by-id route)", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":1,"name":"Admin","type":"admin","protected":true,"traceroute_permissions":[]},{"id":5,"name":"ops","type":"custom","protected":false,"traceroute_permissions":{"traceroutes":["get"]}}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetRole(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected to find role id 5, got nil")
	}
	if got.Name != "ops" {
		t.Errorf("got role %q, want ops", got.Name)
	}
	if got.TraceroutePermissions == nil || len(got.TraceroutePermissions.Traceroutes) != 1 {
		t.Errorf("expected the matched role's permissions, got %+v", got.TraceroutePermissions)
	}
}

// TestGetRole_NotFoundReturnsNilNil confirms an id absent from the list
// returns (nil, nil) per this repo's GetX convention (CLAUDE.md quirk 9),
// so Terraform clears state instead of erroring.
func TestGetRole_NotFoundReturnsNilNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"Admin","traceroute_permissions":[]}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetRole(context.Background(), 404)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected (nil, nil) for a missing id, got %+v", got)
	}
}

// TestGetRole_PropagatesListError confirms an API failure on the underlying
// list is surfaced as an error rather than being flattened into the
// indistinguishable "no role with that id" (nil, nil) result.
func TestGetRole_PropagatesListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"This action is unauthorized."}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetRole(context.Background(), 5)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil role on error, got %+v", got)
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("got error %q, want it to mention the authorization failure", err.Error())
	}
}

// TestUpdateRole_RefetchesSinceResponseCarriesNoObject confirms UpdateRole
// addresses the role by its numeric id (PUT /roles/{id} - unlike GET, PUT
// really is id-addressed) and fetches the updated object afterward, since the
// PUT response is only {"message":"Role updated successfully"}.
func TestUpdateRole_RefetchesSinceResponseCarriesNoObject(t *testing.T) {
	var sawPut bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/roles/5":
			sawPut = true
			_, _ = w.Write([]byte(`{"message":"Role updated successfully"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/roles":
			_, _ = w.Write([]byte(`[{"id":5,"name":"renamed","type":"custom","protected":false,"traceroute_permissions":[]}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateRole(context.Background(), 5, &Role{Name: "renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawPut {
		t.Error("expected a PUT to /api/v1/roles/5")
	}
	if got == nil || got.Name != "renamed" {
		t.Fatalf("expected the refetched role, got %+v", got)
	}
}

// TestUpdateRole_NotFoundUsesErrorKeyNotMessage confirms a PUT against a
// missing id surfaces as an error. Confirmed live: this response uses a
// top-level "error" key ({"error":"Role not found"}), unlike the GET and
// DELETE not-found responses which use "message" - parseError prefers
// "error", so both shapes report usefully.
func TestUpdateRole_NotFoundUsesErrorKeyNotMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Role not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateRole(context.Background(), 99999, &Role{Name: "ghost"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil role, got %+v", got)
	}
	if !strings.Contains(err.Error(), "Role not found") {
		t.Errorf("got error %q, want it to surface the error-key message", err.Error())
	}
}

// TestDeleteRole_Success confirms a successful delete by id.
func TestDeleteRole_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/roles/5" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":"Role deleted"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteRole(context.Background(), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDeleteRole_RepeatDeleteIsNotIdempotent confirms a delete of an
// already-gone id surfaces as an error. Unlike DeleteSource/DeleteSourceGroup
// (which both return 200 on a repeat delete), roles return 404
// {"message":"Role not found"} - confirmed live - so callers must not assume
// a repeat delete is harmless.
func TestDeleteRole_RepeatDeleteIsNotIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Role not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.DeleteRole(context.Background(), 5)
	if err == nil {
		t.Fatal("expected an error on a repeat delete, got nil")
	}
	if !strings.Contains(err.Error(), "Role not found") {
		t.Errorf("got error %q, want it to mention the missing role", err.Error())
	}
}

// TestDeleteRole_ProtectedRoleIsRefused confirms NNA's 403 "Role protected"
// response for the two built-in roles (Admin id 1, User id 2) surfaces as an
// error rather than being mistaken for a successful delete - confirmed live.
func TestDeleteRole_ProtectedRoleIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Role protected"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.DeleteRole(context.Background(), 2)
	if err == nil {
		t.Fatal("expected an error deleting a protected role, got nil")
	}
	if !strings.Contains(err.Error(), "Role protected") {
		t.Errorf("got error %q, want it to mention the protected role", err.Error())
	}
}

// TestListRoles_DecodesBuiltInsAndCustom confirms the list decodes both the
// protected built-ins and a custom role.
func TestListRoles_DecodesBuiltInsAndCustom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"Admin","type":"admin","protected":true,"traceroute_permissions":{"traceroutes":["get","post","delete"]}},{"id":2,"name":"User","type":"user","protected":true,"traceroute_permissions":{"ncpa_host":[]}},{"id":3,"name":"ops","type":"custom","protected":false,"traceroute_permissions":[]}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(got))
	}
	if !got[0].Protected || got[0].Type != "admin" {
		t.Errorf("expected the built-in Admin role to decode as protected/admin, got %+v", got[0])
	}
	if got[2].Protected || got[2].Type != "custom" {
		t.Errorf("expected the custom role to decode as unprotected/custom, got %+v", got[2])
	}
}

// TestDeleteRole_NotFoundErrorExposesStatusCode documents that the 404 from a
// repeat delete surfaces as an *APIError carrying StatusCode 404, rather than
// a bare error that loses the code. Note the provider's Delete deliberately
// does NOT branch on this: 403 is ambiguous for roles ("Role protected" vs.
// an already-gone role on some versions), so it re-queries with GetRole
// instead - see resource_nna_role.go's Delete. This test pins the client-side
// contract only.
func TestDeleteRole_NotFoundErrorExposesStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Role not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.DeleteRole(context.Background(), 5)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

// TestRole_UnmarshalJSON_MalformedGroupDoesNotFailTheDecode confirms a
// permission group NNA stored in a shape this client doesn't expect is
// reported as unconfigured rather than failing the whole decode. NNA performs
// no shape validation on these columns (confirmed live: a wrong-typed
// "sources":"notanarray" is accepted and stored verbatim), and because
// ListRoles decodes the entire list, an error here would break plan/apply/
// destroy for every nagios_nna_role rather than just the malformed one.
func TestRole_UnmarshalJSON_MalformedGroupDoesNotFailTheDecode(t *testing.T) {
	body := `{"id":7,"name":"junk","traceroute_permissions":[],
		"flow_source_permissions":{"sources":"notanarray","bogus_key":["fly"]},
		"nmap_permissions":{"scans":["get"]}}`

	var r Role
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("a malformed permission group must not fail the decode, got: %v", err)
	}
	if r.Name != "junk" || r.ID != 7 {
		t.Errorf("expected the rest of the role to still decode, got %+v", r)
	}
	if r.FlowSourcePermissions != nil {
		t.Errorf("expected an undecodable group to read back as unconfigured (nil), got %+v", r.FlowSourcePermissions)
	}
	// A malformed sibling must not affect a well-formed group on the same role.
	if r.NmapPermissions == nil || len(r.NmapPermissions.Scans) != 1 {
		t.Errorf("expected the well-formed nmap group to still decode, got %+v", r.NmapPermissions)
	}
}

// TestListRoles_MalformedRoleDoesNotPoisonTheList is the list-level guarantee
// behind the decode tolerance above: one bad role must not make every role
// unreadable, since GetRole and getRoleByName both funnel through ListRoles.
func TestListRoles_MalformedRoleDoesNotPoisonTheList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"bad","traceroute_permissions":{"traceroutes":"notanarray"}},{"id":2,"name":"good","traceroute_permissions":{"traceroutes":["get"]}}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("one malformed role must not fail the whole list, got: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(got))
	}
	if got[1].TraceroutePermissions == nil || len(got[1].TraceroutePermissions.Traceroutes) != 1 {
		t.Errorf("expected the healthy role to decode normally, got %+v", got[1].TraceroutePermissions)
	}
}

// TestRole_UnmarshalJSON_WhitespaceArrayIsSetButEmpty confirms the
// empty-group detection matches on the array shape rather than an exact "[]"
// byte compare, so interior whitespace doesn't fall through to the struct
// decode.
func TestRole_UnmarshalJSON_WhitespaceArrayIsSetButEmpty(t *testing.T) {
	var r Role
	if err := json.Unmarshal([]byte(`{"id":1,"name":"n","traceroute_permissions":[ ]}`), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TraceroutePermissions == nil {
		t.Error("expected a whitespace-padded empty array to decode as set-but-empty, got nil")
	}
}
