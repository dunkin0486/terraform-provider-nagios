package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNNARoleBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttrSet(rName, "id"),
					// type and protected are read-only: Network Analyzer
					// forces every API-created role to "custom"/false no
					// matter what is sent (confirmed live, #155).
					resource.TestCheckResourceAttr(rName, "type", "custom"),
					resource.TestCheckResourceAttr(rName, "protected", "false"),
					resource.TestCheckResourceAttr(rName, "traceroute_permissions.traceroutes.#", "1"),
					// Permission groups the config omitted must round-trip as
					// null blocks, not empty ones - Network Analyzer stores
					// them as SQL NULL.
					resource.TestCheckNoResourceAttr(rName, "suricata_permissions.alerts"),
					resource.TestCheckNoResourceAttr(rName, "nmap_permissions.scans.#"),
				),
			},
		},
	})
}

// TestAccNNARoleEmptyTraceroutePermissions covers the quirk that motivated
// making traceroute_permissions a Required block: its database column is
// NOT NULL with no default and Network Analyzer's validator doesn't cover the
// gap, so the empty-block case has to reach the API as an empty object rather
// than a null or an absent key. It also exercises the round-trip of Network
// Analyzer echoing an empty permission object back as an empty JSON ARRAY,
// which must not surface as a diff.
func TestAccNNARoleEmptyTraceroutePermissions(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceEmptyTraceroute(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckNoResourceAttr(rName, "traceroute_permissions.traceroutes.#"),
					// The empty optional block must survive as a present-but-empty
					// block, not collapse to null.
					resource.TestCheckNoResourceAttr(rName, "report_permissions.reports.#"),
				),
			},
		},
	})
}

// TestAccNNARoleAllPermissionGroups confirms every one of the six permission
// groups round-trips, including the booleans (which are always sent, since
// false is a meaningful "explicitly denied") and the verb sets.
func TestAccNNARoleAllPermissionGroups(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceAllGroups(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "flow_source_permissions.sources.#", "4"),
					resource.TestCheckResourceAttr(rName, "flow_source_permissions.start_stop_sources", "true"),
					resource.TestCheckResourceAttr(rName, "report_permissions.reports.#", "1"),
					resource.TestCheckResourceAttr(rName, "traceroute_permissions.ncpa_host.#", "2"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.alerts", "true"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.config", "false"),
					resource.TestCheckResourceAttr(rName, "wireshark_permissions.start_stop_capture", "true"),
					resource.TestCheckResourceAttr(rName, "wireshark_permissions.start_stop_ring_buffer", "false"),
					resource.TestCheckResourceAttr(rName, "nmap_permissions.scans.#", "1"),
				),
			},
		},
	})
}

// TestAccNNARoleUpdateName confirms a rename is an in-place update: unlike
// Nagios XI's rename-by-old-name PUT (CLAUDE.md quirk 3), Network Analyzer
// addresses PUT by the immutable numeric id, so the id must survive.
func TestAccNNARoleUpdateName(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	renamed := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
				),
			},
			{
				Config: testAccNNARoleResourceBasic(renamed),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", renamed),
				),
			},
		},
	})
}

// TestAccNNARoleUpdatePermissions confirms changing a permission group in
// place works, and that dropping a previously-set optional group clears it
// back to unconfigured rather than leaving the old value behind (Network
// Analyzer's PUT is a partial update, so the provider has to send the dropped
// group as an explicit null - see nnaRoleResource.Update).
func TestAccNNARoleUpdatePermissions(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceAllGroups(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "nmap_permissions.scans.#", "1"),
				),
			},
			{
				Config: testAccNNARoleResourceBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckNoResourceAttr(rName, "nmap_permissions.scans.#"),
					resource.TestCheckResourceAttr(rName, "traceroute_permissions.traceroutes.#", "1"),
				),
			},
		},
	})
}

// TestAccNNARoleImport confirms an existing role can be imported by its
// numeric id.
func TestAccNNARoleImport(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceBasic(name),
				Check:  testAccCheckNNARoleExists(t, rName),
			},
			{
				ResourceName:      rName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNNARoleCreateAfterManualDestroy confirms a role deleted outside
// Terraform is recreated on the next apply rather than erroring - exercising
// GetRole's (nil, nil) not-found contract through the list-and-filter path it
// has to use, since Network Analyzer has no get-a-role-by-id route.
func TestAccNNARoleCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceBasic(name),
				Check:  testAccCheckNNARoleExists(t, rName),
			},
			{
				PreConfig: func() {
					c := testAccNNAClient(t)
					roles, err := c.ListRoles(context.Background())
					if err != nil {
						t.Fatalf("listing NNA roles: %s", err)
					}
					for _, role := range roles {
						if role.Name != name {
							continue
						}
						if err := c.DeleteRole(context.Background(), role.ID); err != nil {
							t.Fatalf("manually deleting NNA role %d: %s", role.ID, err)
						}
					}
				},
				Config: testAccNNARoleResourceBasic(name),
				Check:  testAccCheckNNARoleExists(t, rName),
			},
		},
	})
}

func testAccCheckNNARoleExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
		}

		c := testAccNNAClient(t)
		got, err := c.GetRole(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA role id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNARoleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_role" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetRole(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA role id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

func testAccNNARoleResourceBasic(name string) string {
	return fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	traceroute_permissions = {
		traceroutes = ["get"]
	}
}
`, name)
}

func testAccNNARoleResourceEmptyTraceroute(name string) string {
	return fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	traceroute_permissions = {}

	# An empty OPTIONAL group too: report_permissions is made up entirely of
	# omitempty fields, so an empty block is the only shape that reaches
	# Network Analyzer as a bare {} on a NULLABLE column - the one case where
	# a server-side {}-to-NULL normalization would drop the block and fail the
	# apply with "produced inconsistent result after apply".
	report_permissions = {}
}
`, name)
}

func testAccNNARoleResourceAllGroups(name string) string {
	return fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	flow_source_permissions = {
		sources            = ["get", "post", "put", "delete"]
		start_stop_sources = true
	}

	report_permissions = {
		reports        = ["get"]
		report_history = ["get", "put"]
	}

	traceroute_permissions = {
		ncpa_host       = ["get", "post"]
		traceroutes     = ["get"]
		scheduled_scans = ["get"]
	}

	suricata_permissions = {
		data            = ["get"]
		rules           = ["get"]
		rulesets        = ["get"]
		alerts          = true
		config          = false
		scan_pcap       = true
		start_stop_scan = false
	}

	wireshark_permissions = {
		pcaps                  = ["get"]
		ring_buffer            = ["get"]
		start_stop_capture     = true
		start_stop_ring_buffer = false
	}

	nmap_permissions = {
		scans           = ["get"]
		ndiffs          = ["get"]
		profiles        = ["get"]
		scheduled_scans = ["get"]
	}
}
`, name)
}

// TestAccNNARoleRejectsEmptyVerbList confirms an explicit empty verb list is
// rejected at plan time with an actionable message, rather than failing
// mid-apply with Terraform's opaque "produced inconsistent result after
// apply" - Network Analyzer stores an empty verb list and an absent one
// identically, and this provider reads both back as unset.
func TestAccNNARoleRejectsEmptyVerbList(t *testing.T) {
	name := "tf_" + acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	traceroute_permissions = {
		traceroutes = []
	}
}
`, name),
				ExpectError: regexp.MustCompile(`must contain at least 1`),
			},
		},
	})
}

// TestAccNNARoleRejectsInvalidVerb confirms the schema catches a verb Network
// Analyzer itself would accept and store verbatim as a dead permission.
func TestAccNNARoleRejectsInvalidVerb(t *testing.T) {
	name := "tf_" + acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	traceroute_permissions = {
		traceroutes = ["fly"]
	}
}
`, name),
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
			},
		},
	})
}

// TestAccNNARoleDestroyToleratesExternalDelete confirms a role already
// removed out-of-band destroys cleanly. DeleteRole is not idempotent
// server-side (a repeat delete 404s - confirmed live), unlike
// DeleteSource/DeleteSourceGroup, so without the resource's not-found
// tolerance this would fail the destroy and force a `terraform state rm`.
func TestAccNNARoleDestroyToleratesExternalDelete(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNARoleResourceBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					// Delete the role out-of-band as the final act of the
					// step, so the test framework's own destroy at teardown
					// runs against an id that is already gone.
					func(s *terraform.State) error {
						c := testAccNNAClient(t)
						rs := s.RootModule().Resources[rName]
						id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
						if err != nil {
							return err
						}
						return c.DeleteRole(context.Background(), id)
					},
				),
				// The out-of-band delete above deliberately leaves the
				// post-apply refresh wanting to recreate the role.
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccNNARolePartialPermissionBlock pins down the nested booldefault
// behavior in the repo's first SingleNestedAttribute: a block that is present
// but omits its boolean attributes. The booleans are Optional+Computed with a
// false default inside an Optional (non-Computed) nested block, so if the
// framework didn't apply the default here the plan would hold null while
// modelFromNNARole writes false, failing with "Provider produced inconsistent
// result after apply". Nothing else in this provider exercises that path.
func TestAccNNARolePartialPermissionBlock(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_role.role"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNARoleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nagios_nna_role" "role" {
	name = %[1]q

	# Verb list set, every boolean omitted - the defaults must fill in.
	flow_source_permissions = {
		sources = ["get"]
	}

	suricata_permissions = {
		data = ["get"]
	}

	# Booleans set, every verb list omitted - the inverse shape.
	wireshark_permissions = {
		start_stop_capture = true
	}

	traceroute_permissions = {
		traceroutes = ["get"]
	}
}
`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNARoleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "flow_source_permissions.start_stop_sources", "false"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.alerts", "false"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.config", "false"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.scan_pcap", "false"),
					resource.TestCheckResourceAttr(rName, "suricata_permissions.start_stop_scan", "false"),
					resource.TestCheckResourceAttr(rName, "wireshark_permissions.start_stop_capture", "true"),
					resource.TestCheckResourceAttr(rName, "wireshark_permissions.start_stop_ring_buffer", "false"),
					// An omitted verb list inside a present block stays null.
					resource.TestCheckNoResourceAttr(rName, "wireshark_permissions.pcaps.#"),
				),
			},
		},
	})
}
