package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNNANmapScanBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_scan.scan"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapScanDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapScanConfig(name, "-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00" }`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapScanExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "parameters", "-sS -e eth0 10.0.0.1"),
					resource.TestCheckResourceAttr(rName, "schedule_type", "dailyAt"),
					resource.TestCheckResourceAttr(rName, "schedule_parameters.time", "02:00"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttr(rName, "ndiffs_enabled", "true"),
					resource.TestCheckResourceAttr(rName, "times_ran", "0"),
					resource.TestCheckNoResourceAttr(rName, "nmap_profile_id"),
					resource.TestCheckResourceAttrSet(rName, "id"),
				),
			},
			{
				// An identical config must be an empty plan: guards the
				// 0/1-vs-bool ndiffs_enabled read and map ordering.
				Config: testAccNNANmapScanConfig(name, "-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00" }`),
				PlanOnly: true,
			},
			{
				ResourceName:      rName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNNANmapScanUpdate walks one scan through every schedule shape and
// the writable flags. Update is a partial PUT server-side, but the schedule
// parameter object is replaced wholesale, so a type change must not leave
// the previous type's keys behind.
func TestAccNNANmapScanUpdate(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_scan.scan"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapScanDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapScanConfig(name, "-sS -e eth0 10.0.0.1", `schedule_type = "every"
  schedule_parameters = { x = "5" }`),
				Check: resource.TestCheckResourceAttr(rName, "schedule_parameters.x", "5"),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "weeklyOn"
  schedule_parameters = { weekday = "1", time = "03:00" }
  enabled = false
  ndiffs_enabled = false`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "name", name+"_renamed"),
					resource.TestCheckResourceAttr(rName, "parameters", "-sS -e eth0 10.0.0.2"),
					resource.TestCheckResourceAttr(rName, "schedule_type", "weeklyOn"),
					resource.TestCheckResourceAttr(rName, "schedule_parameters.%", "2"),
					resource.TestCheckNoResourceAttr(rName, "schedule_parameters.x"),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					resource.TestCheckResourceAttr(rName, "ndiffs_enabled", "false"),
				),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "yearlyOn"
  schedule_parameters = { month = "3", day = "14", time = "04:00" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "schedule_parameters.month", "3"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttr(rName, "ndiffs_enabled", "true"),
				),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "cron"
  schedule_parameters = { cron = "*/5 * * * *" }`),
				Check: resource.TestCheckResourceAttr(rName, "schedule_parameters.cron", "*/5 * * * *"),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "monthlyOn"
  schedule_parameters = { day = "1", time = "00:30" }`),
				Check: resource.TestCheckResourceAttr(rName, "schedule_parameters.%", "2"),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "hourlyAt"
  schedule_parameters = { minute = "15" }`),
				Check: resource.TestCheckResourceAttr(rName, "schedule_parameters.minute", "15"),
			},
			{
				Config: testAccNNANmapScanConfig(name+"_renamed", "-sS -e eth0 10.0.0.2", `schedule_type = "hourlyAt"
  schedule_parameters = { minute = "15" }`),
				PlanOnly: true,
			},
		},
	})
}

// TestAccNNANmapScanProfile covers nmap_profile_id, which NNA enforces as a
// real foreign key, and clearing it again - PUT is partial, so a cleared
// reference only lands if it's sent as an explicit null.
func TestAccNNANmapScanProfile(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_scan.scan"

	profile := fmt.Sprintf(`
resource "nagios_nna_nmap_profile" "profile" {
  name       = %q
  parameters = "-sS -e eth0"
}
`, name+"_prof")
	cfg := func(profileLine string) string {
		return profile + fmt.Sprintf(`
