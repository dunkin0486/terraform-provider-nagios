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

// decodeWriteBody pulls the nested wizard payload back out of a recorded
// request body, so tests can assert on the step1/step2/step3 shape the
// CheckController actually requires.
func decodeWriteBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
	return body
}

// TestNewCheck_SendsWizardShapedBody confirms the create request is sent in
// NNA's nested step1/step2/step3 wizard shape with the write-side
// warning_threshold/critical_threshold names - not the flat warning/critical
// column names a GET returns.
func TestNewCheck_SendsWizardShapedBody(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/checks":
			if h := r.Header.Get("Authorization"); h != "Bearer TOKEN" {
				t.Errorf("Authorization header = %q, want %q", h, "Bearer TOKEN")
			}
			if h := r.Header.Get("Content-Type"); h != "application/json" {
				t.Errorf("Content-Type header = %q, want application/json", h)
			}
			got = decodeWriteBody(t, r)
			_, _ = w.Write([]byte(`{"message":"Check created successfully","check":{"id":7,"check_type":"flow_source","name":"c","object_type":"source","object_id":3,"metric":"bytes","warning":"100","critical":"200","raw_query":"src port 443","profile_id":null}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/checks/7":
			_, _ = w.Write([]byte(`{"id":7,"active":1,"check_type":"flow_source","name":"c","object_type":"source","object_id":3,"metric":"bytes","warning":"100","critical":"200","raw_query":"src port 443","profile_id":null,"alerting_associations":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	in := &Check{
		Name:       "c",
		CheckType:  "flow_source",
		ObjectType: "source",
		ObjectID:   3,
		Metric:     "bytes",
		Warning:    "100",
		Critical:   "200",
		Queries: []CheckQuery{{
			Location:      "source",
			LocationType:  "port",
			LocationBool:  "is",
			LocationValue: "443",
		}},
		AlertUsers: []int64{1},
	}

	created, err := c.NewCheck(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil || created.ID != 7 {
		t.Fatalf("expected the check with id 7, got %+v", created)
	}

	if got["check_type"] != "flow_source" {
		t.Errorf("check_type = %v, want flow_source", got["check_type"])
	}
	step1, ok := got["step1"].(map[string]any)
	if !ok {
		t.Fatalf("step1 missing or not an object: %v", got["step1"])
	}
	if step1["name"] != "c" {
		t.Errorf("step1.name = %v, want c", step1["name"])
	}
	assoc, ok := step1["association"].(map[string]any)
	if !ok {
		t.Fatalf("step1.association missing: %v", step1["association"])
	}
	if assoc["type"] != "source" || assoc["id"] != "3" {
		t.Errorf("step1.association = %v, want type=source id=\"3\"", assoc)
	}

	step2, ok := got["step2"].(map[string]any)
	if !ok {
		t.Fatalf("step2 missing: %v", got["step2"])
	}
	if step2["warning_threshold"] != "100" || step2["critical_threshold"] != "200" {
		t.Errorf("step2 thresholds = %v, want warning_threshold=100 critical_threshold=200", step2)
	}
	if _, present := step2["warning"]; present {
		t.Error("step2 must use the write-side name warning_threshold, not warning")
	}
	queries, ok := step2["queries"].([]any)
	if !ok || len(queries) != 1 {
		t.Fatalf("step2.queries = %v, want one entry", step2["queries"])
	}

	step3, ok := got["step3"].(map[string]any)
	if !ok {
		t.Fatalf("step3 missing: %v", got["step3"])
	}
	for _, key := range []string{"user", "nagios", "snmp_receiver", "command"} {
		if _, present := step3[key]; !present {
			t.Errorf("step3.%s missing - NNA 500s on any absent step3 key", key)
		}
	}
}

// TestNewCheck_AlwaysSendsEmptyArraysNotNull confirms the optional list
// fields marshal as [] rather than JSON null. NNA's CheckController reads
// every step key with raw array access and has no validator, so a null
// where it expects an array is a 500 rather than a clean error.
func TestNewCheck_AlwaysSendsEmptyArraysNotNull(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			raw = string(b)
			_, _ = w.Write([]byte(`{"message":"Check created successfully","check":{"id":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.NewCheck(context.Background(), &Check{Name: "c", CheckType: "flow_source", ObjectType: "source", ObjectID: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(raw, "null") {
		t.Errorf("request body must not contain null for the array fields, got %s", raw)
	}
	for _, want := range []string{`"queries":[]`, `"user":[]`, `"nagios":[]`, `"snmp_receiver":[]`, `"command":[]`} {
		if !strings.Contains(raw, want) {
			t.Errorf("request body missing %s, got %s", want, raw)
		}
	}
}

// TestNewCheck_ReadsBackFullObjectSinceCreateResponseIsPartial confirms
// NewCheck follows its POST with a GET by the id the create response
// returns. The create response's "check" object omits active,
// alerting_associations and the last_* result fields (confirmed live), so
// it can't be used as the created object on its own - but unlike sources,
// it does carry the id, so no list-and-match-by-name lookup is needed.
func TestNewCheck_ReadsBackFullObjectSinceCreateResponseIsPartial(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"message":"Check created successfully","check":{"id":42,"name":"c"}}`))
			return
		}
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":42,"active":0,"name":"c","alerting_associations":[{"association_type":"user","association_id":5}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	created, err := c.NewCheck(context.Background(), &Check{Name: "c", CheckType: "flow_source", ObjectType: "source", ObjectID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/checks/42" {
		t.Errorf("expected a read-back GET of /api/v1/checks/42, got %q", gotPath)
	}
	if created.Active != 0 {
		t.Errorf("Active = %d, want 0 from the read-back GET", created.Active)
	}
	if len(created.AlertUsers) != 1 || created.AlertUsers[0] != 5 {
		t.Errorf("AlertUsers = %v, want [5] rebuilt from alerting_associations", created.AlertUsers)
	}
}

// TestNewCheck_PropagatesUnsupportedCheckType confirms the one thing NNA
// does validate server-side on this endpoint is check_type, which fails
// with a clean 400 rather than the 500s every other bad input produces.
func TestNewCheck_PropagatesUnsupportedCheckType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Check type [ nonsense ] not supported"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.NewCheck(context.Background(), &Check{CheckType: "nonsense"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected a nil check on failure, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("got error %q, want it to mention the unsupported check type", err.Error())
	}
}

// TestNewCheck_PropagatesMissingStepKey500 confirms the "Undefined array
// key" 500 NNA returns for an absent step key surfaces as an error rather
// than being mistaken for success (every response on this endpoint is a
// real status code, but a 500 still carries a JSON body).
func TestNewCheck_PropagatesMissingStepKey500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Undefined array key \"step3\"","exception":"ErrorException"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.NewCheck(context.Background(), &Check{CheckType: "flow_source"}); err == nil {
		t.Fatal("expected an error, got nil")
	} else if !strings.Contains(err.Error(), "step3") {
		t.Errorf("got error %q, want it to name the missing step key", err.Error())
	}
}

