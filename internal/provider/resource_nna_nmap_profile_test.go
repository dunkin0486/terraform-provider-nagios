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

func TestAccNNANmapProfileBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceBasic(name, "-sn -T4 -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "parameters", "-sn -T4 -e eth0"),
					resource.TestCheckResourceAttr(rName, "description", "created by acceptance tests"),
					resource.TestCheckResourceAttr(rName, "tags.#", "2"),
					resource.TestCheckTypeSetElemAttr(rName, "tags.*", "Ping"),
					resource.TestCheckTypeSetElemAttr(rName, "tags.*", "Quick"),
					resource.TestCheckResourceAttr(rName, "times_ran", "0"),
					resource.TestCheckResourceAttrSet(rName, "id"),
				),
			},
			{
				// ImportState writes only the numeric id and leans entirely
				// on Read to populate everything else, against a null prior
				// state - the one state shape no other step produces, and
				// the one the empty-preserving write-back has to get right.
				ResourceName:      rName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNNANmapProfileUpdate confirms Update addresses the profile by its
// immutable numeric id (so a rename is an ordinary field update, not a
// rename-by-old-name PUT like this provider's XI resources) and that every
// writable field round-trips through a change.
func TestAccNNANmapProfileUpdate(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	renamed := name + "_renamed"
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceBasic(name, "-sn -T4 -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "parameters", "-sn -T4 -e eth0"),
				),
			},
			{
				Config: testAccNNANmapProfileResourceUpdated(renamed, "-sS -p 1-1024 -T3 -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", renamed),
					resource.TestCheckResourceAttr(rName, "parameters", "-sS -p 1-1024 -T3 -e eth0"),
					resource.TestCheckResourceAttr(rName, "description", "updated by acceptance tests"),
					resource.TestCheckResourceAttr(rName, "tags.#", "1"),
					resource.TestCheckTypeSetElemAttr(rName, "tags.*", "TCP SYN"),
				),
			},
		},
	})
}

// TestAccNNANmapProfileMinimal confirms a profile configured with only the
// two required attributes applies cleanly and leaves the optional
// description/tags null without producing a perpetual diff - tags is a
// nullable JSON column, so an unset set must not read back as an empty one.
func TestAccNNANmapProfileMinimal(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceMinimal(name, "-sn -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "parameters", "-sn -e eth0"),
					resource.TestCheckNoResourceAttr(rName, "tags.#"),
				),
			},
		},
	})
}

func testAccCheckNNANmapProfileExists(t *testing.T, resourceName string) resource.TestCheckFunc {
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
		got, err := c.GetNmapProfile(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA nmap profile id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNANmapProfileDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_nmap_profile" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetNmapProfile(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA nmap profile id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

func testAccNNANmapProfileResourceBasic(name, parameters string) string {
	return fmt.Sprintf(`
resource "nagios_nna_nmap_profile" "profile" {
	name        = %[1]q
	parameters  = %[2]q
	description = "created by acceptance tests"
	tags        = ["Ping", "Quick"]
}
`, name, parameters)
}

func testAccNNANmapProfileResourceUpdated(name, parameters string) string {
	return fmt.Sprintf(`
resource "nagios_nna_nmap_profile" "profile" {
	name        = %[1]q
	parameters  = %[2]q
	description = "updated by acceptance tests"
	tags        = ["TCP SYN"]
}
`, name, parameters)
}

func testAccNNANmapProfileResourceMinimal(name, parameters string) string {
	return fmt.Sprintf(`
resource "nagios_nna_nmap_profile" "profile" {
	name       = %[1]q
	parameters = %[2]q
}
`, name, parameters)
}

// TestAccNNANmapProfileClearOptionalFields confirms a description and tag
// list set on create can be driven back to unset. Network Analyzer's PUT
// preserves omitted fields, so this is only possible because the client
// always serializes both explicitly - and it confirms the cleared values
// read back as null rather than as "" / [], which would be perpetual drift.
func TestAccNNANmapProfileClearOptionalFields(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceBasic(name, "-sn -T4 -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", "created by acceptance tests"),
					resource.TestCheckResourceAttr(rName, "tags.#", "2"),
				),
			},
			{
				Config: testAccNNANmapProfileResourceMinimal(name, "-sn -T4 -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckNoResourceAttr(rName, "description"),
					resource.TestCheckNoResourceAttr(rName, "tags.#"),
				),
			},
		},
	})
}

