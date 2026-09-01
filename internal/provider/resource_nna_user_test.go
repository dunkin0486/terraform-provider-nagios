package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// nnaBuiltinUserRoleID is NNA's built-in, protected "User" role - present on
// every fresh instance (confirmed live, id 2 alongside "Admin" at id 1) -
// used here rather than standing up a nagios_nna_role resource this
// provider doesn't manage yet.
const nnaBuiltinUserRoleID = 2

func TestAccNNAUserBasic(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_user.user"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNAUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNAUserResourceBasic(username, "Secret123!"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNAUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "username", username),
					resource.TestCheckResourceAttr(rName, "email", username+"@example.com"),
					resource.TestCheckResourceAttr(rName, "role_id", strconv.Itoa(nnaBuiltinUserRoleID)),
					resource.TestCheckResourceAttr(rName, "apiaccess", "false"),
					resource.TestCheckResourceAttr(rName, "theme", "default"),
					resource.TestCheckResourceAttr(rName, "active", "true"),
					resource.TestCheckResourceAttrSet(rName, "id"),
					resource.TestCheckResourceAttrSet(rName, "apikey"),
				),
			},
		},
	})
}

// TestAccNNAUserUpdateProfile confirms Update addresses the user by its
// immutable numeric id via PATCH (CLAUDE.md/nna package doc: unlike every
// other NNA type in this client, users use PATCH, not PUT) and that changed
// profile fields round-trip.
func TestAccNNAUserUpdateProfile(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_user.user"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNAUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNAUserResourceBasic(username, "Secret123!"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNAUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "theme", "default"),
				),
			},
			{
				Config: testAccNNAUserResourceWithProfile(username, "Secret123!"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNAUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "theme", "dark"),
					resource.TestCheckResourceAttr(rName, "first_name", "Test"),
					resource.TestCheckResourceAttr(rName, "last_name", "User"),
					resource.TestCheckResourceAttr(rName, "active", "false"),
				),
			},
		},
	})
}

// TestAccNNAUserClearingFirstNameForcesReplace confirms the
// nnaUserClearRequiresReplace plan modifier substitutes a destroy+recreate
// for the update Network Analyzer's own PATCH endpoint can't perform:
// confirmed live (#154), first_name can never be cleared back to unset once
// set - sending "", null, or omitting the key are all silently ignored.
func TestAccNNAUserClearingFirstNameForcesReplace(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_user.user"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNAUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNAUserResourceWithProfile(username, "Secret123!"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNAUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "first_name", "Test"),
				),
			},
			{
				Config: testAccNNAUserResourceBasic(username, "Secret123!"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(rName, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNAUserExists(t, rName),
					resource.TestCheckNoResourceAttr(rName, "first_name"),
				),
			},
		},
	})
}

func testAccCheckNNAUserExists(t *testing.T, resourceName string) resource.TestCheckFunc {
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
		got, err := c.GetUser(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA user id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNAUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_user" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetUser(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA user id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

func testAccNNAUserResourceBasic(username, password string) string {
	return fmt.Sprintf(`
resource "nagios_nna_user" "user" {
	username = %[1]q
	password = %[2]q
	email    = "%[1]s@example.com"
	role_id  = %[3]d
}
`, username, password, nnaBuiltinUserRoleID)
}

func testAccNNAUserResourceWithProfile(username, password string) string {
	return fmt.Sprintf(`
resource "nagios_nna_user" "user" {
	username   = %[1]q
	password   = %[2]q
	email      = "%[1]s@example.com"
	role_id    = %[3]d
	theme      = "dark"
	first_name = "Test"
	last_name  = "User"
	active     = false
}
`, username, password, nnaBuiltinUserRoleID)
}