func TestGetCheck_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/checks/3" {
			t.Errorf("expected path /api/v1/checks/3, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":3,"active":1,"name":"c","object_type":"source","object_id":9,"metric":"flows","warning":"10","critical":"20","raw_query":"src port 80","check_type":"flow_source","profile_id":null,"last_val":"5","last_code":0,"alerting_associations":[{"association_type":"user","association_id":2},{"association_type":"command","association_id":4}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetCheck(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a check, got nil")
	}
	if got.Name != "c" || got.Metric != "flows" || got.Warning != "10" || got.Critical != "20" {
		t.Errorf("got %+v, want name=c metric=flows warning=10 critical=20", got)
	}
	if got.RawQuery != "src port 80" {
		t.Errorf("RawQuery = %q, want %q", got.RawQuery, "src port 80")
	}
	if len(got.AlertUsers) != 1 || got.AlertUsers[0] != 2 {
		t.Errorf("AlertUsers = %v, want [2]", got.AlertUsers)
	}
	if len(got.AlertCommands) != 1 || got.AlertCommands[0] != 4 {
		t.Errorf("AlertCommands = %v, want [4]", got.AlertCommands)
	}
}

// TestGetCheck_NotFound confirms NNA's 404 comes back as (nil, nil) per
// CLAUDE.md quirk 9. Note the body is {"message":"Check not found"} here,
// not the {"message":"Resource not found for id: N"} shape sources use -
// GetCheck branches on the status code, so the wording doesn't matter.
func TestGetCheck_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Check not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetCheck(context.Background(), 404)
	if err != nil {
		t.Fatalf("expected no error on not-found, got %v", err)
	}
	if got != nil {
		t.Errorf("expected (nil, nil) on not-found, got %+v", got)
	}
}

