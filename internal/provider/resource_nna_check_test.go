package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNNACheckBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceBasic(name, port, "bytes", "1000", "2000"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "check_type", "flow_source"),
					resource.TestCheckResourceAttr(rName, "object_type", "source"),
					resource.TestCheckResourceAttr(rName, "metric", "bytes"),
					resource.TestCheckResourceAttr(rName, "warning_threshold", "1000"),
					resource.TestCheckResourceAttr(rName, "critical_threshold", "2000"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttrSet(rName, "id"),
					// object_id must have been resolved from the source
					// resource's own id, since NNA won't validate it for us.
					resource.TestCheckResourceAttrPair(rName, "object_id", "nagios_nna_source.source", "id"),
				),
			},
		},
	})
}

// TestAccNNACheckUpdateThresholds confirms Update addresses the check by its
// immutable numeric id and that the write-side step2 threshold names
// round-trip back out of the flat warning/critical fields a GET returns.
func TestAccNNACheckUpdateThresholds(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceBasic(name, port, "bytes", "1000", "2000"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "warning_threshold", "1000"),
				),
			},
			{
				Config: testAccNNACheckResourceBasic(name+"_renamed", port, "flows", "10:20", "@30:40"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name+"_renamed"),
					resource.TestCheckResourceAttr(rName, "metric", "flows"),
					resource.TestCheckResourceAttr(rName, "warning_threshold", "10:20"),
					resource.TestCheckResourceAttr(rName, "critical_threshold", "@30:40"),
				),
			},
		},
	})
}

// TestAccNNACheckDisable confirms enabled=false drives the dedicated
// PATCH .../toggle action, since `active` is ignored in the create/update
// body. It also guards the toggle-is-a-flip-not-a-set quirk: the second
// apply at enabled=false must be a no-op rather than flipping the check
// back on.
func TestAccNNACheckDisable(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceBasic(name, port, "bytes", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
				),
			},
			{
				Config: testAccNNACheckResourceDisabled(name, port, "bytes", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					testAccCheckNNACheckActive(t, rName, false),
				),
			},
			{
				// An unrelated field change while still disabled: if the
				// toggle were called unconditionally, this apply would
				// silently re-enable the check.
				Config: testAccNNACheckResourceDisabled(name, port, "packets", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "metric", "packets"),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					testAccCheckNNACheckActive(t, rName, false),
				),
			},
		},
	})
}

// TestAccNNACheckQueriesCompileToRawQuery confirms the write-only queries
// attribute reaches NNA and that the server-compiled raw_query comes back -
// the only observable confirmation available, since queries itself is never
// returned.
func TestAccNNACheckQueriesCompileToRawQuery(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceWithQuery(name, port, "source", "port", "is", "443"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "queries.#", "1"),
					resource.TestCheckResourceAttr(rName, "raw_query", "src port 443"),
				),
			},
			{
				Config: testAccNNACheckResourceWithQuery(name, port, "destination", "port", "is", "80"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "raw_query", "dst port 80"),
				),
			},
		},
	})
}

// TestAccNNACheckCreateAfterManualDestroy confirms a check deleted outside
// Terraform is detected on refresh and recreated, rather than the provider
// erroring against a dead id. It also exercises DeleteCheck's client-side
// 404 tolerance, since CheckDestroy runs after the id is already gone.
func TestAccNNACheckCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"
	var capturedID int64

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceBasic(name, port, "bytes", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					testAccCaptureNNACheckID(rName, &capturedID),
				),
			},
			{
				PreConfig: func() {
					c := testAccNNAClient(t)
					if err := c.DeleteCheck(context.Background(), capturedID); err != nil {
						t.Fatalf("deleting check %d out of band: %s", capturedID, err)
					}
				},
				Config: testAccNNACheckResourceBasic(name, port, "bytes", "1", "2"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
				),
			},
		},
	})
}

func testAccCaptureNNACheckID(resourceName string, dest *int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := nnaCheckIDFromState(s, resourceName)
		if err != nil {
			return err
		}
		*dest = id
		return nil
	}
}

func nnaCheckIDFromState(s *terraform.State, resourceName string) (int64, error) {
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

func testAccCheckNNACheckExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := nnaCheckIDFromState(s, resourceName)
		if err != nil {
			return err
		}

		got, err := testAccNNAClient(t).GetCheck(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA check id %d not found", id)
		}
		return nil
	}
}

// testAccCheckNNACheckActive asserts the live `active` flag, not just the
// value Terraform recorded - the point of the toggle tests is that state and
// NNA can disagree if the flip semantics are mishandled.
func testAccCheckNNACheckActive(t *testing.T, resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := nnaCheckIDFromState(s, resourceName)
		if err != nil {
			return err
		}

		got, err := testAccNNAClient(t).GetCheck(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA check id %d not found", id)
		}
		if active := got.Active != 0; active != want {
			return fmt.Errorf("NNA check id %d: live active = %t, want %t", id, active, want)
		}
		return nil
	}
}

func testAccCheckNNACheckDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_check" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetCheck(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA check id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

// testAccNNACheckSource is the flow source every check config below hangs
// off: a check needs a real object_id, and NNA won't validate the reference
// itself, so the test creates one rather than assuming an id exists.
func testAccNNACheckSource(name string, port int) string {
	return fmt.Sprintf(`
resource "nagios_nna_source" "source" {
  name        = %[1]q
  port        = %[2]d
  flowtype    = "netflow"
  lifetime    = "30"
  description = "acceptance test source for nagios_nna_check"
}
`, name+"_src", port)
}

func testAccNNACheckResourceBasic(name string, port int, metric, warning, critical string) string {
	return testAccNNACheckSource(name, port) + fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  object_type        = "source"
  object_id          = nagios_nna_source.source.id
  metric             = %[2]q
  warning_threshold  = %[3]q
  critical_threshold = %[4]q
}
`, name, metric, warning, critical)
}

func testAccNNACheckResourceDisabled(name string, port int, metric, warning, critical string) string {
	return testAccNNACheckSource(name, port) + fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  object_type        = "source"
  object_id          = nagios_nna_source.source.id
  metric             = %[2]q
  warning_threshold  = %[3]q
  critical_threshold = %[4]q
  enabled            = false
}
`, name, metric, warning, critical)
}

func testAccNNACheckResourceWithQuery(name string, port int, location, locationType, locationBool, locationValue string) string {
	return testAccNNACheckSource(name, port) + fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name               = %[1]q
  object_type        = "source"
  object_id          = nagios_nna_source.source.id
  metric             = "bytes"
  warning_threshold  = "1000"
  critical_threshold = "2000"

  queries = [{
    location       = %[2]q
    location_type  = %[3]q
    location_bool  = %[4]q
    location_value = %[5]q
  }]
}
`, name, location, locationType, locationBool, locationValue)
}

// TestAccNNACheckOmittedThresholds covers the natural config for the
// abnormal_behavior metric, which ignores thresholds entirely. Both
// threshold attributes are Optional-but-not-Computed, and NNA stores an
// unsent threshold as SQL NULL rather than "" - so this is the case where a
// mishandled null/empty round-trip would surface as a "provider produced
// inconsistent result after apply" error rather than a clean no-op.
func TestAccNNACheckOmittedThresholds(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceNoThresholds(name, port),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "metric", "abnormal_behavior"),
					resource.TestCheckNoResourceAttr(rName, "warning_threshold"),
					resource.TestCheckNoResourceAttr(rName, "critical_threshold"),
				),
			},
			{
				// Re-applying the identical config must be an empty plan;
				// a null-vs-"" mismatch would show up here as a perpetual diff.
				Config:   testAccNNACheckResourceNoThresholds(name, port),
				PlanOnly: true,
			},
		},
	})
}

func testAccNNACheckResourceNoThresholds(name string, port int) string {
	return testAccNNACheckSource(name, port) + fmt.Sprintf(`
resource "nagios_nna_check" "check" {
  name        = %[1]q
  object_type = "source"
  object_id   = nagios_nna_source.source.id
  metric      = "abnormal_behavior"
}
`, name)
}

// TestAccNNACheckAlertUsers covers the alert_* attributes end-to-end, which
// is the only way to catch a wrong read-side association_type discriminator:
// the write succeeds regardless, and a mismatch only shows up when the
// read-back fails to put the id back in the attribute the plan set it on.
// alert_users is the one of the four whose recipient this provider can
// create (nagios_nna_user); the other three reference Network Analyzer
// objects with no resource yet.
func TestAccNNACheckAlertUsers(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_check.check"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNACheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNACheckResourceAlertUsers(name, port, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckResourceAttr(rName, "alert_users.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(rName, "alert_users.*", "nagios_nna_user.recipient", "id"),
				),
			},
			{
				Config:   testAccNNACheckResourceAlertUsers(name, port, true),
				PlanOnly: true,
			},
			{
				// Dropping the attribute must round-trip back to unset
				// rather than leaving a stale association in state.
				Config: testAccNNACheckResourceAlertUsers(name, port, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNACheckExists(t, rName),
					resource.TestCheckNoResourceAttr(rName, "alert_users.#"),
				),
			},
		},
	})
}

func testAccNNACheckResourceAlertUsers(name string, port int, withUser bool) string {
	alert := ""
	if withUser {
		alert = "  alert_users = [nagios_nna_user.recipient.id]\n"
	}
	return testAccNNACheckSource(name, port) + fmt.Sprintf(`
resource "nagios_nna_user" "recipient" {
  username = %[1]q
  password = "Secret123!"
  email    = "%[1]s@example.com"
  role_id  = %[4]d
}

resource "nagios_nna_check" "check" {
  name               = %[2]q
  object_type        = "source"
  object_id          = nagios_nna_source.source.id
  metric             = "bytes"
  warning_threshold  = "1000"
  critical_threshold = "2000"
%[3]s}
`, name+"_user", name, alert, nnaBuiltinUserRoleID)
}
