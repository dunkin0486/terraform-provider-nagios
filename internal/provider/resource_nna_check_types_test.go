package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The nmap and suricata checks below reference an object_id (a scheduled
// scan, a Suricata signature) this provider has no resource for. NNA doesn't
// validate that reference - see nna.Check.ObjectType - so a literal id still
// creates, round-trips and destroys cleanly; it just never produces a result.

func TestAccNNACheckNmap(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckNmapConfig(name, "ports_open", "5", "10"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "check_type", "nmap"),
					resource.TestCheckResourceAttr(rName, "object_type", "scheduled_scan"),
					resource.TestCheckResourceAttr(rName, "object_id", "4242"),
					resource.TestCheckResourceAttr(rName, "metric", "ports_open"),
					resource.TestCheckResourceAttr(rName, "warning_threshold", "5"),
				),
			},
			{
				Config: testAccNNACheckNmapConfig(name, "ports_closed", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "metric", "ports_closed"),
					resource.TestCheckResourceAttr(rName, "critical_threshold", "2"),
				),
			},
			{Config: testAccNNACheckNmapConfig(name, "ports_closed", "1", "2"), PlanOnly: true},
		},
	})
}

func testAccNNACheckNmapConfig(name, metric, warning, critical string) string {
	return fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  check_type         = "nmap"
  object_type        = "scheduled_scan"
  object_id          = 4242
  metric             = %[2]q
  warning_threshold  = %[3]q
  critical_threshold = %[4]q
}
`, name, metric, warning, critical)
}

func TestAccNNACheckSuricata(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckSuricataConfig(name, "7d", "Hourly"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "check_type", "suricata"),
					resource.TestCheckResourceAttr(rName, "object_type", "signature_id"),
					resource.TestCheckResourceAttr(rName, "metric", "alert_count"),
					resource.TestCheckResourceAttr(rName, "lookback_period", "7d"),
					resource.TestCheckResourceAttr(rName, "check_frequency", "Hourly"),
					resource.TestCheckResourceAttr(rName, "raw_query", "frequency=Hourly&lookback_period=7d"),
				),
			},
			{
				Config: testAccNNACheckSuricataConfig(name, "30d", "Daily"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "lookback_period", "30d"),
					resource.TestCheckResourceAttr(rName, "check_frequency", "Daily"),
				),
			},
			{Config: testAccNNACheckSuricataConfig(name, "30d", "Daily"), PlanOnly: true},
			{
				ResourceName:      rName,
				ImportState:       true,
				ImportStateVerify: true,
				// queries is write-only and unset here anyway; nothing else
				// is excluded, so lookback_period/check_frequency must come
				// back out of raw_query on import.
			},
		},
	})
}

func testAccNNACheckSuricataConfig(name, lookback, frequency string) string {
	return fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  check_type         = "suricata"
  object_type        = "signature_id"
  object_id          = 2100498
  metric             = "alert_count"
  warning_threshold  = "5"
  critical_threshold = "10"
  lookback_period    = %[2]q
  check_frequency    = %[3]q
}
`, name, lookback, frequency)
}

// TestAccNNACheckSystem covers the check type with no target object: object_id
// is omitted from configuration and NNA reports the -1 sentinel back, so it
// must be Computed rather than perpetually diffing against null.
func TestAccNNACheckSystem(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckSystemConfig(name, "memory", "swap_percent", "80", "90"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "check_type", "system"),
					resource.TestCheckResourceAttr(rName, "object_type", "memory"),
					resource.TestCheckResourceAttr(rName, "object_id", "-1"),
					resource.TestCheckResourceAttr(rName, "metric", "swap_percent"),
					resource.TestCheckResourceAttr(rName, "warning_threshold", "80"),
				),
			},
			{
				Config: testAccNNACheckSystemConfig(name, "drive", "percent", "70", "85"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "object_type", "drive"),
					resource.TestCheckResourceAttr(rName, "metric", "percent"),
					resource.TestCheckResourceAttr(rName, "object_id", "-1"),
				),
			},
			{Config: testAccNNACheckSystemConfig(name, "drive", "percent", "70", "85"), PlanOnly: true},
		},
	})
}