// TestGetCheck_NullThresholdsDecodeToEmpty confirms a check stored with no
// threshold (NNA writes an empty threshold through as SQL NULL rather than
// "") decodes without error.
func TestGetCheck_NullThresholdsDecodeToEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"active":1,"warning":"@10:20","critical":null,"last_val":null,"last_run":null,"last_code":null,"last_stdout":null,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.GetCheck(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Warning != "@10:20" {
		t.Errorf("Warning = %q, want %q", got.Warning, "@10:20")
	}
	if got.Critical != "" {
		t.Errorf("Critical = %q, want \"\" for a JSON null", got.Critical)
	}
}

// TestUpdateCheck_ReReadsSinceResponseCarriesNoObject confirms UpdateCheck
// follows its PUT with a GET. Unlike UpdateSource's {"source": {...}}
// envelope, NNA's check update response is only
// {"message":"Check updated successfully"} - no object at all (confirmed
// live), so the updated object has to be fetched separately.
func TestUpdateCheck_ReReadsSinceResponseCarriesNoObject(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"message":"Check updated successfully"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":5,"active":1,"name":"renamed","metric":"flows","warning":"77","critical":"88","alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	got, err := c.UpdateCheck(context.Background(), 5, &Check{Name: "renamed", CheckType: "flow_source", ObjectType: "source", ObjectID: 1, Metric: "flows", Warning: "77", Critical: "88"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "renamed" || got.Warning != "77" {
		t.Fatalf("got %+v, want the re-read object", got)
	}
	want := []string{"PUT /api/v1/checks/5", "GET /api/v1/checks/5"}
	if len(methods) != 2 || methods[0] != want[0] || methods[1] != want[1] {
		t.Errorf("requests = %v, want %v", methods, want)
	}
}

// TestUpdateCheck_NotFound confirms a PUT against a vanished check is
// surfaced as an error rather than silently succeeding.
func TestUpdateCheck_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Check not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if _, err := c.UpdateCheck(context.Background(), 9, &Check{CheckType: "flow_source"}); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestDeleteCheck_TreatsNotFoundAsSuccess confirms DeleteCheck tolerates a
// 404. Unlike DeleteSource (which NNA makes idempotent server-side by
// returning 200 for an already-gone id), deleting a check twice returns
// 404 {"message":"Resource not found for id: N"} - confirmed live - so the
// idempotency has to be supplied client-side or a `terraform destroy`
// after an out-of-band delete fails.
func TestDeleteCheck_TreatsNotFoundAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 1"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteCheck(context.Background(), 1); err != nil {
		t.Errorf("expected a 404 delete to be treated as success, got %v", err)
	}
}

func TestDeleteCheck_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.DeleteCheck(context.Background(), 1); err == nil {
		t.Fatal("expected a 500 delete to return an error")
	}
}