// TestAccNNANmapProfileRejectsParametersWithoutInterfaceFlag confirms the
// schema validator refuses a parameters string lacking a standalone
// "-e <interface>" flag at plan time. Network Analyzer only enforces that
// rule on create, not on update, so without this a config could be applied
// as a change but never recreated from scratch.
func TestAccNNANmapProfileRejectsParametersWithoutInterfaceFlag(t *testing.T) {
	name := "tf_" + acctest.RandString(10)

	for _, parameters := range []string{"-sn -T4", "-sn -e=eth0", "-sn -eeth0", "-sn --interface eth0", "-T4 -A -v -e <iface>"} {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccNNAPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config:      testAccNNANmapProfileResourceMinimal(name, parameters),
					ExpectError: regexp.MustCompile(`must include a standalone "-e <interface>" flag`),
				},
			},
		})
	}
}

// TestAccNNANmapProfileAcceptsInterfaceFlagVariants is the positive half of
// the test above: forms Network Analyzer does accept must not be rejected
// by the provider's own validator.
func TestAccNNANmapProfileAcceptsInterfaceFlagVariants(t *testing.T) {
	for _, parameters := range []string{"-e eth0", "-sn -e  eth0", "-sn --script-args e=1 -e eth0", "-sn -e eth0 127.0.0.1"} {
		name := "tf_" + acctest.RandString(10)
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccNNAPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
			Steps: []resource.TestStep{
				{
					Config: testAccNNANmapProfileResourceMinimal(name, parameters),
					Check: resource.ComposeTestCheckFunc(
						testAccCheckNNANmapProfileExists(t, "nagios_nna_nmap_profile.profile"),
						resource.TestCheckResourceAttr("nagios_nna_nmap_profile.profile", "parameters", parameters),
					),
				},
			},
		})
	}
}

// TestAccNNANmapProfileExplicitEmptyOptionals is the regression test for
// the empty-vs-unset ambiguity. Network Analyzer collapses an empty
// description and an empty tag list into "no value" and cannot report them
// back as distinct from unset, while Terraform plans `description = ""` and
// `tags = []` as known, non-null values. Neither attribute is Computed, so
// normalizing the server's collapsed form into state would abort the apply
// with "Provider produced inconsistent result after apply" - this asserts
// both forms apply cleanly and survive a second plan with no diff.
func TestAccNNANmapProfileExplicitEmptyOptionals(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceExplicitEmpty(name, "-sn -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
					resource.TestCheckResourceAttr(rName, "tags.#", "0"),
				),
			},
			{
				// A no-op re-apply: if the explicit empties were being
				// rewritten to null in state, this step would show a diff.
				Config:   testAccNNANmapProfileResourceExplicitEmpty(name, "-sn -e eth0"),
				PlanOnly: true,
			},
			{
				// Import is the one place the explicit-empty forms
				// legitimately can't be recovered: there's no plan to
				// preserve them from, and Network Analyzer reports an empty
				// description and an empty tag list identically to unset
				// ones, so both come back null rather than "" / [].
				// ImportStateVerifyIgnore records that divergence rather
				// than hiding it - it converges on the next apply, which
				// the PlanOnly step above already proves is a no-op from
				// the configured side.
				ResourceName:            rName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"description", "tags.#"},
			},
		},
	})
}

// TestAccNNANmapProfileExplicitEmptyToPopulated confirms the explicit-empty
// forms are a real starting point for an update, not just a stable
// end state - going from "" / [] to populated values and back exercises
// both directions through the same partial-update PUT.
func TestAccNNANmapProfileExplicitEmptyToPopulated(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_nmap_profile.profile"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNANmapProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNANmapProfileResourceExplicitEmpty(name, "-sn -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
					resource.TestCheckResourceAttr(rName, "tags.#", "0"),
				),
			},
			{
				Config: testAccNNANmapProfileResourceBasic(name, "-sn -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", "created by acceptance tests"),
					resource.TestCheckResourceAttr(rName, "tags.#", "2"),
				),
			},
			{
				Config: testAccNNANmapProfileResourceExplicitEmpty(name, "-sn -e eth0"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNANmapProfileExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
					resource.TestCheckResourceAttr(rName, "tags.#", "0"),
				),
			},
		},
	})
}

func testAccNNANmapProfileResourceExplicitEmpty(name, parameters string) string {
	return fmt.Sprintf(`
resource "nagios_nna_nmap_profile" "profile" {
	name        = %[1]q
	parameters  = %[2]q
	description = ""
	tags        = []
}
`, name, parameters)
}