func testAccNNACheckSystemConfig(name, objectType, metric, warning, critical string) string {
	return fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  check_type         = "system"
  object_type        = %[2]q
  metric             = %[3]q
  warning_threshold  = %[4]q
  critical_threshold = %[5]q
}
`, name, objectType, metric, warning, critical)
}

// TestAccNNACheckInvalidCombinations confirms the per-type pairings are
// rejected at plan time, since NNA itself accepts any of them.
func TestAccNNACheckInvalidCombinations(t *testing.T) {
	cfg := func(body string) string {
		return `resource "nagios_nna_check" "check" {
  name = "tf_invalid"
` + body + `}
`
	}
	cases := map[string]struct{ config, errRe string }{
		"nmap with a flow metric": {
			cfg(`  check_type = "nmap"
  object_type = "scheduled_scan"
  object_id = 1
  metric = "bytes"
`), `metric "bytes" is not valid for check_type "nmap"`},
		"flow_source with a system object_type": {
			cfg(`  object_type = "cpu"
  object_id = 1
  metric = "bytes"
`), `object_type "cpu" is not valid for check_type "flow_source"`},
		"system cpu with a memory metric": {
			cfg(`  check_type = "system"
  object_type = "cpu"
  metric = "used_percent"
`), `metric "used_percent" is not valid for object_type "cpu"`},
		"system with object_id": {
			cfg(`  check_type = "system"
  object_type = "cpu"
  object_id = 3
  metric = "cpu_usage"
`), `object_id must not be set`},
		"flow_source without object_id": {
			cfg(`  object_type = "source"
  metric = "bytes"
`), `object_id is required`},
		"suricata without lookback_period": {
			cfg(`  check_type = "suricata"
  object_type = "signature_id"
  object_id = 1
  metric = "alert_count"
  check_frequency = "Daily"
`), `lookback_period is required`},
		"nmap with lookback_period": {
			cfg(`  check_type = "nmap"
  object_type = "scheduled_scan"
  object_id = 1
  metric = "ports_open"
  lookback_period = "7d"
`), `lookback_period and check_frequency are only valid`},
		"nmap with queries": {
			cfg(`  check_type = "nmap"
  object_type = "scheduled_scan"
  object_id = 1
  metric = "ports_open"
  queries = [{ location = "source", location_type = "port", location_bool = "is", location_value = "80" }]
`), `queries are only valid for check_type "flow_source"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccNNAPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      tc.config,
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(tc.errRe), " ", `[\s│]+`)),
				}},
			})
		})
	}
}

// TestAccNNACheckChangeTypeReplaces guards switching an existing check's
// check_type. object_id keeps its prior state value across an in-place
// update (UseStateForUnknown), so an nmap -> system edit would plan the old
// id while NNA reports -1, failing with "inconsistent result after apply".
// check_type changes the shape of the whole check, so it forces replacement.
func TestAccNNACheckChangeTypeReplaces(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_check.check"
	var firstID, secondID int64

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckNmapConfig(name, "ports_open", "5", "10"),
				Check:  testAccCaptureNNACheckID(rName, &firstID),
			},
			{
				Config: testAccNNACheckSystemConfig(name, "cpu", "cpu_usage", "80", "90"),
				Check: resource.ComposeTestCheckFunc(
					testAccCaptureNNACheckID(rName, &secondID),
					resource.TestCheckResourceAttr(rName, "check_type", "system"),
					resource.TestCheckResourceAttr(rName, "object_id", "-1"),
					func(*terraform.State) error {
						if firstID == secondID {
							return fmt.Errorf("check id %d was updated in place, want replacement", firstID)
						}
						return nil
					},
				),
			},
		},
	})
}