// TestSetCheckActive_SkipsToggleWhenAlreadyInDesiredState is the crux of
// the toggle quirk: PATCH /checks/{id}/toggle FLIPS active rather than
// setting it, so an unconditional call would invert a check that was
// already correct.
func TestSetCheckActive_SkipsToggleWhenAlreadyInDesiredState(t *testing.T) {
	toggles := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			toggles++
			_, _ = w.Write([]byte(`{"active":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.SetCheckActive(context.Background(), 1, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toggles != 0 {
		t.Errorf("toggle called %d times, want 0 - the check was already active", toggles)
	}
}

func TestSetCheckActive_TogglesOnceWhenStateDiffers(t *testing.T) {
	toggles := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			if r.URL.Path != "/api/v1/checks/1/toggle" {
				t.Errorf("toggle path = %s, want /api/v1/checks/1/toggle", r.URL.Path)
			}
			toggles++
			_, _ = w.Write([]byte(`{"active":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.SetCheckActive(context.Background(), 1, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toggles != 1 {
		t.Errorf("toggle called %d times, want exactly 1", toggles)
	}
}

// TestSetCheckActive_ErrorsWhenToggleLandsOnWrongState guards against the
// toggle silently reporting the state we didn't ask for - the toggle
// response body is the authority on where it ended up, and it is a JSON
// bool here even though a GET reports active as 0/1.
func TestSetCheckActive_ErrorsWhenToggleLandsOnWrongState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			_, _ = w.Write([]byte(`{"active":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	err := c.SetCheckActive(context.Background(), 1, false)
	if err == nil {
		t.Fatal("expected an error when the toggle reports the wrong resulting state")
	}
}

// TestSetCheckActive_NotFound confirms a vanished check surfaces as an
// error rather than a silent no-op.
func TestSetCheckActive_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Check not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN")
	if err := c.SetCheckActive(context.Background(), 1, false); err == nil {
		t.Fatal("expected an error for a check that no longer exists")
	}
}

// TestNewCheck_MissingIDInResponseDoesNotLeakBody covers the defensive
// branch for a 2xx create response that carries no id. The error must not
// echo the raw body: a non-JSON 2xx is most likely an HTML error/debug
// page, and Laravel's debug page reflects the request's Authorization
// header (this client's Bearer token) back into the page body when
// APP_DEBUG is on - which is exactly why response.go's parseError refuses
// to include raw bodies either.
func TestNewCheck_MissingIDInResponseDoesNotLeakBody(t *testing.T) {
	const secret = "Bearer SUPERSECRETTOKEN"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"ok","debug":{"Authorization":"` + secret + `"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "SUPERSECRETTOKEN")
	got, err := c.NewCheck(context.Background(), &Check{Name: "c", CheckType: "flow_source"})
	if err == nil {
		t.Fatal("expected an error when the create response carries no id")
	}
	if got != nil {
		t.Errorf("expected a nil check, got %+v", got)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "SUPERSECRETTOKEN") {
		t.Errorf("error leaked the response body verbatim: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "no id") {
		t.Errorf("got error %q, want it to explain the missing id", err.Error())
	}
}

// TestGetCheck_RedistributesAllFourAssociationTypes pins the read-side
// association_type discriminators. All four were confirmed live against a
// check created with every recipient kind at once, and all four match the
// step3 write-side key names exactly - a mismatch on any of them would
// silently strand ids in the wrong attribute (or, with the default branch
// below, error out).
func TestGetCheck_RedistributesAllFourAssociationTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[
			{"association_type":"user","association_id":11},
			{"association_type":"nagios","association_id":22},
			{"association_type":"snmp_receiver","association_id":33},
			{"association_type":"command","association_id":44}
		]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").GetCheck(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tc := range []struct {
		name string
		got  []int64
		want int64
	}{
		{"AlertUsers", got.AlertUsers, 11},
		{"AlertNagiosServers", got.AlertNagiosServers, 22},
		{"AlertSNMPReceivers", got.AlertSNMPReceivers, 33},
		{"AlertCommands", got.AlertCommands, 44},
	} {
		if len(tc.got) != 1 || tc.got[0] != tc.want {
			t.Errorf("%s = %v, want [%d]", tc.name, tc.got, tc.want)
		}
	}
}

// TestGetCheck_UnknownAssociationTypeErrors confirms an unrecognized
// association_type fails loudly rather than being dropped. A silent drop
// would resurface much later as a "provider produced inconsistent result
// after apply" on whichever alert_* attribute the id belonged to.
func TestGetCheck_UnknownAssociationTypeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":9,"active":1,"alerting_associations":[{"association_type":"carrier_pigeon","association_id":7}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").GetCheck(context.Background(), 9)
	if err == nil {
		t.Fatal("expected an error for an unrecognized association type")
	}
	if got != nil {
		t.Errorf("expected a nil check, got %+v", got)
	}
	if !strings.Contains(err.Error(), "carrier_pigeon") {
		t.Errorf("got error %q, want it to name the unrecognized type", err.Error())
	}
}

// TestNewCheck_SendsAllFourStep3Arrays confirms each Alert* slice lands
// under its matching step3 key.
func TestNewCheck_SendsAllFourStep3Arrays(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			got = decodeWriteBody(t, r)
			_, _ = w.Write([]byte(`{"message":"ok","check":{"id":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"active":1,"alerting_associations":[]}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "TOKEN").NewCheck(context.Background(), &Check{
		Name: "c", CheckType: "flow_source", ObjectType: "source", ObjectID: 1,
		AlertUsers: []int64{11}, AlertNagiosServers: []int64{22},
		AlertSNMPReceivers: []int64{33}, AlertCommands: []int64{44},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step3 := got["step3"].(map[string]any)
	for key, want := range map[string]float64{"user": 11, "nagios": 22, "snmp_receiver": 33, "command": 44} {
		arr, ok := step3[key].([]any)
		if !ok || len(arr) != 1 || arr[0].(float64) != want {
			t.Errorf("step3.%s = %v, want [%v]", key, step3[key], want)
		}
	}
}
