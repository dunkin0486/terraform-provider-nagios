package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// multiValueHostFields lists the nagios_host fields that arrive over the
// wire as a single CSV-joined form/query value (see setURLParams'
// mapArrayToString) but must round-trip back out of GET as a JSON array,
// matching the real API's response shape.
var multiValueHostFields = map[string]bool{
	"contacts":               true,
	"templates":              true,
	"use":                    true,
	"contact_groups":         true,
	"parents":                true,
	"flap_detection_options": true,
}

// newMockNagiosHostServer stands in for a real Nagios XI instance, backing
// only the "host" object type and applyconfig - enough to drive
// nagios_host's full Create/Read/Update/Delete lifecycle through
// resource.UnitTest without TF_ACC or Docker (see CLAUDE.md's description of
// #88's phase 2). It returns the server and a function to check whether a
// host by that name currently "exists" in the mock backend, so a test can
// confirm Delete actually ran.
func newMockNagiosHostServer(t *testing.T) (*httptest.Server, func(name string) bool) {
	t.Helper()

	var mu sync.Mutex
	hosts := map[string]url.Values{}

	writeSuccess := func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"success":"ok"}`))
	}

	toJSONObject := func(v url.Values) map[string]any {
		obj := map[string]any{}
		for k, vals := range v {
			if k == "apikey" || k == "pretty" || k == "force" || len(vals) == 0 {
				continue
			}
			if multiValueHostFields[k] {
				if vals[0] == "" {
					obj[k] = []string{}
				} else {
					obj[k] = strings.Split(vals[0], ",")
				}
				continue
			}
			obj[k] = vals[0]
		}
		return obj
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/system/applyconfig", func(w http.ResponseWriter, r *http.Request) {
		writeSuccess(w)
	})

	mux.HandleFunc("POST /api/v1/config/host", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		hosts[r.PostForm.Get("host_name")] = cloneURLValues(r.PostForm)
		mu.Unlock()
		writeSuccess(w)
	})

	mux.HandleFunc("GET /api/v1/config/host", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		rec, ok := hosts[r.URL.Query().Get("host_name")]
		mu.Unlock()
		if !ok {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		body, err := json.Marshal([]map[string]any{toJSONObject(rec)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})

	mux.HandleFunc("PUT /api/v1/config/host/{oldName}", func(w http.ResponseWriter, r *http.Request) {
		oldName := r.PathValue("oldName")

		mu.Lock()
		defer mu.Unlock()
		if _, ok := hosts[oldName]; !ok {
			_, _ = w.Write([]byte(`{"error":"Does the host exist?"}`))
			return
		}
		delete(hosts, oldName)
		hosts[r.URL.Query().Get("host_name")] = cloneURLValues(r.URL.Query())
		writeSuccess(w)
	})

	mux.HandleFunc("DELETE /api/v1/config/host", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		delete(hosts, r.URL.Query().Get("host_name"))
		mu.Unlock()
		writeSuccess(w)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	exists := func(name string) bool {
		mu.Lock()
		defer mu.Unlock()
		_, ok := hosts[name]
		return ok
	}
	return srv, exists
}

func cloneURLValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vals := range v {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
}

func testUnitHostConfig(nagiosURL, address string) string {
	return fmt.Sprintf(`
provider "nagios" {
  url   = %[1]q
  token = "TEST-TOKEN"
}

resource "nagios_host" "test" {
  host_name             = "myhost"
  address               = %[2]q
  max_check_attempts    = "5"
  check_period          = "24x7"
  notification_interval  = "10"
  notification_period    = "24x7"
  contacts               = ["nagiosadmin"]
  alias                  = "myalias"
}
`, nagiosURL, address)
}

// TestUnitHostLifecycle drives nagios_host's full Create/Read/Update/Delete
// path through the real terraform-plugin-framework server, against a mock
// Nagios backend instead of a live instance or TF_ACC/Docker - closing the
// #88 gap that this path was previously only ever exercised by the
// TF_ACC-gated acceptance suite, which CI never runs.
func TestUnitHostLifecycle(t *testing.T) {
	srv, hostExists := newMockNagiosHostServer(t)
	rName := "nagios_host.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitHostConfig(srv.URL, "127.0.0.1"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "host_name", "myhost"),
					resource.TestCheckResourceAttr(rName, "address", "127.0.0.1"),
					resource.TestCheckResourceAttr(rName, "alias", "myalias"),
					resource.TestCheckResourceAttr(rName, "register", "true"),
				),
			},
			{
				// Changes a non-identifying attribute in place - the exact
				// gap #88 called out: every acceptance UpdateName test only
				// ever renames, never exercises a plain in-place attribute
				// change through UpdateHost.
				Config: testUnitHostConfig(srv.URL, "127.0.0.2"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "host_name", "myhost"),
					resource.TestCheckResourceAttr(rName, "address", "127.0.0.2"),
				),
			},
		},
	})

	if hostExists("myhost") {
		t.Error("expected the host to be deleted from the mock backend after the test, but it still exists")
	}
}