resource "nagios_nna_nmap_scan" "scan" {
  name                = %q
  parameters          = "-sS -e eth0 10.0.0.1"
  %s
  schedule_type       = "dailyAt"
  schedule_parameters = { time = "02:00" }
}
`, name, profileLine)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapScanDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg("nmap_profile_id = nagios_nna_nmap_profile.profile.id"),
				Check: resource.TestCheckResourceAttrPair(rName, "nmap_profile_id",
					"nagios_nna_nmap_profile.profile", "id"),
			},
			{
				Config: cfg(""),
				Check:  resource.TestCheckNoResourceAttr(rName, "nmap_profile_id"),
			},
		},
	})
}

func TestAccNNANmapScanCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_scan.scan"
	cfg := testAccNNANmapScanConfig(name, "-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00" }`)
	var capturedID int64

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapScanDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(s *terraform.State) error {
					id, err := nnaNmapScanIDFromState(s, rName)
					capturedID = id
					return err
				},
			},
			{
				PreConfig: func() {
					if err := testAccNNAClient(t).DeleteNmapScheduledScan(context.Background(), capturedID); err != nil {
						t.Fatalf("deleting scan %d out of band: %s", capturedID, err)
					}
				},
				Config: cfg,
				Check:  testAccCheckNNANmapScanExists(t, rName),
			},
		},
	})
}

// TestAccNNANmapScanReferencedByCheck is the payoff for the whole resource:
// an nmap check whose object_id is a real, provider-managed scan rather than
// a literal id NNA happens not to validate.
func TestAccNNANmapScanReferencedByCheck(t *testing.T) {
	name := "tf_" + acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{{
			Config: testAccNNANmapScanConfig(name, "-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00" }`) + fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %q
  check_type         = "nmap"
  object_type        = "scheduled_scan"
  object_id          = nagios_nna_nmap_scan.scan.id
  metric             = "ports_open"
  warning_threshold  = "5"
  critical_threshold = "10"
}
`, name+"_chk"),
			Check: resource.TestCheckResourceAttrPair("nagios_nna_check.check", "object_id",
				"nagios_nna_nmap_scan.scan", "id"),
		}},
	})
}

// TestAccNNANmapScanInvalidConfig confirms NNA's silent acceptance of bad
// schedules, and its update-time skipping of the -e check, are both caught
// at plan time.
func TestAccNNANmapScanInvalidConfig(t *testing.T) {
	cases := map[string]struct{ params, sched, errRe string }{
		"no interface flag": {"-sS 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00" }`, `standalone "-e <interface>" flag`},
		"unknown schedule type": {"-sS -e eth0 10.0.0.1", `schedule_type = "fortnightly"
  schedule_parameters = { time = "02:00" }`, `schedule_type value must be one of`},
		"missing required key": {"-sS -e eth0 10.0.0.1", `schedule_type = "weeklyOn"
  schedule_parameters = { time = "02:00" }`, `requires schedule_parameters key "weekday"`},
		"unexpected key": {"-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "02:00", minute = "5" }`, `does not use schedule_parameters key "minute"`},
		"malformed time": {"-sS -e eth0 10.0.0.1", `schedule_type = "dailyAt"
  schedule_parameters = { time = "25:99" }`, `must be HH:MM`},
		"non numeric day": {"-sS -e eth0 10.0.0.1", `schedule_type = "monthlyOn"
  schedule_parameters = { day = "first", time = "02:00" }`, `must be a whole number`},
		"unsupported key": {"-sS -e eth0 10.0.0.1", `schedule_type = "every"
  schedule_parameters = { x = "5", days = "1" }`, `unsupported schedule_parameters key "days"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccNNAPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      testAccNNANmapScanConfig("tf_invalid", tc.params, tc.sched),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(tc.errRe), " ", `[\s│]+`)),
				}},
			})
		})
	}
}

func testAccNNANmapScanConfig(name, parameters, schedule string) string {
	return fmt.Sprintf(`
resource "nagios_nna_nmap_scan" "scan" {
  name       = %[1]q
  parameters = %[2]q
  %[3]s
}
`, name, parameters, schedule)
}

func nnaNmapScanIDFromState(s *terraform.State, resourceName string) (int64, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return 0, fmt.Errorf("resource not found in state: %s", resourceName)
	}
	id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
	}
	return id, nil
}

func testAccCheckNNANmapScanExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := nnaNmapScanIDFromState(s, resourceName)
		if err != nil {
			return err
		}
		got, err := testAccNNAClient(t).GetNmapScheduledScan(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA nmap scheduled scan id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNANmapScanDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_nmap_scan" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetNmapScheduledScan(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA nmap scheduled scan id %d still exists after destroy", id)
			}
		}
		return nil
	}
}
