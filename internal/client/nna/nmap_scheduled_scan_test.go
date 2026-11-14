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

const scanGET = `{"id":7,"user_id":1,"nmap_profile_id":null,"name":"s","parameters":"-sS -e eth0 10.0.0.1","schedule_type":"weeklyOn","schedule_parameters":{"time":"03:00","weekday":"1"},"last_run_at":null,"times_ran":2,"enabled":true,"ndiffs_enabled":1,"created_at":"x","updated_at":"y"}`

func TestNewNmapScheduledScan_SendsBodyAndReadsBack(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nmap/scheduled-scans":
			if h := r.Header.Get("Authorization"); h != "Bearer TOKEN" {
				t.Errorf("Authorization = %q", h)
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":"Nmap scheduled scan created successfully","nmap_scheduled_scan":{"id":7,"name":"s"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nmap/scheduled-scans/7":
			_, _ = w.Write([]byte(scanGET))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	profileID := int64(3)
	created, err := NewClient(srv.URL, "TOKEN").NewNmapScheduledScan(context.Background(), &NmapScheduledScan{
		Name: "s", Parameters: "-sS -e eth0 10.0.0.1", NmapProfileID: &profileID,
		ScheduleType: "weeklyOn", ScheduleParameters: map[string]string{"weekday": "1", "time": "03:00"},
		Enabled: true, NdiffsEnabled: true,
	})
	if err != nil {
		t.Fatalf("NewNmapScheduledScan: %v", err)
	}
	if got["name"] != "s" || got["nmap_profile_id"] != float64(3) || got["enabled"] != true || got["ndiffs_enabled"] != true {
		t.Errorf("body = %v", got)
	}
	if sp := got["schedule_parameters"].(map[string]any); sp["weekday"] != "1" || sp["time"] != "03:00" {
		t.Errorf("schedule_parameters = %v", sp)
	}
	for _, k := range []string{"times_ran", "last_run_at", "id"} {
		if _, present := got[k]; present {
			t.Errorf("read-only field %q must not be sent", k)
		}
	}
	// The create response is partial, so the returned scan must be the GET.
	if created.ID != 7 || created.TimesRan != 2 || !created.Enabled {
		t.Errorf("created = %+v, want the read-back object", created)
	}
}

// A cleared profile must be sent as an explicit null: PUT is partial, so an
// omitted nmap_profile_id would leave the old one in place.
func TestUpdateNmapScheduledScan_SendsNullProfileExplicitly(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			_, _ = w.Write([]byte(`{"message":"ok","nmap_scheduled_scan":` + scanGET + `}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "TOKEN").UpdateNmapScheduledScan(context.Background(), 7, &NmapScheduledScan{
		Name: "s", Parameters: "p", ScheduleType: "hourlyAt", ScheduleParameters: map[string]string{"minute": "5"},
	})
	if err != nil {
		t.Fatalf("UpdateNmapScheduledScan: %v", err)
	}
	if !strings.Contains(body, `"nmap_profile_id":null`) {
		t.Errorf("body %s must contain an explicit nmap_profile_id null", body)
	}
	if !strings.Contains(body, `"enabled":false`) {
		t.Errorf("body %s must always carry enabled", body)
	}
}

// GET reports ndiffs_enabled as 0/1 while create/PUT report a bool.
func TestNmapScheduledScan_DecodesBoolAsEitherBoolOrNumber(t *testing.T) {
	for in, want := range map[string]bool{`1`: true, `0`: false, `true`: true, `false`: false} {
		var s NmapScheduledScan
		if err := json.Unmarshal([]byte(`{"ndiffs_enabled":`+in+`,"enabled":`+in+`}`), &s); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if s.NdiffsEnabled != want || s.Enabled != want {
			t.Errorf("%s decoded to enabled=%t ndiffs=%t, want %t", in, s.Enabled, s.NdiffsEnabled, want)
		}
	}
}

// NNA stores an empty schedule_parameters as a PHP empty array.
func TestNmapScheduledScan_EmptyScheduleParametersDecodeFromArray(t *testing.T) {
	var s NmapScheduledScan
	if err := json.Unmarshal([]byte(`{"schedule_parameters":[]}`), &s); err != nil {
		t.Fatalf("decoding []: %v", err)
	}
	if len(s.ScheduleParameters) != 0 {
		t.Errorf("ScheduleParameters = %v, want empty", s.ScheduleParameters)
	}
}

func TestNmapScheduledScan_NonStringScheduleParameterErrors(t *testing.T) {
	var s NmapScheduledScan
	err := json.Unmarshal([]byte(`{"schedule_parameters":{"x":"5","days":["1","2"]}}`), &s)
	if err == nil || !strings.Contains(err.Error(), "days") {
		t.Fatalf("err = %v, want one naming the unsupported \"days\" parameter", err)
	}
}

func TestNmapScheduledScan_NumericScheduleParameterDecodesToString(t *testing.T) {
	var s NmapScheduledScan
	if err := json.Unmarshal([]byte(`{"schedule_parameters":{"x":5}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.ScheduleParameters["x"] != "5" {
		t.Errorf("x = %q, want \"5\"", s.ScheduleParameters["x"])
	}
}

func TestGetNmapScheduledScan_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 9"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "TOKEN").GetNmapScheduledScan(context.Background(), 9)
	if err != nil || got != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", got, err)
	}
}

func TestDeleteNmapScheduledScan_TreatsNotFoundAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource not found for id: 9"}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "TOKEN").DeleteNmapScheduledScan(context.Background(), 9); err != nil {
		t.Fatalf("DeleteNmapScheduledScan: %v", err)
	}
}

func TestDeleteNmapScheduledScan_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "TOKEN").DeleteNmapScheduledScan(context.Background(), 9); err == nil {
		t.Fatal("want an error for a 500")
	}
}

func TestNewNmapScheduledScan_PropagatesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The -e flag (interface) is required for Nmap scans."}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "TOKEN").NewNmapScheduledScan(context.Background(), &NmapScheduledScan{Name: "s"})
	if err == nil || !strings.Contains(err.Error(), "-e flag") {
		t.Fatalf("err = %v, want the -e flag message", err)
	}
}

func TestNewNmapScheduledScan_MissingIDInResponseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`<html>debug page</html>`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "TOKEN").NewNmapScheduledScan(context.Background(), &NmapScheduledScan{Name: "s"})
	if err == nil {
		t.Fatal("want an error for an unusable create response")
	}
	if strings.Contains(err.Error(), "debug page") {
		t.Errorf("error %q must not echo the raw response body", err)
	}
}
